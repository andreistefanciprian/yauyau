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

// Event is a generic event exactly as backend-api's combined /events
// endpoint returns it: event_type plus its type-specific attributes, not a
// typed per-event shape. Interpreting Attributes is internal/handlers' job,
// same division of responsibility as backend-api's own store.Event.
type Event struct {
	ID         string         `json:"id"`
	BabyID     string         `json:"baby_id"`
	EventType  string         `json:"event_type"`
	Attributes map[string]any `json:"attributes"`
	OccurredAt time.Time      `json:"occurred_at"`
	CreatedAt  time.Time      `json:"created_at"`
}
