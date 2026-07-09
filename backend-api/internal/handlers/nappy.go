package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"yauyau/backend-api/internal/store"
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
	Colour     string `json:"colour"`
	OccurredAt string `json:"occurred_at"`
}

// nappyResponse is a nappy-change event as returned to API consumers.
type nappyResponse struct {
	ID         uuid.UUID `json:"id"`
	BabyID     uuid.UUID `json:"baby_id"`
	Kind       NappyKind `json:"kind"`
	Colour     string    `json:"colour,omitempty"`
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

	occurredAt, err := parseOccurredAt(req.OccurredAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "occurred_at must be RFC3339 formatted")
		return
	}

	attributes := map[string]any{"kind": string(kind)}
	if req.Colour != "" {
		attributes["colour"] = req.Colour
	}

	createAndRespond(w, r, h, eventTypeNappy, attributes, occurredAt, nappyFromEvent)
}

func (h *Handlers) ListNappies(w http.ResponseWriter, r *http.Request) {
	listAndRespond(w, r, h, eventTypeNappy, nappyFromEvent)
}

func nappyFromEvent(ev store.Event) nappyResponse {
	resp := nappyResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if kind, ok := ev.Attributes["kind"].(string); ok {
		resp.Kind = NappyKind(kind)
	}
	if colour, ok := ev.Attributes["colour"].(string); ok {
		resp.Colour = colour
	}
	return resp
}
