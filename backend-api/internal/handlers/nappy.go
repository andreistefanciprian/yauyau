package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const eventTypeNappy = "nappy"

// NappyKind is the set of valid nappy change kinds.
type NappyKind string

const (
	NappyKindWet  NappyKind = "wet"
	NappyKindPoo  NappyKind = "poo"
	NappyKindBoth NappyKind = "both"
)

func (k NappyKind) Valid() bool {
	switch k {
	case NappyKindWet, NappyKindPoo, NappyKindBoth:
		return true
	default:
		return false
	}
}

type createNappyRequest struct {
	Kind       string `json:"kind"`
	Notes      string `json:"notes"`
	OccurredAt string `json:"occurred_at"`
}

// nappyResponse is a nappy-change event as returned to API consumers.
type nappyResponse struct {
	ID         uuid.UUID `json:"id"`
	BabyID     uuid.UUID `json:"baby_id"`
	Kind       NappyKind `json:"kind"`
	Notes      string    `json:"notes,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handlers) CreateNappy(w http.ResponseWriter, r *http.Request) {
	var req createNappyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	kind := NappyKind(req.Kind)
	if !kind.Valid() {
		writeError(w, http.StatusBadRequest, "kind must be one of: wet, poo, both")
		return
	}

	occurredAt, ok := parseOccurredAt(w, req.OccurredAt)
	if !ok {
		return
	}

	attributes := map[string]any{"kind": string(kind)}
	if req.Notes != "" {
		attributes["notes"] = req.Notes
	}

	createAndRespond(w, r, h, eventTypeNappy, attributes, occurredAt, nappyFromEvent)
}

func nappyFromEvent(ev store.Event) nappyResponse {
	resp := nappyResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if kind, ok := ev.Attributes["kind"].(string); ok {
		resp.Kind = NappyKind(kind)
	}
	if notes, ok := ev.Attributes["notes"].(string); ok {
		resp.Notes = notes
	} else if colour, ok := ev.Attributes["colour"].(string); ok {
		resp.Notes = colour
	}
	return resp
}
