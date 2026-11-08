package engine

import (
	"math/rand/v2"
	"time"
)

// Pace is the pause a user takes before its next action.
type Pace interface {
	Delay(r *rand.Rand) time.Duration
}

// Fixed is a base delay with a random jitter around it.
type Fixed struct{ Base, Jitter time.Duration }

// Delay implements Pace.
func (f Fixed) Delay(r *rand.Rand) time.Duration { return Jittered(r, f.Base, f.Jitter) }

// Jittered draws a delay uniformly in [base-jitter, base+jitter], cut at zero.
// The interval is cut, not the value: with a base of zero the delays spread
// over [0, jitter] instead of piling up at zero, so users never fire together.
func Jittered(r *rand.Rand, base, jitter time.Duration) time.Duration {
	lo, hi := max(base-jitter, 0), max(base+jitter, 0)
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(r.Int64N(int64(hi-lo)+1))
}
