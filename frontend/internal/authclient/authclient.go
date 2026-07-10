// Package authclient holds auth-service's response types and the HTTP
// client that fetches them. See internal/handlers for the interface that
// consumes this package.
package authclient

// VerifyResult is auth-service's POST /internal/auth/verify response.
// FamilyID is a pointer so omitempty actually omits it from JSON when the
// session has no family yet — a brand-new signup that still needs
// onboarding, not an error.
type VerifyResult struct {
	SessionID string  `json:"session_id"`
	UserID    string  `json:"user_id"`
	FamilyID  *string `json:"family_id,omitempty"`
}
