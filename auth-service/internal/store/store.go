package store

import "errors"

// ErrNotFound is returned by Store methods when the requested record does
// not exist (or, for ConsumeMagicLink, no longer qualifies — already used
// or expired), so callers never need to know about the underlying driver's
// not-found error.
var ErrNotFound = errors.New("not found")

// ErrAlreadyAttached is returned by AttachFamily when the session exists
// but already has a family_id set. Kept distinct from ErrNotFound so a
// caller (or an operator reading logs) can tell a harmless retry collision
// — the previous attach-family call actually succeeded — apart from a real
// bug, like a garbage or expired session id.
var ErrAlreadyAttached = errors.New("session already has a family")
