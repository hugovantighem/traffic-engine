package engine

import (
	"context"
	"math/rand/v2"
	"time"
)

// Config is how a simulated user behaves.
type Config struct {
	Starts   []Start
	Picker   Picker
	MaxSteps int
	// Pace is the pause between two steps; nil for none.
	Pace Pace
	// OnStep is called after each step, when set.
	OnStep func(userID string, r Record)
	// Only, when set, keeps the steps of the actions it accepts: the others are never played,
	// even when an action links to them (see Graph).
	Only func(action string) bool
}

// User is one simulated user. It keeps only its journal: what to do next
// comes from the previous action, not from a memory.
type User struct {
	ID      string
	Seed    uint64
	Journal []Record

	cfg   Config
	rand  *rand.Rand
	walks int
}

// NewUser makes a user whose choices are reproducible from its seed.
func NewUser(id string, seed uint64, cfg Config) *User {
	if cfg.Picker == nil {
		cfg.Picker = Weighted{StopProb: 0.15}
	}
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 20
	}
	return &User{ID: id, Seed: seed, cfg: cfg, rand: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

// Walk runs one journey: an entry point, then follow-ups until the picker ends
// the walk or MaxSteps is reached.
func (u *User) Walk(ctx context.Context) error {
	u.walks++
	cands := make([]Candidate, 0, len(u.cfg.Starts))
	for _, s := range u.cfg.Starts {
		cands = append(cands, Candidate{Step: s.make(u.rand), Weight: s.Weight, Fail: s.fail})
	}
	for n := 0; n < u.cfg.MaxSteps; n++ {
		cands = u.keep(cands)
		i, err := u.cfg.Picker.Pick(u.rand, cands, n)
		if err != nil {
			return err
		}
		if i < 0 {
			return nil
		}
		if n > 0 {
			if err := u.think(ctx); err != nil {
				return err
			}
		}
		if cands[i].Fail {
			// A deliberate error leads nowhere: the user stays where it was and
			// picks again among the same follow-ups.
			if _, err := cands[i].Step.Run(ctx, u); err != nil {
				return err
			}
			continue
		}
		if cands, err = cands[i].Step.Run(ctx, u); err != nil {
			return err
		}
	}
	return nil
}

// keep drops the candidates of the actions Only refuses.
func (u *User) keep(cands []Candidate) []Candidate {
	if u.cfg.Only == nil {
		return cands
	}
	out := cands[:0:0]
	for _, c := range cands {
		if u.cfg.Only(c.Step.Name()) {
			out = append(out, c)
		}
	}
	return out
}

func (u *User) think(ctx context.Context) error {
	if u.cfg.Pace == nil {
		return nil
	}
	return Sleep(ctx, u.cfg.Pace.Delay(u.rand))
}

// Sleep waits for d, or until ctx is done.
func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (u *User) record(r Record) {
	r.Walk = u.walks
	u.Journal = append(u.Journal, r)
	if u.cfg.OnStep != nil {
		u.cfg.OnStep(u.ID, r)
	}
}
