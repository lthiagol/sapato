package ops

import "errors"

// ErrNeedsForce marks a refusal that Options.Force overrides. Check for it with
// errors.Is.
var ErrNeedsForce = errors.New("needs --force")

// forceRefusal is a refusal that Options.Force overrides. Its text is the
// refusal alone, so the message reads as it always did.
type forceRefusal string

func (e forceRefusal) Error() string { return string(e) }

// Unwrap makes errors.Is(err, ErrNeedsForce) true.
func (e forceRefusal) Unwrap() error { return ErrNeedsForce }
