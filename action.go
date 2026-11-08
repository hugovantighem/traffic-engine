// Package engine is a small traffic engine: simulated users walk through the
// actions of a system. Each action declares what can follow it and how to build
// the parameters of that next action from what it sent and what it received,
// so there is no global memory and no scenario to write by hand.
package engine

import (
	"context"
	"math/rand/v2"
	"time"
)

// CallTimeout bounds one call; stopping a user never cuts a call short.
const CallTimeout = 30 * time.Second

// Result is what an action brings back from one call.
type Result[R any] struct {
	Status int
	// Body is the decoded success body, nil when the API answered with an error.
	Body *R
	// Err is a technical error: no HTTP response (timeout, connection refused...).
	Err error
}

// Call performs one request. traceparent is the W3C header the call must send,
// so that the trace ID is known by the client even when the call fails.
type Call[P, R any] func(ctx context.Context, p P, traceparent string) Result[R]

// Action is one operation of the API (named after its operationId). P is the
// type of its parameters, R the type of its success body.
type Action[P, R any] struct {
	Name string
	Do   Call[P, R]
	// Generate builds valid parameters. Only needed for actions that can start
	// a walk (nothing to link from).
	Generate func(r *rand.Rand) P

	next []Next[P, R]
}

// Then declares what can follow this action. Returns the action for chaining.
func (a *Action[P, R]) Then(n ...Next[P, R]) *Action[P, R] {
	a.next = append(a.next, n...)
	return a
}

// Link is a follow-up an action declares: the action it leads to, and the
// deliberate error, if it is one.
type Link struct {
	To   string
	Fail string // the kind of the deliberate error, empty for a normal follow-up
}

// ActionName is the name of the action (see Graph).
func (a *Action[P, R]) ActionName() string { return a.Name }

// Links lists what can follow the action, so that a run can tell what it
// covered.
func (a *Action[P, R]) Links() []Link {
	out := make([]Link, 0, len(a.next))
	for _, n := range a.next {
		out = append(out, n.link())
	}
	return out
}

// Next is one possible follow-up of an Action[P, R].
type Next[P, R any] interface {
	link() Link
	// offer builds the follow-up step, or reports that it is not possible
	// (for example: nothing to pick in an empty list).
	offer(r *rand.Rand, in P, out R) (Step, bool)
}

// Follow declares that `to` can come after an action. from receives the
// parameters the action sent and the body it received, and builds the
// parameters of `to`. It returns false when `to` is not possible this time.
func Follow[P, R, P2, R2 any](to *Action[P2, R2], from func(r *rand.Rand, in P, out R) (P2, bool)) Next[P, R] {
	return follow[P, R, P2, R2]{to: to, from: from}
}

type follow[P, R, P2, R2 any] struct {
	to   *Action[P2, R2]
	from func(r *rand.Rand, in P, out R) (P2, bool)
}

func (f follow[P, R, P2, R2]) link() Link { return Link{To: f.to.Name} }

func (f follow[P, R, P2, R2]) offer(r *rand.Rand, in P, out R) (Step, bool) {
	p, ok := f.from(r, in, out)
	if !ok {
		return nil, false
	}
	return bound[P2, R2]{a: f.to, p: p}, true
}

// Step is an action with its parameters, ready to run.
type Step interface {
	Name() string
	// Run executes the call, records it in the user's journal and returns the
	// steps that can follow.
	Run(ctx context.Context, u *User) ([]Candidate, error)
}

// Candidate is a step the picker may choose.
type Candidate struct {
	Step   Step
	Weight int
	// Fail marks a deliberate error (see Fail).
	Fail bool
}

type bound[P, R any] struct {
	a *Action[P, R]
	p P
}

func (b bound[P, R]) Name() string { return b.a.Name }

func (b bound[P, R]) Run(ctx context.Context, u *User) ([]Candidate, error) {
	tr := newTrace()
	start := time.Now()
	// A stop request lets the call in flight finish: the user stops between two
	// actions, never in the middle of one.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CallTimeout)
	defer cancel()
	res := b.a.Do(callCtx, b.p, tr.header())
	rec := Record{
		Time:     start,
		Action:   b.a.Name,
		Params:   b.p,
		Status:   res.Status,
		TraceID:  tr.id,
		Duration: time.Since(start),
	}
	if res.Err != nil {
		rec.Err = res.Err.Error()
	}
	u.record(rec)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if res.Err != nil || res.Body == nil {
		return nil, nil // nothing to follow from: the walk ends
	}
	var out []Candidate
	for _, n := range b.a.next {
		if s, ok := n.offer(u.rand, b.p, *res.Body); ok {
			_, isFail := n.(interface{ failing() bool })
			out = append(out, Candidate{Step: s, Weight: 1, Fail: isFail})
		}
	}
	return out, nil
}

// Start declares an action that can start a walk, with its weight.
type Start struct {
	Weight int
	// Action and Fail name the entry point, for the coverage of a run (Fail: the kind of a
	// deliberate error, empty for a normal action).
	Action, Fail string
	fail         bool
	make         func(r *rand.Rand) Step
}

// StartWith makes an action an entry point of the walks.
func StartWith[P, R any](a *Action[P, R], weight int) Start {
	return Start{Weight: weight, Action: a.Name, make: func(r *rand.Rand) Step {
		return bound[P, R]{a: a, p: a.Generate(r)}
	}}
}
