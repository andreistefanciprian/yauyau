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

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

// Store is the persistence boundary this package needs. Defined here (the
// consumer) rather than in internal/store (the producer) so it stays a
// minimal, purpose-built contract instead of growing to match whatever the
// Postgres implementation happens to expose. It is deliberately generic over
// event type (nappy, feed, ...); interpreting Attributes is this package's
// job, not the store's.
type Store interface {
	GetBaby(ctx context.Context, id uuid.UUID) (store.Baby, error)
	GetCurrentBaby(ctx context.Context, familyID uuid.UUID) (store.Baby, error)
	CreateBaby(ctx context.Context, familyID uuid.UUID, name, timezone string) (store.Baby, error)
	CreateEvent(ctx context.Context, familyID, babyID uuid.UUID, eventType string, attributes map[string]any, occurredAt time.Time) (store.Event, error)
	ListAllEvents(ctx context.Context, familyID, babyID uuid.UUID, limit int) ([]store.Event, error)
	DeleteEvent(ctx context.Context, familyID, babyID, id uuid.UUID) error
}

// FamilyStore is the persistence boundary the internal, auth-service-facing
// API (internal.go) needs. Kept separate from Store — a different domain
// (users/family-membership vs babies/events) with no overlapping callers —
// rather than one interface spanning both.
type FamilyStore interface {
	UpsertUserByEmail(ctx context.Context, email string) (store.User, error)
	GetFamilyMembership(ctx context.Context, userID uuid.UUID) (store.FamilyMembership, error)
	GetFamilyMembershipForFamily(ctx context.Context, userID, familyID uuid.UUID) (store.FamilyMembership, error)
	CreateFamilyWithOwner(ctx context.Context, userID uuid.UUID, familyName string) (uuid.UUID, error)
	ActivateInvitedMembership(ctx context.Context, userID, familyID uuid.UUID) error
	CreateInvite(ctx context.Context, familyID uuid.UUID, email string) error
	ListTimelineMembers(ctx context.Context, familyID uuid.UUID) ([]store.TimelineMember, error)
	UpdateTimelineMemberRelationship(ctx context.Context, familyID, userID uuid.UUID, relationship string) error
	RemoveTimelineMember(ctx context.Context, familyID, userID uuid.UUID) error
}

// AuthClient is the narrow auth-service boundary backend-api needs for
// membership changes that must revoke durable sessions.
type AuthClient interface {
	RevokeFamilyMemberSessions(ctx context.Context, userID, familyID uuid.UUID) error
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
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	events, err := h.Store.ListAllEvents(r.Context(), baby.FamilyID, baby.ID, allEventsLimit)
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

	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	if err := h.Store.DeleteEvent(r.Context(), baby.FamilyID, baby.ID, id); err != nil {
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
	Store       Store
	FamilyStore FamilyStore
	Auth        AuthClient
}

// New wires up Handlers from a single concrete store that satisfies both
// Store and FamilyStore (as *store.PostgresStore does), plus the narrow
// auth-service client needed for membership/session coordination.
func New(s interface {
	Store
	FamilyStore
}, auth AuthClient) *Handlers {
	return &Handlers{Store: s, FamilyStore: s, Auth: auth}
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// parseOccurredAt parses an optional RFC3339 timestamp from a request body,
// defaulting to the current server time when raw is empty.
func parseOccurredAt(raw string) (time.Time, error) {
	if raw == "" {
		return time.Now(), nil
	}
	return time.Parse(time.RFC3339, raw)
}

// currentBabyForRequest resolves the authenticated caller's current baby.
// The event routes are exposed as /babies/current/... rather than
// /babies/{id}/..., so this is the single place they translate a verified
// family_id claim into the concrete baby_id used by the events table.
func (h *Handlers) currentBabyForRequest(w http.ResponseWriter, r *http.Request) (store.Baby, bool) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return store.Baby{}, false
	}
	if claims.FamilyID == nil {
		writeError(w, http.StatusNotFound, "baby not found")
		return store.Baby{}, false
	}

	baby, err := h.Store.GetCurrentBaby(r.Context(), *claims.FamilyID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "baby not found")
		return store.Baby{}, false
	}
	if err != nil {
		log.Printf("get current baby for event route: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load baby")
		return store.Baby{}, false
	}

	return baby, true
}

// createAndRespond is the shared tail of every Create<X> handler: persist
// attributes as an event of eventType via the generic store, then respond
// with the caller's typed view of it. Decoding the request, validating it,
// and building attributes stays in each event-specific file since that part
// genuinely differs per event type.
func createAndRespond[T any](w http.ResponseWriter, r *http.Request, h *Handlers, eventType string, attributes map[string]any, occurredAt time.Time, fromEvent func(store.Event) T) {
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	ev, err := h.Store.CreateEvent(r.Context(), baby.FamilyID, baby.ID, eventType, attributes, occurredAt)
	if err != nil {
		log.Printf("create %s event: %v", eventType, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save %s event", eventType))
		return
	}

	writeJSON(w, http.StatusCreated, fromEvent(ev))
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
