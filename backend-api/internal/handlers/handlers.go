package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"yauyau/backend-api/internal/store"
)

// Store is the persistence boundary this package needs. Defined here (the
// consumer) rather than in internal/store (the producer) so it stays a
// minimal, purpose-built contract instead of growing to match whatever the
// Postgres implementation happens to expose.
type Store interface {
	GetCurrentBaby(ctx context.Context) (store.Baby, error)
	CreateNappy(ctx context.Context, kind store.NappyKind, colour string, occurredAt time.Time) (store.NappyEvent, error)
	ListRecentNappies(ctx context.Context, limit int) ([]store.NappyEvent, error)
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

type createNappyRequest struct {
	Kind       string `json:"kind"`
	Colour     string `json:"colour"`
	OccurredAt string `json:"occurred_at"`
}

func (h *Handlers) CreateNappy(w http.ResponseWriter, r *http.Request) {
	var req createNappyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	kind := store.NappyKind(req.Kind)
	if !kind.Valid() {
		writeError(w, http.StatusBadRequest, "kind must be one of: wet, poo, both")
		return
	}

	occurredAt := time.Now()
	if req.OccurredAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.OccurredAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "occurred_at must be RFC3339 formatted")
			return
		}
		occurredAt = parsed
	}

	nappy, err := h.Store.CreateNappy(r.Context(), kind, req.Colour, occurredAt)
	if err != nil {
		log.Printf("create nappy event: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save nappy event")
		return
	}

	writeJSON(w, http.StatusCreated, nappy)
}

func (h *Handlers) ListNappies(w http.ResponseWriter, r *http.Request) {
	nappies, err := h.Store.ListRecentNappies(r.Context(), 20)
	if err != nil {
		log.Printf("list nappy events: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load nappy events")
		return
	}

	if nappies == nil {
		nappies = []store.NappyEvent{}
	}

	writeJSON(w, http.StatusOK, nappies)
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
