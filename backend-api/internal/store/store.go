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

// NappyKind is the set of valid nappy change kinds.
type NappyKind string

const (
	NappyKindWet  NappyKind = "wet"
	NappyKindPoo  NappyKind = "poo"
	NappyKindBoth NappyKind = "both"
)

func (k NappyKind) Valid() bool {
	switch k {
	case NappyKindWet, NappyKindPoo, NappyKindBoth:
		return true
	default:
		return false
	}
}

const EventTypeNappy = "nappy"

// Baby is the hardcoded baby record as returned to API consumers.
type Baby struct {
	ID       uuid.UUID `json:"id"`
	FamilyID uuid.UUID `json:"family_id"`
	Name     string    `json:"name"`
	Timezone string    `json:"timezone"`
}

// NappyEvent is a nappy-change event as returned to API consumers.
type NappyEvent struct {
	ID         uuid.UUID `json:"id"`
	BabyID     uuid.UUID `json:"baby_id"`
	Kind       NappyKind `json:"kind"`
	Colour     string    `json:"colour,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}
