package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	// Alpine's base image has no OS tzdata; embed the IANA database in the
	// binary so time.LoadLocation works regardless of the host.
	_ "time/tzdata"

	"yauyau/frontend/internal/backendclient"
)

// dateFieldLayout and timeFieldLayout match the value formats of
// <input type="date"> and <input type="time"> respectively.
const (
	dateFieldLayout = "2006-01-02"
	timeFieldLayout = "15:04"
)

// Backend is the backend-api boundary this package needs. Defined here (the
// consumer) rather than in internal/backendclient (the producer) so it stays
// a minimal, purpose-built contract instead of growing to match whatever the
// HTTP client happens to expose. It is deliberately generic over resource
// (nappies, feeds, ...) so adding an event type never grows this interface.
type Backend interface {
	GetCurrentBaby(ctx context.Context) (backendclient.Baby, error)
	ListEvents(ctx context.Context, resource string, out any) error
	CreateEvent(ctx context.Context, resource string, payload map[string]any) error
}

type Handlers struct {
	Backend   Backend
	Templates *template.Template
}

func New(backend Backend, templates *template.Template) *Handlers {
	return &Handlers{Backend: backend, Templates: templates}
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	baby, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	var nappies []backendclient.Nappy
	if err := h.Backend.ListEvents(r.Context(), "nappies", &nappies); err != nil {
		log.Printf("list nappies: %v", err)
		http.Error(w, "failed to load nappy events", http.StatusBadGateway)
		return
	}

	var feeds []backendclient.Feed
	if err := h.Backend.ListEvents(r.Context(), "feeds", &feeds); err != nil {
		log.Printf("list feeds: %v", err)
		http.Error(w, "failed to load feed events", http.StatusBadGateway)
		return
	}

	var baths []backendclient.Bath
	if err := h.Backend.ListEvents(r.Context(), "baths", &baths); err != nil {
		log.Printf("list baths: %v", err)
		http.Error(w, "failed to load bath events", http.StatusBadGateway)
		return
	}

	var observations []backendclient.Observation
	if err := h.Backend.ListEvents(r.Context(), "observations", &observations); err != nil {
		log.Printf("list observations: %v", err)
		http.Error(w, "failed to load observation events", http.StatusBadGateway)
		return
	}

	now := time.Now().In(loc)
	data := struct {
		Baby         backendclient.Baby
		Nappies      []backendclient.Nappy
		Feeds        []backendclient.Feed
		Baths        []backendclient.Bath
		Observations []backendclient.Observation
		NowDate      string
		NowTime      string
	}{
		Baby:         baby,
		Nappies:      inLocation(nappies, loc, nappyOccurredAt),
		Feeds:        inLocation(feeds, loc, feedOccurredAt),
		Baths:        inLocation(baths, loc, bathOccurredAt),
		Observations: inLocation(observations, loc, observationOccurredAt),
		NowDate:      now.Format(dateFieldLayout),
		NowTime:      now.Format(timeFieldLayout),
	}

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

	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	occurredAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse nappy time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"kind":        r.FormValue("kind"),
		"colour":      r.FormValue("colour"),
		"occurred_at": occurredAt.Format(time.RFC3339),
	}
	if err := h.Backend.CreateEvent(r.Context(), "nappies", payload); err != nil {
		log.Printf("create nappy: %v", err)
		http.Error(w, "failed to save nappy event", http.StatusBadGateway)
		return
	}

	var nappies []backendclient.Nappy
	if err := h.Backend.ListEvents(r.Context(), "nappies", &nappies); err != nil {
		log.Printf("list nappies: %v", err)
		http.Error(w, "failed to load nappy events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "nappy_list", inLocation(nappies, loc, nappyOccurredAt)); err != nil {
		log.Printf("render nappy_list template: %v", err)
	}
}

