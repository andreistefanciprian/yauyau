package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"yauyau/backend-api/internal/store"
)

// Store is the persistence boundary this package needs. Defined here (the
// consumer) rather than in internal/store (the producer) so it stays a
// minimal, purpose-built contract instead of growing to match whatever the
// Postgres implementation happens to expose. It is deliberately generic over
// event type (nappy, feed, ...); interpreting Attributes is this package's
// job, not the store's.
type Store interface {
	GetCurrentBaby(ctx context.Context) (store.Baby, error)
	CreateEvent(ctx context.Context, eventType string, attributes map[string]any, occurredAt time.Time) (store.Event, error)
	ListEvents(ctx context.Context, eventType string, limit int) ([]store.Event, error)
}

type Handlers struct {
	Store Store
}

func New(s Store) *Handlers {
	return &Handlers{Store: s}
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) GetCurrentBaby(w http.ResponseWriter, r *http.Request) {
	baby, err := h.Store.GetCurrentBaby(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "baby not found")
		return
	}
	if err != nil {
		log.Printf("get current baby: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load baby")
		return
	}

	writeJSON(w, http.StatusOK, baby)
}

// parseOccurredAt parses an optional RFC3339 timestamp from a request body,
// defaulting to the current server time when raw is empty.
func parseOccurredAt(raw string) (time.Time, error) {
	if raw == "" {
		return time.Now(), nil
	}
	return time.Parse(time.RFC3339, raw)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
