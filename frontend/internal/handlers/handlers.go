package handlers

import (
	"context"
	"errors"
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

	"github.com/go-chi/chi/v5"

	"github.com/andreistefanciprian/yauli/frontend/internal/authclient"
	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
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
	CreateBaby(ctx context.Context, name string) (backendclient.Baby, error)
	UpdateCurrentBaby(ctx context.Context, name, timezone string) (backendclient.Baby, error)
	ArchiveCurrentBaby(ctx context.Context, confirmName string) error
	ListEvents(ctx context.Context, resource, rangeKey string, out any) error
	CreateEvent(ctx context.Context, resource string, payload map[string]any) error
	UpdateEvent(ctx context.Context, id string, payload map[string]any) error
	DeleteEvent(ctx context.Context, id string) error
	InviteHelper(ctx context.Context, babyID, email string) error
	ListTimelineMembers(ctx context.Context) (backendclient.TimelineMembersResult, error)
	UpdateTimelineMemberRelationship(ctx context.Context, userID, relationship string) error
	RemoveTimelineMember(ctx context.Context, userID string) error
}

// AuthClient is the auth-service boundary this package needs. Kept separate
// from Backend — a different service, a different domain (login vs. baby
// events) — rather than one interface spanning both.
type AuthClient interface {
	RequestMagicLink(ctx context.Context, email string) error
	RequestInviteMagicLink(ctx context.Context, email, babyName string) error
	VerifyMagicLink(ctx context.Context, token string) (authclient.VerifyResult, error)
	Logout(ctx context.Context, sessionID string) error
	MintToken(ctx context.Context, sessionID string) (authclient.MintResult, error)
	AttachFamily(ctx context.Context, sessionID, familyID string) error
}

type Handlers struct {
	Backend       Backend
	Auth          AuthClient
	Templates     *template.Template
	SecureCookies bool
}

// New wires up Handlers. secureCookies sets the yauli_session cookie's
// Secure flag — true in production (HTTPS), false in local dev (plain
// HTTP, where a Secure cookie would silently never be sent back).
func New(backend Backend, auth AuthClient, templates *template.Template, secureCookies bool) *Handlers {
	return &Handlers{Backend: backend, Auth: auth, Templates: templates, SecureCookies: secureCookies}
}

// TimelineEvent is the single presentation shape every event type (nappy,
// feed, bath, sleep, observation) is flattened into, so the timeline can
// render one kind of card regardless of how many event types exist.
type TimelineEvent struct {
	ID              string
	EventType       string
	CSSClass        string // "nappy", "feed", "bath", "observation" — drives card colour
	Icon            string
	TypeLabel       string
	Kind            string // nappy's kind / feed & bath's type / observation's category — shown as "(Kind)" next to TypeLabel
	Detail          string
	Time            string // pre-formatted for display, e.g. "11:15 AM" or "Jan 2, 11:15 AM"
	DateValue       string
	TimeValue       string
	KindValue       string
	TypeValue       string
	Colour          string
	AmountMl        string
	DurationMinutes string
	Notes           string
	Text            string
	Category        string
}

type TimelineViewData struct {
	Events       []TimelineEvent
	Ranges       []TimelineRangeOption
	Selected     string
	EmptyMessage string
}

type TimelineRangeOption struct {
	Key    string
	Label  string
	Href   string
	Active bool
}

type inviteStatus struct {
	Message string
	Error   string
}

type indexPageData struct {
	Baby     backendclient.Baby
	Timeline TimelineViewData
	NowDate  string
	NowTime  string
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	h.renderIndex(w, r)
}

func (h *Handlers) renderIndex(w http.ResponseWriter, r *http.Request) {
	baby, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		if errors.Is(err, backendclient.ErrNotFound) {
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	rangeKey := selectedTimelineRange(r)
	timeline, err := h.loadTimeline(r.Context(), loc, rangeKey)
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load events", http.StatusBadGateway)
		return
	}

	now := time.Now().In(loc)
	data := indexPageData{
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

	h.renderTimeline(w, r, loc)
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

	h.renderTimeline(w, r, loc)
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

	h.renderTimeline(w, r, loc)
}

func (h *Handlers) CreateSleep(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("parse sleep time: %v", err)
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
	if err := h.Backend.CreateEvent(r.Context(), "sleeps", payload); err != nil {
		log.Printf("create sleep: %v", err)
		http.Error(w, "failed to save sleep event", http.StatusBadGateway)
		return
	}

	h.renderTimeline(w, r, loc)
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

	h.renderTimeline(w, r, loc)
}

// UpdateEvent edits a single event, regardless of its type, and re-renders
// the timeline — the same shared tail as every Create* and Delete handler.
func (h *Handlers) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

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

	payload, err := h.eventUpdatePayloadFromForm(loc, r)
	if err != nil {
		log.Printf("build event update payload: %v", err)
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}

	if err := h.Backend.UpdateEvent(r.Context(), id, payload); err != nil {
		log.Printf("update event: %v", err)
		http.Error(w, "failed to update event", http.StatusBadGateway)
		return
	}

	h.renderTimeline(w, r, loc)
}