func (h *Handlers) CreateFeed(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	occurredAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse feed time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	amountMl, err := parseOptionalInt(r.FormValue("amount_ml"))
	if err != nil {
		http.Error(w, "invalid amount", http.StatusBadRequest)
		return
	}

	durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
	if err != nil {
		http.Error(w, "invalid duration", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"type":             r.FormValue("type"),
		"amount_ml":        amountMl,
		"duration_minutes": durationMinutes,
		"occurred_at":      occurredAt.Format(time.RFC3339),
	}
	if err := h.Backend.CreateEvent(r.Context(), "feeds", payload); err != nil {
		log.Printf("create feed: %v", err)
		http.Error(w, "failed to save feed event", http.StatusBadGateway)
		return
	}

	var feeds []backendclient.Feed
	if err := h.Backend.ListEvents(r.Context(), "feeds", &feeds); err != nil {
		log.Printf("list feeds: %v", err)
		http.Error(w, "failed to load feed events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "feed_list", inLocation(feeds, loc, feedOccurredAt)); err != nil {
		log.Printf("render feed_list template: %v", err)
	}
}

func (h *Handlers) CreateBath(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	occurredAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse bath time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
	if err != nil {
		http.Error(w, "invalid duration", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"type":             r.FormValue("type"),
		"notes":            r.FormValue("notes"),
		"duration_minutes": durationMinutes,
		"occurred_at":      occurredAt.Format(time.RFC3339),
	}
	if err := h.Backend.CreateEvent(r.Context(), "baths", payload); err != nil {
		log.Printf("create bath: %v", err)
		http.Error(w, "failed to save bath event", http.StatusBadGateway)
		return
	}

	var baths []backendclient.Bath
	if err := h.Backend.ListEvents(r.Context(), "baths", &baths); err != nil {
		log.Printf("list baths: %v", err)
		http.Error(w, "failed to load bath events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "bath_list", inLocation(baths, loc, bathOccurredAt)); err != nil {
		log.Printf("render bath_list template: %v", err)
	}
}

func (h *Handlers) CreateObservation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	occurredAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse observation time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"text":        r.FormValue("text"),
		"category":    r.FormValue("category"),
		"occurred_at": occurredAt.Format(time.RFC3339),
	}
	if err := h.Backend.CreateEvent(r.Context(), "observations", payload); err != nil {
		log.Printf("create observation: %v", err)
		http.Error(w, "failed to save observation event", http.StatusBadGateway)
		return
	}

	var observations []backendclient.Observation
	if err := h.Backend.ListEvents(r.Context(), "observations", &observations); err != nil {
		log.Printf("list observations: %v", err)
		http.Error(w, "failed to load observation events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "observation_list", inLocation(observations, loc, observationOccurredAt)); err != nil {
		log.Printf("render observation_list template: %v", err)
	}
}

// currentBabyLocation fetches the current baby and resolves its IANA
// timezone in one call, since every handler that needs one needs the other.
func (h *Handlers) currentBabyLocation(ctx context.Context) (backendclient.Baby, *time.Location, error) {
	baby, err := h.Backend.GetCurrentBaby(ctx)
	if err != nil {
		return backendclient.Baby{}, nil, fmt.Errorf("get current baby: %w", err)
	}

	loc, err := time.LoadLocation(baby.Timezone)
	if err != nil {
		return backendclient.Baby{}, nil, fmt.Errorf("load baby timezone: %w", err)
	}

	return baby, loc, nil
}

// parseEventTime combines a "date" ("2006-01-02") and "time" ("15:04") form
// value in loc, so the event is recorded at the chosen date and time rather
// than the moment the request happened to hit the server.
func parseEventTime(loc *time.Location, date, hhmm string) (time.Time, error) {
	parsed, err := time.ParseInLocation(dateFieldLayout+" "+timeFieldLayout, date+" "+hhmm, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing date %q time %q: %w", date, hhmm, err)
	}
	return parsed, nil
}

// parseOptionalInt parses a form field that may be blank, returning nil
// rather than an error when it is.
func parseOptionalInt(raw string) (*int, error) {
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing int %q: %w", raw, err)
	}
	return &v, nil
}

// inLocation returns a copy of items with each element's OccurredAt (as
// located by the occurredAt accessor) converted to loc, so recent-events
// lists display the baby's local time instead of UTC. One generic helper
// replaces a hand-written copy per event type.
func inLocation[T any](items []T, loc *time.Location, occurredAt func(*T) *time.Time) []T {
	converted := make([]T, len(items))
	copy(converted, items)
	for i := range converted {
		t := occurredAt(&converted[i])
		*t = t.In(loc)
	}
	return converted
}

func nappyOccurredAt(n *backendclient.Nappy) *time.Time             { return &n.OccurredAt }
func feedOccurredAt(f *backendclient.Feed) *time.Time               { return &f.OccurredAt }
func bathOccurredAt(b *backendclient.Bath) *time.Time               { return &b.OccurredAt }
func observationOccurredAt(o *backendclient.Observation) *time.Time { return &o.OccurredAt }
