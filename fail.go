package engine

import (
	"context"
	"math/rand/v2"
	"time"
)

// Breaker is one deliberate way for an action to fail: from valid parameters,
// it sends something the API must refuse, and says which status to expect.
type Breaker[P any] struct {
	// Kind names the error, as shown in the journal: "missing description".
	Kind string
	// Expect is the HTTP status the API must answer.
	Expect int
	// Call sends the broken request built from valid parameters. It returns
	// the status, what was actually sent (for the journal) and a technical
	// error if there was no answer.
	Call func(ctx context.Context, valid P, traceparent string) (status int, sent any, err error)
}

// Fail declares a deliberate error that can follow an action: `to` is called
// the wrong way, as described by b, with parameters built by from (as for
// Follow). The walk then goes on from the same place, like a user who fixes
// a form and tries again.
func Fail[P, R, P2, R2 any](to *Action[P2, R2], b Breaker[P2], from func(r *rand.Rand, in P, out R) (P2, bool)) Next[P, R] {
	return fail[P, R, P2]{name: to.Name, b: b, from: from}
}

// FailAll declares every breaker of `to` as a follow-up in error.
func FailAll[P, R, P2, R2 any](to *Action[P2, R2], bs []Breaker[P2], from func(r *rand.Rand, in P, out R) (P2, bool)) []Next[P, R] {
	out := make([]Next[P, R], 0, len(bs))
	for _, b := range bs {
		out = append(out, Fail(to, b, from))
	}
	return out
}

type fail[P, R, P2 any] struct {
	name string
	b    Breaker[P2]
	from func(r *rand.Rand, in P, out R) (P2, bool)
}

func (f fail[P, R, P2]) offer(r *rand.Rand, in P, out R) (Step, bool) {
	p, ok := f.from(r, in, out)
	if !ok {
		return nil, false
	}
	return failing[P2]{name: f.name, b: f.b, p: p}, true
}

func (fail[P, R, P2]) failing() bool { return true }

func (f fail[P, R, P2]) link() Link { return Link{To: f.name, Fail: f.b.Kind} }

// FailStart makes a deliberate error an entry point of the walks.
func FailStart[P, R any](a *Action[P, R], b Breaker[P], weight int) Start {
	return Start{Weight: weight, Action: a.Name, Fail: b.Kind, fail: true, make: func(r *rand.Rand) Step {
		return failing[P]{name: a.Name, b: b, p: a.Generate(r)}
	}}
}

// failing is a deliberate error, ready to run.
type failing[P any] struct {
	name string
	b    Breaker[P]
	p    P
}

func (f failing[P]) Name() string { return f.name }

func (f failing[P]) Run(ctx context.Context, u *User) ([]Candidate, error) {
	tr := newTrace()
	start := time.Now()
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CallTimeout)
	defer cancel()
	status, sent, err := f.b.Call(callCtx, f.p, tr.header())
	rec := Record{
		Time: start, Action: f.name, Params: sent, Status: status, TraceID: tr.id,
		Duration: time.Since(start), Intent: f.b.Kind, Expect: f.b.Expect,
	}
	if err != nil {
		rec.Err = err.Error()
	}
	u.record(rec)
	return nil, ctx.Err() // the walk keeps the candidates it had (see User.Walk)
}
