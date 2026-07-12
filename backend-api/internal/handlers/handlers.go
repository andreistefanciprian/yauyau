package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	// backend-api runs from a scratch image with no OS timezone database.
	// Embed IANA tzdata so calendar timeline ranges can use baby.Timezone.
	_ "time/tzdata"

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
	UpdateBaby(ctx context.Context, familyID, babyID uuid.UUID, baby store.Baby) (store.Baby, error)
	ArchiveBaby(ctx context.Context, familyID, babyID uuid.UUID) error
	CreateEvent(ctx context.Context, familyID, babyID uuid.UUID, eventType string, attributes map[string]any, occurredAt time.Time) (store.Event, error)
	UpdateEvent(ctx context.Context, familyID, babyID, id uuid.UUID, eventType string, attributes map[string]any, occurredAt time.Time) (store.Event, error)
	ListAllEvents(ctx context.Context, familyID, babyID uuid.UUID, from, to time.Time, limit int) ([]store.Event, error)
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
	HasPendingInviteOutsideFamily(ctx context.Context, userID, excludeFamilyID uuid.UUID) (bool, error)
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

const defaultTimelineRange = "today"

// timelineRangeDays is how many days back the timeline nav reaches: today
// plus the six days before it, so a full week is selectable one day at a
// time.
const timelineRangeDays = 7

var errUnsupportedTimelineRange = errors.New("unsupported timeline range")

type timelineRangeWindow struct {
	From time.Time
	To   time.Time
}

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

type updateEventRequest struct {
	EventType  string         `json:"event_type"`
	Attributes map[string]any `json:"attributes"`
	OccurredAt string         `json:"occurred_at"`
}

// ListAllEvents returns the most recent events across every event type,
// merged and ordered newest-first by the store, for a single combined
// timeline instead of one list call per event type.
func (h *Handlers) ListAllEvents(w http.ResponseWriter, r *http.Request) {
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	window, err := timelineRangeFor(r.URL.Query().Get("range"), baby.Timezone)
	if err != nil {
		if errors.Is(err, errUnsupportedTimelineRange) {
			writeError(w, http.StatusBadRequest, "invalid timeline range")
			return
		}
		log.Printf("resolve timeline range: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve timeline range")
		return
	}

	events, err := h.Store.ListAllEvents(r.Context(), baby.FamilyID, baby.ID, window.From, window.To, allEventsLimit)
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

func timelineRangeFor(rawRange, timezone string) (timelineRangeWindow, error) {
	rangeKey := rawRange
	if rangeKey == "" {
		rangeKey = defaultTimelineRange
	}
	offset, ok := timelineRangeOffset(rangeKey)
	if !ok {
		return timelineRangeWindow{}, fmt.Errorf("%w: %s", errUnsupportedTimelineRange, rawRange)
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return timelineRangeWindow{}, fmt.Errorf("load baby timezone %q: %w", timezone, err)
	}

	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dayStart := todayStart.AddDate(0, 0, -offset)
	return timelineRangeWindow{From: dayStart, To: dayStart.AddDate(0, 0, 1)}, nil
}

// timelineRangeOffset maps a range key to how many days before today it
// selects: "today" and "yesterday" are 0 and 1, and earlier days use
// "day-N" (2 through timelineRangeDays-1) since the frontend labels them
// with the weekday name rather than a fixed word.
func timelineRangeOffset(rangeKey string) (int, bool) {
	switch rangeKey {
	case "today":
		return 0, true
	case "yesterday":
		return 1, true
	}
	suffix, ok := strings.CutPrefix(rangeKey, "day-")
	if !ok {
		return 0, false
	}
	offset, err := strconv.Atoi(suffix)
	if err != nil || offset < 2 || offset >= timelineRangeDays {
		return 0, false
	}
	return offset, true
}

// UpdateEvent edits a single event by id, regardless of its event_type.
func (h *Handlers) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var req updateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	attributes, ok := normalizeEventAttributes(w, req.EventType, req.Attributes)
	if !ok {
		return
	}

	occurredAt, ok := parseOccurredAt(w, req.OccurredAt)
	if !ok {
		return
	}

	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	ev, err := h.Store.UpdateEvent(r.Context(), baby.FamilyID, baby.ID, id, req.EventType, attributes, occurredAt)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if err != nil {
		log.Printf("update event: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update event")
		return
	}

	writeJSON(w, http.StatusOK, eventResponse{
		ID:         ev.ID,
		BabyID:     ev.BabyID,
		EventType:  ev.EventType,
		Attributes: ev.Attributes,
		OccurredAt: ev.OccurredAt,
		CreatedAt:  ev.CreatedAt,
	})
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

// futureToleranceForOccurredAt absorbs clock skew between the client that
// picked "now" and this server, plus the time form field's minute-only
// granularity (which can round up to 59s ahead of the true instant) — without
// this, a legitimate "log this right now" submission can be rejected as
// future-dated for reasons that have nothing to do with the user's input.
const futureToleranceForOccurredAt = 5 * time.Minute

// parseOccurredAt parses an optional RFC3339 "occurred_at" from a request
// body, defaulting to the current server time when raw is empty. Writes the
// appropriate 400 response and returns ok=false if it's malformed or too far
// in the future — events record things that have already happened.
func parseOccurredAt(w http.ResponseWriter, raw string) (time.Time, bool) {
	if raw == "" {
		return time.Now(), true
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "occurred_at must be RFC3339 formatted")
		return time.Time{}, false
	}
	if parsed.After(time.Now().Add(futureToleranceForOccurredAt)) {
		writeError(w, http.StatusBadRequest, "occurred_at cannot be in the future")
		return time.Time{}, false
	}
	return parsed, true
}

