package store

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Hardcoded family/baby for this first vertical slice. No multi-tenancy yet.
var (
	FamilyID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	BabyID   = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

// ErrNotFound is returned by Store methods when the requested record does
// not exist, so callers never need to know about the underlying driver's
// not-found error.
var ErrNotFound = errors.New("not found")

// Baby is the hardcoded baby record as returned to API consumers.
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
// brand-new signup with no family created and no pending invite).
type FamilyMembership struct {
	Found    bool
	FamilyID uuid.UUID
	Role     MembershipRole
	Status   MembershipStatus
}