func (h *Handlers) eventUpdatePayloadFromForm(loc *time.Location, r *http.Request) (map[string]any, error) {
	eventType := r.FormValue("event_type")
	occurredAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		return nil, err
	}

	attributes := map[string]any{}
	switch eventType {
	case "nappy":
		attributes["kind"] = r.FormValue("kind")
		attributes["colour"] = r.FormValue("colour")
	case "feed":
		amountMl, err := parseOptionalInt(r.FormValue("amount_ml"))
		if err != nil {
			return nil, err
		}
		durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
		if err != nil {
			return nil, err
		}
		attributes["type"] = r.FormValue("type")
		attributes["amount_ml"] = amountMl
		attributes["duration_minutes"] = durationMinutes
	case "bath", "sleep":
		durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
		if err != nil {
			return nil, err
		}
		attributes["type"] = r.FormValue("type")
		attributes["notes"] = r.FormValue("notes")
		attributes["duration_minutes"] = durationMinutes
	case "observation":
		attributes["text"] = r.FormValue("text")
		attributes["category"] = r.FormValue("category")
	default:
		return nil, fmt.Errorf("unsupported event type %q", eventType)
	}

	return map[string]any{
		"event_type":  eventType,
		"attributes":  attributes,
		"occurred_at": occurredAt.Format(time.RFC3339),
	}, nil
}

// DeleteEvent removes a single event, regardless of its type, and re-renders
// the timeline — the same shared tail as every Create* handler.
func (h *Handlers) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	if err := h.Backend.DeleteEvent(r.Context(), id); err != nil {
		log.Printf("delete event: %v", err)
		http.Error(w, "failed to delete event", http.StatusBadGateway)
		return
	}

	h.renderTimeline(w, r, loc)
}

// renderTimeline re-fetches every event type and writes the combined,
// sorted timeline partial. It's the shared tail of every Create* handler,
// since all four forms target the same #timeline container.
func (h *Handlers) renderTimeline(w http.ResponseWriter, r *http.Request, loc *time.Location) {
	timeline, err := h.loadTimeline(r.Context(), loc, selectedTimelineRange(r))
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
func (h *Handlers) loadTimeline(ctx context.Context, loc *time.Location, rangeKey string) (TimelineViewData, error) {
	var events []backendclient.Event
	if err := h.Backend.ListEvents(ctx, "events", rangeKey, &events); err != nil {
		return TimelineViewData{}, fmt.Errorf("list events: %w", err)
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

	return TimelineViewData{
		Events:       timeline,
		Ranges:       timelineRangeOptions(rangeKey),
		Selected:     rangeKey,
		EmptyMessage: emptyTimelineMessage(rangeKey),
	}, nil
}

func selectedTimelineRange(r *http.Request) string {
	raw := r.FormValue("range")
	switch raw {
	case "", "today":
		return "today"
	case "yesterday", "24h", "3d":
		return raw
	default:
		return "today"
	}
}

func timelineRangeOptions(selected string) []TimelineRangeOption {
	options := []TimelineRangeOption{
		{Key: "today", Label: "Today"},
		{Key: "yesterday", Label: "Yesterday"},
		{Key: "24h", Label: "24h"},
		{Key: "3d", Label: "3 days"},
	}
	for i := range options {
		options[i].Href = "/app?range=" + options[i].Key
		options[i].Active = options[i].Key == selected
	}
	return options
}

func emptyTimelineMessage(rangeKey string) string {
	switch rangeKey {
	case "yesterday":
		return "No events logged yesterday."
	case "24h":
		return "No events logged in the last 24 hours."
	case "3d":
		return "No events logged in the last 3 days."
	default:
		return "No events logged today. Tap \"Add Event\" to log the first one."
	}
}

// timelineEvent dispatches a generic Event to the builder for its type. ok
// is false for an event_type this frontend doesn't know how to render (e.g.
// a new type added to backend-api before the frontend picks it up).
func timelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) (TimelineEvent, bool) {
	var te TimelineEvent
	switch ev.EventType {
	case "nappy":
		te = nappyTimelineEvent(ev, loc, now)
	case "feed":
		te = feedTimelineEvent(ev, loc, now)
	case "bath":
		te = bathTimelineEvent(ev, loc, now)
	case "sleep":
		te = sleepTimelineEvent(ev, loc, now)
	case "observation":
		te = observationTimelineEvent(ev, loc, now)
	default:
		return TimelineEvent{}, false
	}
	te.ID = ev.ID
	te.EventType = ev.EventType
	occurredAt := ev.OccurredAt.In(loc)
	te.DateValue = occurredAt.Format(dateFieldLayout)
	te.TimeValue = occurredAt.Format(timeFieldLayout)
	return te, true
}

func nappyTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)
	kind := attributeString(ev.Attributes, "kind")
	colour := attributeString(ev.Attributes, "colour")

	return TimelineEvent{
		CSSClass:  "nappy",
		Icon:      "💩",
		TypeLabel: "Nappy",
		Kind:      titleCase(kind),
		Detail:    colour,
		Time:      formatEventTime(occurredAt, now),
		KindValue: kind,
		Colour:    colour,
	}
}

func feedTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)
	feedType := attributeString(ev.Attributes, "type")

	detail := amountAndDuration(ev.Attributes, "amount_ml", "ml")
	amountMl := ""
	if amount, ok := attributeInt(ev.Attributes, "amount_ml"); ok {
		amountMl = strconv.Itoa(amount)
	}
	durationMinutes := ""
	if duration, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		durationMinutes = strconv.Itoa(duration)
	}

	return TimelineEvent{
		CSSClass:        "feed",
		Icon:            "🍼",
		TypeLabel:       "Feed",
		Kind:            titleCase(feedType),
		Detail:          detail,
		Time:            formatEventTime(occurredAt, now),
		TypeValue:       feedType,
		AmountMl:        amountMl,
		DurationMinutes: durationMinutes,
	}
}

func bathTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)

	detail := attributeString(ev.Attributes, "notes")
	durationMinutes := ""
	if durationMinutes, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		if detail != "" {
			detail += " · "
		}
		detail += fmt.Sprintf("%d min", durationMinutes)
	}
	if duration, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		durationMinutes = strconv.Itoa(duration)
	}
	bathType := attributeString(ev.Attributes, "type")
	notes := attributeString(ev.Attributes, "notes")

	return TimelineEvent{
		CSSClass:        "bath",
		Icon:            "🛁",
		TypeLabel:       "Bath",
		Kind:            titleCase(bathType),
		Detail:          detail,
		Time:            formatEventTime(occurredAt, now),
		TypeValue:       bathType,
		Notes:           notes,
		DurationMinutes: durationMinutes,
	}
}

func sleepTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)

	detail := attributeString(ev.Attributes, "notes")
	durationMinutes := ""
	if durationMinutes, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		if detail != "" {
			detail += " · "
		}
		detail += fmt.Sprintf("%d min", durationMinutes)
	}
	if duration, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		durationMinutes = strconv.Itoa(duration)
	}
	sleepType := attributeString(ev.Attributes, "type")
	notes := attributeString(ev.Attributes, "notes")

	return TimelineEvent{
		CSSClass:        "sleep",
		Icon:            "😴",
		TypeLabel:       "Sleep",
		Kind:            titleCase(sleepType),
		Detail:          detail,
		Time:            formatEventTime(occurredAt, now),
		TypeValue:       sleepType,
		Notes:           notes,
		DurationMinutes: durationMinutes,
	}
}

func observationTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)
	text := attributeString(ev.Attributes, "text")
	category := attributeString(ev.Attributes, "category")

	return TimelineEvent{
		CSSClass:  "observation",
		Icon:      "📝",
		TypeLabel: "Observation",
		Kind:      titleCase(category),
		Detail:    text,
		Time:      formatEventTime(occurredAt, now),
		Text:      text,
		Category:  category,
	}
}

// amountAndDuration builds "<amount> <unit> · <duration> min", omitting
// either half that's absent — shared by any event type whose Detail is just
// an optional quantity plus an optional duration (currently only feed).
func amountAndDuration(attributes map[string]any, amountKey, unit string) string {
	var detail string
	if amount, ok := attributeInt(attributes, amountKey); ok {
		detail = fmt.Sprintf("%d %s", amount, unit)
	}
	if durationMinutes, ok := attributeInt(attributes, "duration_minutes"); ok {
		if detail != "" {
			detail += " · "
		}
		detail += fmt.Sprintf("%d min", durationMinutes)
	}
	return detail
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
