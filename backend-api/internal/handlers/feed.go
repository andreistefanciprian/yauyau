package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const eventTypeFeed = "feed"

// FeedType is the set of valid feed types.
type FeedType string

const (
	FeedTypeBreast    FeedType = "breast"
	FeedTypeFormula   FeedType = "formula"
	FeedTypeExpressed FeedType = "expressed"
)

func (t FeedType) Valid() bool {
	switch t {
	case FeedTypeBreast, FeedTypeFormula, FeedTypeExpressed:
		return true
	default:
		return false
	}
}

type createFeedRequest struct {
	Type            string `json:"type"`
	AmountMl        *int   `json:"amount_ml"`
	DurationMinutes *int   `json:"duration_minutes"`
	Notes           string `json:"notes"`
	OccurredAt      string `json:"occurred_at"`
}

// feedResponse is a feed event as returned to API consumers.
type feedResponse struct {
	ID              uuid.UUID `json:"id"`
	BabyID          uuid.UUID `json:"baby_id"`
	Type            FeedType  `json:"type"`
	AmountMl        *int      `json:"amount_ml,omitempty"`
	DurationMinutes *int      `json:"duration_minutes,omitempty"`
	Notes           string    `json:"notes,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
	CreatedAt       time.Time `json:"created_at"`
}

func (h *Handlers) CreateFeed(w http.ResponseWriter, r *http.Request) {
	var req createFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	feedType := FeedType(req.Type)
	if !feedType.Valid() {
		writeError(w, http.StatusBadRequest, "type must be one of: breast, formula, expressed")
		return
	}

	occurredAt, ok := parseOccurredAt(w, req.OccurredAt)
	if !ok {
		return
	}

	attributes := map[string]any{"type": string(feedType)}
	if req.AmountMl != nil {
		attributes["amount_ml"] = *req.AmountMl
	}
	if req.DurationMinutes != nil {
		attributes["duration_minutes"] = *req.DurationMinutes
	}
	if req.Notes != "" {
		attributes["notes"] = req.Notes
	}

	createAndRespond(w, r, h, eventTypeFeed, attributes, occurredAt, feedFromEvent)
}

func feedFromEvent(ev store.Event) feedResponse {
	resp := feedResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if t, ok := ev.Attributes["type"].(string); ok {
		resp.Type = FeedType(t)
	}
	if v, ok := attributeInt(ev.Attributes, "amount_ml"); ok {
		resp.AmountMl = &v
	}
	if v, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		resp.DurationMinutes = &v
	}
	if notes, ok := ev.Attributes["notes"].(string); ok {
		resp.Notes = notes
	}
	return resp
}

// attributeInt reads an int out of an events.attributes map. The value is a
// native Go int right after CreateEvent builds the map in-process, but a
// float64 once it round-trips through Postgres JSONB and back (pgx decodes
// JSON numbers as float64), so both forms have to be handled.
func attributeInt(attributes map[string]any, key string) (int, bool) {
	switch v := attributes[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}
