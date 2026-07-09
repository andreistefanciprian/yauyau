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
// HTTP client happens to expose.
type Backend interface {
	GetCurrentBaby(ctx context.Context) (backendclient.Baby, error)
	ListNappies(ctx context.Context) ([]backendclient.Nappy, error)
	CreateNappy(ctx context.Context, kind, colour string, occurredAt time.Time) error
	ListFeeds(ctx context.Context) ([]backendclient.Feed, error)
	CreateFeed(ctx context.Context, feedType string, amountMl, durationMinutes *int, occurredAt time.Time) error
	ListBaths(ctx context.Context) ([]backendclient.Bath, error)
	CreateBath(ctx context.Context, bathType, notes string, durationMinutes *int, occurredAt time.Time) error
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

	nappies, err := h.Backend.ListNappies(r.Context())
	if err != nil {
		log.Printf("list nappies: %v", err)
		http.Error(w, "failed to load nappy events", http.StatusBadGateway)
		return
	}

	feeds, err := h.Backend.ListFeeds(r.Context())
	if err != nil {
		log.Printf("list feeds: %v", err)
		http.Error(w, "failed to load feed events", http.StatusBadGateway)
		return
	}

	baths, err := h.Backend.ListBaths(r.Context())
	if err != nil {
		log.Printf("list baths: %v", err)
		http.Error(w, "failed to load bath events", http.StatusBadGateway)
		return
	}

	now := time.Now().In(loc)
	data := struct {
		Baby    backendclient.Baby
		Nappies []backendclient.Nappy
		Feeds   []backendclient.Feed
		Baths   []backendclient.Bath
		NowDate string
		NowTime string
	}{
		Baby:    baby,
		Nappies: inLocation(nappies, loc),
		Feeds:   feedsInLocation(feeds, loc),
		Baths:   bathsInLocation(baths, loc),
		NowDate: now.Format(dateFieldLayout),
		NowTime: now.Format(timeFieldLayout),
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

	if err := h.Backend.CreateFeed(r.Context(), r.FormValue("type"), amountMl, durationMinutes, occurredAt); err != nil {
		log.Printf("create feed: %v", err)
		http.Error(w, "failed to save feed event", http.StatusBadGateway)
		return
	}

	feeds, err := h.Backend.ListFeeds(r.Context())
	if err != nil {
		log.Printf("list feeds: %v", err)
		http.Error(w, "failed to load feed events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "feed_list", feedsInLocation(feeds, loc)); err != nil {
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

	if err := h.Backend.CreateBath(r.Context(), r.FormValue("type"), r.FormValue("notes"), durationMinutes, occurredAt); err != nil {
		log.Printf("create bath: %v", err)
		http.Error(w, "failed to save bath event", http.StatusBadGateway)
		return
	}

	baths, err := h.Backend.ListBaths(r.Context())
	if err != nil {
		log.Printf("list baths: %v", err)
		http.Error(w, "failed to load bath events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "bath_list", bathsInLocation(baths, loc)); err != nil {
		log.Printf("render bath_list template: %v", err)
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

// feedsInLocation is inLocation's counterpart for feeds.
func feedsInLocation(feeds []backendclient.Feed, loc *time.Location) []backendclient.Feed {
	converted := make([]backendclient.Feed, len(feeds))
	for i, f := range feeds {
		f.OccurredAt = f.OccurredAt.In(loc)
		converted[i] = f
	}
	return converted
}

// bathsInLocation is inLocation's counterpart for baths.
func bathsInLocation(baths []backendclient.Bath, loc *time.Location) []backendclient.Bath {
	converted := make([]backendclient.Bath, len(baths))
	for i, b := range baths {
		b.OccurredAt = b.OccurredAt.In(loc)
		converted[i] = b
	}
	return converted
}
