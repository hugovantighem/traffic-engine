package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Record is one line of a user's journal.
type Record struct {
	// Walk numbers the walk of the user the step belongs to (1 for its first walk).
	Walk     int
	Time     time.Time
	Action   string
	Params   any
	Status   int    // 0 when there was no HTTP response
	TraceID  string // chosen by the client, sent as traceparent
	Err      string // technical error, if any
	Duration time.Duration
	// Intent names a deliberate error ("missing description"), empty for a
	// normal action; Expect is the status the API must answer then.
	Intent string
	Expect int
}

// Missed tells whether a deliberate error did not get the expected status.
func (r Record) Missed() bool { return r.Intent != "" && r.Status != r.Expect }

// PrintJournal writes the user's journey, one line per step.
func (u *User) PrintJournal(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%s (seed %d)\n", u.ID, u.Seed)
	for i, r := range u.Journal {
		params, _ := json.Marshal(r.Params)
		status := fmt.Sprint(r.Status)
		if r.Err != "" {
			status = "ERR " + r.Err
		}
		if r.Intent != "" {
			status += fmt.Sprintf(" (intended: %s, expected %d)", r.Intent, r.Expect)
		}
		_, _ = fmt.Fprintf(w, "%2d %-14s %-8s %s trace=%s %s\n", i+1, r.Action, status, params, r.TraceID, r.Duration.Round(time.Millisecond))
	}
}
