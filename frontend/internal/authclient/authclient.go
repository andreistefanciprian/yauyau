// Package authclient holds auth-service's response types and the HTTP
// client that fetches them. See internal/handlers for the interface that
// consumes this package.
package authclient

import "errors"

// ErrUnauthorized is returned when auth-service rejects a request with 401
// — shared across every authclient call, since more than one auth-service
// endpoint uses 401 for its own unrelated reason (MintToken: an invalid,
// revoked, or expired session; VerifyMagicLink: an invalid or expired
// magic-link token). The message is deliberately generic rather than
// session-specific so a caller like VerifyMagicLink's — which doesn't mean
// "session" at all — doesn't get a misleading message in its logs; only
// mintAccessToken, which calls MintToken exclusively, is entitled to
// interpret this as "the session is dead".
var ErrUnauthorized = errors.New("request not authorized")

// ErrAlreadyAttached is returned by AttachFamily when the session already
// has a family (409) — a harmless double-submit of the onboarding form, not
// a real failure.
var ErrAlreadyAttached = errors.New("session already has a family")

// VerifyResult is auth-service's POST /internal/auth/verify response.
// FamilyID is a pointer so omitempty actually omits it from JSON when the
// session has no family yet — a brand-new signup that still needs
// onboarding, not an error.
type VerifyResult struct {
	SessionID string  `json:"session_id"`
	UserID    string  `json:"user_id"`
	FamilyID  *string `json:"family_id,omitempty"`
}

// MintResult is auth-service's POST /internal/auth/token response. FamilyID
// is nil until the session has completed onboarding — the frontend's
// session middleware branches on this to decide whether to route a request
// to the onboarding UI instead of the requested page.
type MintResult struct {
	AccessToken string  `json:"access_token"`
	FamilyID    *string `json:"family_id,omitempty"`
}
