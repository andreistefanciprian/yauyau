package store

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned by Store methods when the requested record does
// not exist, so callers never need to know about the underlying driver's
// not-found error.
var ErrNotFound = errors.New("not found")

// ErrActiveMembershipExists is returned by CreateFamilyWithOwner when userID
// already has an active family membership — either a genuine second call, or
// the losing side of two concurrent "create my first family" calls for the
// same brand-new user (e.g. a double-submitted onboarding form). Callers can
// re-fetch the membership instead of treating this as a hard failure.
var ErrActiveMembershipExists = errors.New("user already has an active family membership")

// Baby is a baby record as returned to API consumers.
type Baby struct {
	ID       uuid.UUID `json:"id"`
	FamilyID uuid.UUID `json:"family_id"`
	Name     string    `json:"name"`
	Timezone string    `json:"timezone"`
}

// Event is a generic append-only event: nappy, feed, sleep, etc. all live in
// the same table, distinguished by EventType, with type-specific fields kept
// in Attributes. Interpreting Attributes is the handlers package's job, not
// this package's — store stays domain-agnostic.
type Event struct {
	ID         uuid.UUID      `json:"id"`
	BabyID     uuid.UUID      `json:"baby_id"`
	EventType  string         `json:"event_type"`
	OccurredAt time.Time      `json:"occurred_at"`
	CreatedAt  time.Time      `json:"created_at"`
	Attributes map[string]any `json:"attributes"`
}

// User is a person who can log in via magic link. Email is the only
// identity a user has — there is no password.
type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// MembershipRole and MembershipStatus are the fixed sets of values a
// family_members row can hold. Validated in Go rather than a DB CHECK
// constraint, consistent with how event-attribute enums (e.g. NappyKind)
// are handled elsewhere in this codebase.
type MembershipRole string

const (
	MembershipRoleOwner  MembershipRole = "owner"
	MembershipRoleMember MembershipRole = "member"
)

type MembershipStatus string

const (
	MembershipStatusInvited MembershipStatus = "invited"
	MembershipStatusActive  MembershipStatus = "active"
)

// FamilyMembership describes a user's relationship to a family, if any.
// Found is false when the user has no family_members row at all yet (a
// brand-new signup with no family created and no pending invite). FamilyID
// is a pointer (rather than uuid.UUID directly) so that omitempty actually
// omits it from JSON when Found is false — encoding/json's omitempty never
// treats a fixed-size array type like uuid.UUID as empty, but it does treat
// a nil pointer as empty.
type FamilyMembership struct {
	Found    bool             `json:"found"`
	FamilyID *uuid.UUID       `json:"family_id,omitempty"`
	Role     MembershipRole   `json:"role,omitempty"`
	Status   MembershipStatus `json:"status,omitempty"`
}
