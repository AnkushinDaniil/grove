package core

import "errors"

// ErrInvalid is the sentinel wrapped by all domain validation failures.
// Callers branch with errors.Is(err, ErrInvalid).
var ErrInvalid = errors.New("invalid")
