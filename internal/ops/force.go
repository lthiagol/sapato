package ops

import (
	"errors"
	"slices"
)

// ErrNeedsForce marks a refusal that Options.Force overrides. Check for it with
// errors.Is.
var ErrNeedsForce = errors.New("needs --force")

// ForceRefusal is a refusal that Options.Force overrides. Msg is the CLI text. Reasons
// says why, one short line each, for a UI to show without the op prefix or the
// --force hint. Read it with errors.As.
type ForceRefusal struct {
	Msg     string
	Reasons []string
}

func (e *ForceRefusal) Error() string { return e.Msg }

// Unwrap makes errors.Is(err, ErrNeedsForce) true.
func (e *ForceRefusal) Unwrap() error { return ErrNeedsForce }

// forceRefusal is a ForceRefusal with the CLI text msg and the reasons.
func forceRefusal(msg string, reasons ...string) error {
	return &ForceRefusal{Msg: msg, Reasons: reasons}
}

// addReason appends reason to reasons unless it is there already, so an agent
// whose several copies are refused is named once.
func addReason(reasons []string, reason string) []string {
	if slices.Contains(reasons, reason) {
		return reasons
	}
	return append(reasons, reason)
}
