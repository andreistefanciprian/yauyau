package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const eventTypePump = "pump"

type createPumpRequest struct {
	AmountMl   int    `json:"amount_ml"`
	Notes      string `json:"notes"`
	OccurredAt string `json:"occurred_at"`
}

// pumpResponse is a pump event as returned to API consumers.
type pumpResponse struct {
	ID         uuid.UUID `json:"id"`
	BabyID     uuid.UUID `json:"baby_id"`
	AmountMl   int       `json:"amount_ml"`
	Notes      string    `json:"notes,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handlers) CreatePump(w http.ResponseWriter, r *http.Request) {
	var req createPumpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.AmountMl <= 0 {
		writeError(w, http.StatusBadRequest, "amount_ml must be a positive number")
		return
	}

	occurredAt, ok := parseOccurredAt(w, req.OccurredAt)
	if !ok {
		return
	}

	attributes := map[string]any{"amount_ml": req.AmountMl}
	if notes := strings.TrimSpace(req.Notes); notes != "" {
		attributes["notes"] = notes
	}

	createAndRespond(w, r, h, eventTypePump, attributes, occurredAt, pumpFromEvent)
}

func pumpFromEvent(ev store.Event) pumpResponse {
	resp := pumpResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if v, ok := attributeInt(ev.Attributes, "amount_ml"); ok {
		resp.AmountMl = v
	}
	if notes, ok := ev.Attributes["notes"].(string); ok {
		resp.Notes = notes
	}
	return resp
}
