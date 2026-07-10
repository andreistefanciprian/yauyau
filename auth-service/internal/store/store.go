package store

import "errors"

// ErrNotFound is returned by Store methods when the requested record does
// not exist (or, for ConsumeMagicLink, no longer qualifies — already used
// or expired), so callers never need to know about the underlying driver's
// not-found error.
var ErrNotFound = errors.New("not found")
