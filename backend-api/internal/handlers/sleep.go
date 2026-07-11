package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const eventTypeSleep = "sleep"

// SleepType is the set of valid sleep types.
type SleepType string

const (
	SleepTypeNap   SleepType = "nap"
	SleepTypeNight SleepType = "night"
)

func (t SleepType) Valid() bool {
	switch t {
	case SleepTypeNap, SleepTypeNight:
		return true
	default:
		return false
	}
}

type createSleepRequest struct {
	Type            string `json:"type"`
	Notes           string `json:"notes"`
	DurationMinutes *int   `json:"duration_minutes"`
	OccurredAt      string `json:"occurred_at"`
}

// sleepResponse is a sleep event as returned to API consumers.
type sleepResponse struct {
	ID              uuid.UUID `json:"id"`
	BabyID          uuid.UUID `json:"baby_id"`
	Type            SleepType `json:"type"`
	Notes           string    `json:"notes,omitempty"`
	DurationMinutes *int      `json:"duration_minutes,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
	CreatedAt       time.Time `json:"created_at"`
}

func (h *Handlers) CreateSleep(w http.ResponseWriter, r *http.Request) {
	var req createSleepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	sleepType := SleepType(req.Type)
	if !sleepType.Valid() {
		writeError(w, http.StatusBadRequest, "type must be one of: nap, night")
		return
	}

	occurredAt, ok := parseOccurredAt(w, req.OccurredAt)
	if !ok {
		return
	}

	attributes := map[string]any{"type": string(sleepType)}
	if req.Notes != "" {
		attributes["notes"] = req.Notes
	}
	if req.DurationMinutes != nil {
		attributes["duration_minutes"] = *req.DurationMinutes
	}

	createAndRespond(w, r, h, eventTypeSleep, attributes, occurredAt, sleepFromEvent)
}

func sleepFromEvent(ev store.Event) sleepResponse {
	resp := sleepResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if t, ok := ev.Attributes["type"].(string); ok {
		resp.Type = SleepType(t)
	}
	if notes, ok := ev.Attributes["notes"].(string); ok {
		resp.Notes = notes
	}
	if v, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		resp.DurationMinutes = &v
	}
	return resp
}
