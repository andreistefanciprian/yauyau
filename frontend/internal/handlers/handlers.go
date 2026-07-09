package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

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

// TimelineEvent is the single presentation shape every event type (nappy,
// feed, bath, observation) is flattened into, so the timeline can render one
// kind of card regardless of how many event types exist.
type TimelineEvent struct {
	CSSClass  string // "nappy", "feed", "bath", "observation" — drives card colour
	Icon      string
	TypeLabel string
	Subtitle  string // only observations use this, for the category
	Detail    string
	Time      string // pre-formatted for display, e.g. "11:15 AM" or "Jan 2, 11:15 AM"
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	baby, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	timeline, err := h.loadTimeline(r.Context(), loc)
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load events", http.StatusBadGateway)
		return
	}

	now := time.Now().In(loc)
	data := struct {
		Baby     backendclient.Baby
		Timeline []TimelineEvent
		NowDate  string
		NowTime  string
	}{
		Baby:     baby,
		Timeline: timeline,
		NowDate:  now.Format(dateFieldLayout),
		NowTime:  now.Format(timeFieldLayout),
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

	h.renderTimeline(w, r.Context(), loc)
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

	h.renderTimeline(w, r.Context(), loc)
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

	h.renderTimeline(w, r.Context(), loc)
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

	h.renderTimeline(w, r.Context(), loc)
}

// renderTimeline re-fetches every event type and writes the combined,
// sorted timeline partial. It's the shared tail of every Create* handler,
// since all four forms target the same #timeline container.
func (h *Handlers) renderTimeline(w http.ResponseWriter, ctx context.Context, loc *time.Location) {
	timeline, err := h.loadTimeline(ctx, loc)
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "timeline", timeline); err != nil {
		log.Printf("render timeline template: %v", err)
	}
}

// loadTimeline fetches the most recent events across every type from
// backend-api's combined /events endpoint — already merged and ordered
// newest-first by the store — and flattens each into a TimelineEvent.
func (h *Handlers) loadTimeline(ctx context.Context, loc *time.Location) ([]TimelineEvent, error) {
	var events []backendclient.Event
	if err := h.Backend.ListEvents(ctx, "events", &events); err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	now := time.Now().In(loc)
	timeline := make([]TimelineEvent, 0, len(events))
	for _, ev := range events {
		te, ok := timelineEvent(ev, loc, now)
		if !ok {
			log.Printf("skipping event %s: unknown event_type %q", ev.ID, ev.EventType)
			continue
		}
		timeline = append(timeline, te)
	}

	return timeline, nil
}

// timelineEvent dispatches a generic Event to the builder for its type. ok
// is false for an event_type this frontend doesn't know how to render (e.g.
// a new type added to backend-api before the frontend picks it up).
func timelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) (TimelineEvent, bool) {
	switch ev.EventType {
	case "nappy":
		return nappyTimelineEvent(ev, loc, now), true
	case "feed":
		return feedTimelineEvent(ev, loc, now), true
	case "bath":
		return bathTimelineEvent(ev, loc, now), true
	case "observation":
		return observationTimelineEvent(ev, loc, now), true
	default:
		return TimelineEvent{}, false
	}
}

func nappyTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)

	detail := titleCase(attributeString(ev.Attributes, "kind"))
	if colour := attributeString(ev.Attributes, "colour"); colour != "" {
		detail += ", " + colour
	}

	return TimelineEvent{
		CSSClass:  "nappy",
		Icon:      "💩",
		TypeLabel: "Nappy",
		Detail:    detail,
		Time:      formatEventTime(occurredAt, now),
	}
}

func feedTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)

	feedType := attributeString(ev.Attributes, "type")
	var detail string
	if amountMl, ok := attributeInt(ev.Attributes, "amount_ml"); ok {
		detail = fmt.Sprintf("%d ml %s", amountMl, feedType)
	} else {
		detail = feedType
	}
	detail = titleCase(detail)
	if durationMinutes, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		detail += fmt.Sprintf(" · %d min", durationMinutes)
	}

	return TimelineEvent{
		CSSClass:  "feed",
		Icon:      "🍼",
		TypeLabel: "Feed",
		Detail:    detail,
		Time:      formatEventTime(occurredAt, now),
	}
}

func bathTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)

	detail := titleCase(attributeString(ev.Attributes, "type"))
	if notes := attributeString(ev.Attributes, "notes"); notes != "" {
		detail += ", " + notes
	}
	if durationMinutes, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		detail += fmt.Sprintf(" · %d min", durationMinutes)
	}

	return TimelineEvent{
		CSSClass:  "bath",
		Icon:      "🛁",
		TypeLabel: "Bath",
		Detail:    detail,
		Time:      formatEventTime(occurredAt, now),
	}
}

func observationTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)

	return TimelineEvent{
		CSSClass:  "observation",
		Icon:      "📝",
		TypeLabel: "Observation",
		Subtitle:  titleCase(attributeString(ev.Attributes, "category")),
		Detail:    attributeString(ev.Attributes, "text"),
		Time:      formatEventTime(occurredAt, now),
	}
}

// attributeString reads a string field out of an event's attributes map,
// returning "" if the key is absent (an optional field, like nappy colour
// or bath notes, that wasn't recorded).
func attributeString(attributes map[string]any, key string) string {
	s, _ := attributes[key].(string)
	return s
}

// attributeInt reads an int field out of an event's attributes map. JSON
// numbers decode into map[string]any as float64, so that's the only
// numeric type checked; ok is false when the key is absent (an optional
// field like amount_ml or duration_minutes that wasn't recorded).
func attributeInt(attributes map[string]any, key string) (int, bool) {
	v, ok := attributes[key].(float64)
	if !ok {
		return 0, false
	}
	return int(v), true
}

// titleCase turns a snake_case or lowercase value (e.g. "whole_body", "poo",
// or free-text like a user-typed observation category) into display text
// ("Whole body", "Poo"). Slices by rune, not byte, so a leading multi-byte
// character (e.g. "über") isn't split mid-encoding.
func titleCase(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	r := []rune(s)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

// formatEventTime renders occurredAt as a bare time ("11:15 AM") when it
// falls on the same day as now, or with a date prefix otherwise, so older
// timeline entries stay unambiguous.
func formatEventTime(occurredAt, now time.Time) string {
	if occurredAt.Year() == now.Year() && occurredAt.YearDay() == now.YearDay() {
		return occurredAt.Format("3:04 PM")
	}
	return occurredAt.Format("Jan 2, 3:04 PM")
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