func normalizeEventAttributes(w http.ResponseWriter, eventType string, raw map[string]any) (map[string]any, bool) {
	switch eventType {
	case eventTypeNappy:
		kind := NappyKind(attributeString(raw, "kind"))
		if !kind.Valid() {
			writeError(w, http.StatusBadRequest, "kind must be one of: wet, poo, both")
			return nil, false
		}
		attributes := map[string]any{"kind": string(kind)}
		if kind == NappyKindPoo || kind == NappyKindBoth {
			pooSize := PooSizeMedium
			if rawPooSize := attributeString(raw, "poo_size"); rawPooSize != "" {
				pooSize = PooSize(rawPooSize)
			}
			if !pooSize.Valid() {
				writeError(w, http.StatusBadRequest, "poo_size must be one of: smear, small, medium, large, blowout")
				return nil, false
			}
			attributes["poo_size"] = string(pooSize)
		}
		if rawLabels, ok := raw["labels"]; ok {
			labels, ok := nappyLabelsFromAttribute(rawLabels)
			if !ok {
				writeError(w, http.StatusBadRequest, "labels include an unsupported nappy label")
				return nil, false
			}
			if len(labels) > 0 {
				attributes["labels"] = labels
			}
		}
		if notes := strings.TrimSpace(attributeString(raw, "notes")); notes != "" {
			attributes["notes"] = notes
		} else if colour := strings.TrimSpace(attributeString(raw, "colour")); colour != "" {
			attributes["notes"] = colour
		}
		return attributes, true
	case eventTypeFeed:
		feedType := FeedType(attributeString(raw, "type"))
		if !feedType.Valid() {
			writeError(w, http.StatusBadRequest, "type must be one of: breast, formula, expressed")
			return nil, false
		}
		attributes := map[string]any{"type": string(feedType)}
		if amountMl, ok := attributeOptionalInt(raw, "amount_ml"); ok {
			attributes["amount_ml"] = amountMl
		}
		if durationMinutes, ok := attributeOptionalInt(raw, "duration_minutes"); ok {
			attributes["duration_minutes"] = durationMinutes
		}
		if notes := strings.TrimSpace(attributeString(raw, "notes")); notes != "" {
			attributes["notes"] = notes
		}
		return attributes, true
	case eventTypePump:
		amountMl, ok := attributeRequiredPositiveInt(raw, "amount_ml")
		if !ok {
			writeError(w, http.StatusBadRequest, "amount_ml must be a positive number")
			return nil, false
		}
		attributes := map[string]any{"amount_ml": amountMl}
		if notes := strings.TrimSpace(attributeString(raw, "notes")); notes != "" {
			attributes["notes"] = notes
		}
		return attributes, true
	case eventTypeBath:
		bathType := BathType(attributeString(raw, "type"))
		if !bathType.Valid() {
			writeError(w, http.StatusBadRequest, "type must be one of: whole_body, bottom_part")
			return nil, false
		}
		attributes := map[string]any{"type": string(bathType)}
		if notes := strings.TrimSpace(attributeString(raw, "notes")); notes != "" {
			attributes["notes"] = notes
		}
		if durationMinutes, ok := attributeOptionalInt(raw, "duration_minutes"); ok {
			attributes["duration_minutes"] = durationMinutes
		}
		return attributes, true
	case eventTypeSleep:
		sleepType := SleepType(attributeString(raw, "type"))
		if !sleepType.Valid() {
			writeError(w, http.StatusBadRequest, "type must be one of: nap, night")
			return nil, false
		}
		attributes := map[string]any{"type": string(sleepType)}
		if notes := strings.TrimSpace(attributeString(raw, "notes")); notes != "" {
			attributes["notes"] = notes
		}
		if durationMinutes, ok := attributeOptionalInt(raw, "duration_minutes"); ok {
			attributes["duration_minutes"] = durationMinutes
		}
		return attributes, true
	case eventTypeObservation:
		text := strings.TrimSpace(attributeString(raw, "text"))
		if text == "" {
			writeError(w, http.StatusBadRequest, "text is required")
			return nil, false
		}
		category := strings.TrimSpace(attributeString(raw, "category"))
		if category == "" {
			category = defaultObservationCategory
		}
		return map[string]any{"text": text, "category": category}, true
	default:
		writeError(w, http.StatusBadRequest, "event_type is invalid")
		return nil, false
	}
}

func attributeString(attributes map[string]any, key string) string {
	if attributes == nil {
		return ""
	}
	value, _ := attributes[key].(string)
	return value
}

func attributeOptionalInt(attributes map[string]any, key string) (int, bool) {
	if attributes == nil {
		return 0, false
	}
	switch value := attributes[key].(type) {
	case float64:
		return int(value), true
	case int:
		return value, true
	default:
		return 0, false
	}
}

func attributeRequiredPositiveInt(attributes map[string]any, key string) (int, bool) {
	value, ok := attributeOptionalInt(attributes, key)
	return value, ok && value > 0
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
