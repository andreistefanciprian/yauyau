package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"yauyau/backend-api/internal/store"
)

const eventTypeBath = "bath"

// BathType is the set of valid bath types.
type BathType string

const (
	BathTypeWholeBody  BathType = "whole_body"
	BathTypeBottomPart BathType = "bottom_part"
)

func (t BathType) Valid() bool {
	switch t {
	case BathTypeWholeBody, BathTypeBottomPart:
		return true
	default:
		return false
	}
}

type createBathRequest struct {
	Type            string `json:"type"`
	Notes           string `json:"notes"`
	DurationMinutes *int   `json:"duration_minutes"`
	OccurredAt      string `json:"occurred_at"`
}

// bathResponse is a bath event as returned to API consumers.
type bathResponse struct {
	ID              uuid.UUID `json:"id"`
	BabyID          uuid.UUID `json:"baby_id"`
	Type            BathType  `json:"type"`
	Notes           string    `json:"notes,omitempty"`
	DurationMinutes *int      `json:"duration_minutes,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
	CreatedAt       time.Time `json:"created_at"`
}

func (h *Handlers) CreateBath(w http.ResponseWriter, r *http.Request) {
	var req createBathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	bathType := BathType(req.Type)
	if !bathType.Valid() {
		writeError(w, http.StatusBadRequest, "type must be one of: whole_body, bottom_part")
		return
	}

	occurredAt, err := parseOccurredAt(req.OccurredAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "occurred_at must be RFC3339 formatted")
		return
	}

	attributes := map[string]any{"type": string(bathType)}
	if req.Notes != "" {
		attributes["notes"] = req.Notes
	}
	if req.DurationMinutes != nil {
		attributes["duration_minutes"] = *req.DurationMinutes
	}

	ev, err := h.Store.CreateEvent(r.Context(), eventTypeBath, attributes, occurredAt)
	if err != nil {
		log.Printf("create bath event: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save bath event")
		return
	}

	writeJSON(w, http.StatusCreated, bathFromEvent(ev))
}

func (h *Handlers) ListBaths(w http.ResponseWriter, r *http.Request) {
	events, err := h.Store.ListEvents(r.Context(), eventTypeBath, 20)
	if err != nil {
		log.Printf("list bath events: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load bath events")
		return
	}

	baths := make([]bathResponse, len(events))
	for i, ev := range events {
		baths[i] = bathFromEvent(ev)
	}

	writeJSON(w, http.StatusOK, baths)
}

func bathFromEvent(ev store.Event) bathResponse {
	resp := bathResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if t, ok := ev.Attributes["type"].(string); ok {
		resp.Type = BathType(t)
	}
	if notes, ok := ev.Attributes["notes"].(string); ok {
		resp.Notes = notes
	}
	if v, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		resp.DurationMinutes = &v
	}
	return resp
}
