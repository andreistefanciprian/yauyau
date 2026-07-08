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
