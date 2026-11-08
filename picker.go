package engine

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Picker chooses the next step among the candidates. n is the number of steps
// already done. It returns the index of the chosen candidate, or -1 to end
// the walk.
type Picker interface {
	Pick(r *rand.Rand, c []Candidate, n int) (int, error)
}

// Weighted picks at random, honouring the weights of the candidates. After the
// first step, the walk ends with probability StopProb at each step. With
// probability Errors() it picks among the deliberate errors on offer, if any;
// otherwise among the normal follow-ups.
type Weighted struct {
	StopProb float64
	// Errors is the share of deliberate errors, from 0 to 1; nil for none.
	Errors func() float64
}

// Pick implements Picker.
func (w Weighted) Pick(r *rand.Rand, c []Candidate, n int) (int, error) {
	if len(c) == 0 {
		return -1, nil
	}
	if n > 0 && r.Float64() < w.StopProb {
		return -1, nil
	}
	var normal, failing []int
	for i, x := range c {
		if x.Fail {
			failing = append(failing, i)
		} else {
			normal = append(normal, i)
		}
	}
	pool := normal
	if len(failing) > 0 && w.Errors != nil && r.Float64() < w.Errors() {
		pool = failing
	}
	if len(pool) == 0 {
		return -1, nil
	}
	return pool[weighted(r, c, pool)], nil
}

// weighted draws a position in pool, by the weights of the candidates.
func weighted(r *rand.Rand, c []Candidate, pool []int) int {
	total := 0
	for _, i := range pool {
		total += max(c[i].Weight, 0)
	}
	if total == 0 {
		return r.IntN(len(pool))
	}
	v := r.IntN(total)
	for k, i := range pool {
		v -= max(c[i].Weight, 0)
		if v < 0 {
			return k
		}
	}
	return len(pool) - 1
}

// Scripted imposes a scenario: the operationIds to run, in order. Each step
// must be reachable (an entry point, or a follow-up of the previous action),
// so its parameters still come from the links between actions.
type Scripted []string

// Pick implements Picker.
func (s Scripted) Pick(_ *rand.Rand, c []Candidate, n int) (int, error) {
	if n >= len(s) {
		return -1, nil
	}
	var names []string
	for i, x := range c {
		if x.Fail {
			continue // a scenario plays the normal actions
		}
		if x.Step.Name() == s[n] {
			return i, nil
		}
		names = append(names, x.Step.Name())
	}
	return -1, fmt.Errorf("scenario step %d %q is not reachable here (available: %s)", n+1, s[n], strings.Join(names, ", "))
}
