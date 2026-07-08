package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	// Alpine's base image has no OS tzdata; embed the IANA database in the
	// binary so time.LoadLocation works regardless of the host.
	_ "time/tzdata"

	"yauyau/frontend/internal/backendclient"
)

const timeFieldLayout = "15:04"

// Backend is the backend-api boundary this package needs. Defined here (the
// consumer) rather than in internal/backendclient (the producer) so it stays
// a minimal, purpose-built contract instead of growing to match whatever the
// HTTP client happens to expose.
type Backend interface {
	GetCurrentBaby(ctx context.Context) (backendclient.Baby, error)
	ListNappies(ctx context.Context) ([]backendclient.Nappy, error)
	CreateNappy(ctx context.Context, kind, colour string, occurredAt time.Time) error
}

type Handlers struct {
	Backend   Backend
	Templates *template.Template
}

func New(backend Backend, templates *template.Template) *Handlers {
	return &Handlers{Backend: backend, Templates: templates}
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	baby, err := h.Backend.GetCurrentBaby(r.Context())
	if err != nil {
		log.Printf("get current baby: %v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	loc, err := time.LoadLocation(baby.Timezone)
	if err != nil {
		log.Printf("load baby timezone: %v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	nappies, err := h.Backend.ListNappies(r.Context())
	if err != nil {
		log.Printf("list nappies: %v", err)
		http.Error(w, "failed to load nappy events", http.StatusBadGateway)
		return
	}

	data := struct {
		Baby    backendclient.Baby
		Nappies []backendclient.Nappy
		NowTime string
	}{Baby: baby, Nappies: inLocation(nappies, loc), NowTime: time.Now().In(loc).Format(timeFieldLayout)}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "index", data); err != nil {
		log.Printf("render index template: %v", err)
	}
}

func (h *Handlers) CreateNappy(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	baby, err := h.Backend.GetCurrentBaby(r.Context())
	if err != nil {
		log.Printf("get current baby: %v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	loc, err := time.LoadLocation(baby.Timezone)
	if err != nil {
		log.Printf("load baby timezone: %v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	occurredAt, err := todayAt(loc, r.FormValue("time"))
	if err != nil {
		log.Printf("parse nappy time: %v", err)
		http.Error(w, "invalid time", http.StatusBadRequest)
		return
	}

	if err := h.Backend.CreateNappy(r.Context(), r.FormValue("kind"), r.FormValue("colour"), occurredAt); err != nil {
		log.Printf("create nappy: %v", err)
		http.Error(w, "failed to save nappy event", http.StatusBadGateway)
		return
	}

	nappies, err := h.Backend.ListNappies(r.Context())
	if err != nil {
		log.Printf("list nappies: %v", err)
		http.Error(w, "failed to load nappy events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "nappy_list", inLocation(nappies, loc)); err != nil {
		log.Printf("render nappy_list template: %v", err)
	}
}

// todayAt combines an "HH:MM" form value with today's date in loc, so the
// nappy event is recorded at the chosen hour rather than the moment the
// request happened to hit the server.
func todayAt(loc *time.Location, hhmm string) (time.Time, error) {
	parsed, err := time.ParseInLocation(timeFieldLayout, hhmm, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing time %q: %w", hhmm, err)
	}

	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc), nil
}

// inLocation returns a copy of nappies with OccurredAt converted to loc, so
// the recent-events list displays the baby's local time instead of UTC.
func inLocation(nappies []backendclient.Nappy, loc *time.Location) []backendclient.Nappy {
	converted := make([]backendclient.Nappy, len(nappies))
	for i, n := range nappies {
		n.OccurredAt = n.OccurredAt.In(loc)
		converted[i] = n
	}
	return converted
}
