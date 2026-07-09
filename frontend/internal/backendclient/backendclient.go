// Package backendclient holds the backend-api response types and the HTTP
// client that fetches them. See internal/handlers for the interface that
// consumes this package.
package backendclient

import "time"

type Baby struct {
	ID       string `json:"id"`
	FamilyID string `json:"family_id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

type Nappy struct {
	ID         string    `json:"id"`
	BabyID     string    `json:"baby_id"`
	Kind       string    `json:"kind"`
	Colour     string    `json:"colour"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type Feed struct {
	ID              string    `json:"id"`
	BabyID          string    `json:"baby_id"`
	Type            string    `json:"type"`
	AmountMl        *int      `json:"amount_ml,omitempty"`
	DurationMinutes *int      `json:"duration_minutes,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// HasAmount and Amount let templates render AmountMl without printing a raw
// pointer (Go's text/template prints *int as a hex address, not its value).
func (f Feed) HasAmount() bool { return f.AmountMl != nil }
func (f Feed) Amount() int {
	if f.AmountMl == nil {
		return 0
	}
	return *f.AmountMl
}

// HasDuration and Duration are Amount's counterpart for DurationMinutes.
func (f Feed) HasDuration() bool { return f.DurationMinutes != nil }
func (f Feed) Duration() int {
	if f.DurationMinutes == nil {
		return 0
	}
	return *f.DurationMinutes
}

type Bath struct {
	ID              string    `json:"id"`
	BabyID          string    `json:"baby_id"`
	Type            string    `json:"type"`
	Notes           string    `json:"notes"`
	DurationMinutes *int      `json:"duration_minutes,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
	CreatedAt       time.Time `json:"created_at"`
}
