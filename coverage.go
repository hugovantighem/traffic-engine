package engine

import (
	"fmt"
	"sort"
)

// Node is an action and its links, as declared: what a run should cover.
type Node struct {
	Name  string
	Links []Link
}

// Covered is what runs played, against what the graph declares.
type Covered struct {
	// Missing are the declared entry points and links never played as expected, as
	// "start -> recordDirect", "recordDirect -> getBalance", "recordDirect -> getPurchase !unknown purchase".
	Missing []string
	// Broken are the steps that went wrong: a technical error, a normal action that did not
	// succeed, a deliberate error that did not get its status.
	Broken []string
}

// OK tells whether everything was played, and nothing went wrong.
func (c Covered) OK() bool { return len(c.Missing) == 0 && len(c.Broken) == 0 }

func key(from, to, fail string) string {
	if fail != "" {
		return from + " -> " + to + " !" + fail
	}
	return from + " -> " + to
}

// Cover compares the journals of users with what the graph declares (its entry points and the
// links of its actions). A normal step counts when it succeeded (2xx); a deliberate error when it
// got the status it expected.
func Cover(starts []Start, nodes []Node, journals ...[]Record) Covered {
	want := map[string]bool{}
	for _, s := range starts {
		want[key("start", s.Action, s.Fail)] = true
	}
	for _, n := range nodes {
		for _, l := range n.Links {
			want[key(n.Name, l.To, l.Fail)] = true
		}
	}
	played := map[string]bool{}
	var c Covered
	for _, j := range journals {
		prev, walk := "start", 0
		for _, r := range j {
			if r.Walk != walk {
				prev, walk = "start", r.Walk
			}
			switch {
			case r.Err != "":
				c.Broken = append(c.Broken, fmt.Sprintf("%s: %s", r.Action, r.Err))
			case r.Intent != "" && r.Missed():
				c.Broken = append(c.Broken, fmt.Sprintf("%s !%s: got %d, want %d", r.Action, r.Intent, r.Status, r.Expect))
			case r.Intent != "":
				played[key(prev, r.Action, r.Intent)] = true
			case r.Status < 200 || r.Status > 299:
				c.Broken = append(c.Broken, fmt.Sprintf("%s: got %d", r.Action, r.Status))
			default:
				played[key(prev, r.Action, "")] = true
			}
			if r.Intent == "" {
				prev = r.Action // a deliberate error leaves the user where it was
			}
		}
	}
	for k := range want {
		if !played[k] {
			c.Missing = append(c.Missing, k)
		}
	}
	sort.Strings(c.Missing)
	return c
}
