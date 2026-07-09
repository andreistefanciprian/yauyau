package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"yauyau/backend-api/internal/store"
)

const eventTypeObservation = "observation"

// defaultObservationCategory is used when category is omitted. Categories
// are suggestions only (general, behaviour, feeding, sleep, health, doctor,
// milestone) and are deliberately not validated against a fixed set — any
// non-empty string is accepted, unlike NappyKind/FeedType/BathType.
const defaultObservationCategory = "general"

type createObservationRequest struct {
	Text       string `json:"text"`
	Category   string `json:"category"`
	OccurredAt string `json:"occurred_at"`
}

// observationResponse is an observation event as returned to API consumers.
type observationResponse struct {
	ID         uuid.UUID `json:"id"`
	BabyID     uuid.UUID `json:"baby_id"`
	Text       string    `json:"text"`
	Category   string    `json:"category"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handlers) CreateObservation(w http.ResponseWriter, r *http.Request) {
	var req createObservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = defaultObservationCategory
	}

	occurredAt, err := parseOccurredAt(req.OccurredAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "occurred_at must be RFC3339 formatted")
		return
	}

	attributes := map[string]any{"text": text, "category": category}

	createAndRespond(w, r, h, eventTypeObservation, attributes, occurredAt, observationFromEvent)
}

func (h *Handlers) ListObservations(w http.ResponseWriter, r *http.Request) {
	listAndRespond(w, r, h, eventTypeObservation, observationFromEvent)
}

func observationFromEvent(ev store.Event) observationResponse {
	resp := observationResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if text, ok := ev.Attributes["text"].(string); ok {
		resp.Text = text
	}
	if category, ok := ev.Attributes["category"].(string); ok {
		resp.Category = category
	}
	return resp
}
