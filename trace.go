package engine

import (
	"crypto/rand"
	"encoding/hex"
)

// trace is the client side of a W3C trace context: the client chooses the
// trace ID, so the journal has it even when the service never answers.
type trace struct{ id, span string }

func newTrace() trace {
	var t [16]byte
	var s [8]byte
	_, _ = rand.Read(t[:]) // crypto/rand: trace IDs must be unique across runs, whatever the seed
	_, _ = rand.Read(s[:])
	return trace{id: hex.EncodeToString(t[:]), span: hex.EncodeToString(s[:])}
}

func (t trace) header() string { return "00-" + t.id + "-" + t.span + "-01" }
