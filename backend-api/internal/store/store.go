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
