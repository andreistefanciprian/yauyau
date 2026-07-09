package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
	ListAllEvents(ctx context.Context, limit int) ([]store.Event, error)
	DeleteEvent(ctx context.Context, id uuid.UUID) error
}

// allEventsLimit caps the combined /events endpoint. It's set higher than
// each per-type List<X> endpoint's limit (20) since this one is shared
// across every event type rather than counted per type.
const allEventsLimit = 40

// eventResponse is a generic event exactly as stored (event_type +
// attributes, not a typed per-event shape), for consumers that need every
// event type ordered together by occurred_at rather than filtered to one.
type eventResponse struct {
	ID         uuid.UUID      `json:"id"`
	BabyID     uuid.UUID      `json:"baby_id"`
	EventType  string         `json:"event_type"`
	Attributes map[string]any `json:"attributes"`
	OccurredAt time.Time      `json:"occurred_at"`
	CreatedAt  time.Time      `json:"created_at"`
}

// ListAllEvents returns the most recent events across every event type,
// merged and ordered newest-first by the store, for a single combined
// timeline instead of one list call per event type.
func (h *Handlers) ListAllEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.Store.ListAllEvents(r.Context(), allEventsLimit)
	if err != nil {
		log.Printf("list all events: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load events")
		return
	}

	mapped := make([]eventResponse, len(events))
	for i, ev := range events {
		mapped[i] = eventResponse{
			ID:         ev.ID,
			BabyID:     ev.BabyID,
			EventType:  ev.EventType,
			Attributes: ev.Attributes,
			OccurredAt: ev.OccurredAt,
			CreatedAt:  ev.CreatedAt,
		}
	}

	writeJSON(w, http.StatusOK, mapped)
}

// DeleteEvent removes a single event by id, regardless of its event_type.
func (h *Handlers) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	if err := h.Store.DeleteEvent(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		log.Printf("delete event: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete event")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

// createAndRespond is the shared tail of every Create<X> handler: persist
// attributes as an event of eventType via the generic store, then respond
// with the caller's typed view of it. Decoding the request, validating it,
// and building attributes stays in each event-specific file since that part
// genuinely differs per event type.
func createAndRespond[T any](w http.ResponseWriter, r *http.Request, h *Handlers, eventType string, attributes map[string]any, occurredAt time.Time, fromEvent func(store.Event) T) {
	ev, err := h.Store.CreateEvent(r.Context(), eventType, attributes, occurredAt)
	if err != nil {
		log.Printf("create %s event: %v", eventType, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save %s event", eventType))
		return
	}

	writeJSON(w, http.StatusCreated, fromEvent(ev))
}

// listAndRespond is the shared tail of every List<X> handler.
func listAndRespond[T any](w http.ResponseWriter, r *http.Request, h *Handlers, eventType string, fromEvent func(store.Event) T) {
	events, err := h.Store.ListEvents(r.Context(), eventType, 20)
	if err != nil {
		log.Printf("list %s events: %v", eventType, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load %s events", eventType))
		return
	}

	mapped := make([]T, len(events))
	for i, ev := range events {
		mapped[i] = fromEvent(ev)
	}

	writeJSON(w, http.StatusOK, mapped)
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
