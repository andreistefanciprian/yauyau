package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"math"
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
	GetCurrentUser(ctx context.Context) (backendclient.User, error)
	UpdateCurrentUser(ctx context.Context, displayName string) (backendclient.User, error)
	GetCurrentBaby(ctx context.Context) (backendclient.Baby, error)
	CreateBaby(ctx context.Context, name string) (backendclient.Baby, error)
	UpdateCurrentBaby(ctx context.Context, baby backendclient.Baby) (backendclient.Baby, error)
	ArchiveCurrentBaby(ctx context.Context, confirmName string) error
	ListEvents(ctx context.Context, resource, date string, out any) error
	GetDailyReport(ctx context.Context, date string) (backendclient.DailyReport, error)
	CreateEvent(ctx context.Context, resource string, payload map[string]any) error
	UpdateEvent(ctx context.Context, id string, payload map[string]any) error
	DeleteEvent(ctx context.Context, id string) error
	InviteHelper(ctx context.Context, babyID, email string) error
	ListTimelineMembers(ctx context.Context) (backendclient.TimelineMembersResult, error)
	UpdateTimelineMemberRelationship(ctx context.Context, userID, relationship string) error
	UpdateTimelineMemberReportPreferences(ctx context.Context, userID string, dailyReportEmailEnabled bool) error
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
// feed, pump, bath, sleep, observation) is flattened into, so the timeline can
// render one kind of card regardless of how many event types exist.
type TimelineEvent struct {
	ID                  string
	EventType           string
	CSSClass            string // "nappy", "feed", "pump", "bath", "observation" — drives card colour
	TypeLabel           string
	Kind                string // feed & bath's type / observation's category — shown next to TypeLabel
	InlineDetail        string // short high-signal detail shown beside the event type, e.g. pump amount
	Detail              string
	StatusLabel         string
	CanFinishFeed       bool
	CanFinishSleep      bool
	CanFinishPump       bool
	Ongoing             bool
	Time                string // pre-formatted for display, e.g. "11:15 AM" or "Jan 2, 11:15 AM"
	DateValue           string
	TimeValue           string
	KindValue           string
	PooSizeValue        string
	LabelValues         string
	TypeValue           string
	AmountMl            string
	DurationMinutes     string
	TemperatureC        string
	WeightKg            string
	LengthCM            string
	HeadCircumferenceCM string
	Method              string
	Notes               string
	Text                string
	Category            string
}

type TimelineViewData struct {
	Events             []TimelineEvent
	Ranges             []TimelineRangeOption
	SelectedDate       string
	EmptyMessage       string
	AutoRefresh        bool
	AutoRefreshTrigger string
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

type accountViewData struct {
	Label       string
	Email       string
	DisplayName string
}

type indexPageData struct {
	Baby        backendclient.Baby
	Account     accountViewData
	Timeline    TimelineViewData
	DailyReport *backendclient.DailyReport
	NowDate     string
	NowTime     string
}

type timelineWorkspaceData struct {
	Timeline    TimelineViewData
	DailyReport *backendclient.DailyReport
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	h.renderIndex(w, r)
}

func (h *Handlers) TimelineEvents(w http.ResponseWriter, r *http.Request) {
	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		if errors.Is(err, backendclient.ErrNotFound) {
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	selectedDate := selectedTimelineDate(r, loc)
	timeline, err := h.loadTimeline(r.Context(), loc, selectedDate)
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load events", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "timeline-section", timeline); err != nil {
		log.Printf("render timeline section template: %v", err)
	}
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

	selectedDate := selectedTimelineDate(r, loc)
	dailyReport := h.loadDailyReport(r.Context(), selectedDate)

	timeline, err := h.loadTimeline(r.Context(), loc, selectedDate)
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load events", http.StatusBadGateway)
		return
	}

	// The day-range links fetch this same route via htmx, so switching days
	// re-renders just the report + timeline in place — no full-page
	// reload/flash, matching how the (client-side) type filter already
	// feels instant.
	if r.Header.Get("HX-Request") == "true" {
		h.renderTimelineWorkspace(w, timeline, dailyReport)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	now := time.Now().In(loc)
	data := indexPageData{
		Baby:        baby,
		Account:     h.loadAccount(r.Context()),
		Timeline:    timeline,
		DailyReport: dailyReport,
		NowDate:     now.Format(dateFieldLayout),
		NowTime:     now.Format(timeFieldLayout),
	}

	if err := h.Templates.ExecuteTemplate(w, "index", data); err != nil {
		log.Printf("render index template: %v", err)
	}
}

func (h *Handlers) loadAccount(ctx context.Context) accountViewData {
	user, err := h.Backend.GetCurrentUser(ctx)
	if err != nil {
		log.Printf("load current user: %v", err)
		return accountViewData{Label: "Signed in", Email: "Signed in"}
	}

	return accountFromUser(user)
}

func accountFromUser(user backendclient.User) accountViewData {
	label := strings.TrimSpace(user.DisplayName)
	if label == "" {
		label = user.Email
	}
	return accountViewData{
		Label:       label,
		Email:       user.Email,
		DisplayName: user.DisplayName,
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
		"poo_size":    r.FormValue("poo_size"),
		"labels":      r.Form["labels"],
		"notes":       r.FormValue("notes"),
		"occurred_at": occurredAt.Format(time.RFC3339),
	}
	if err := h.Backend.CreateEvent(r.Context(), "nappies", payload); err != nil {
		log.Printf("create nappy: %v", err)
		writeBackendEventError(w, err, "failed to save nappy event")
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

	durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
	if err != nil {
		http.Error(w, "invalid duration", http.StatusBadRequest)
		return
	}
	feedType := r.FormValue("type")
	amountMl, err := feedAmountFromForm(feedType, r.FormValue("amount_ml"))
	if err != nil {
		http.Error(w, "invalid amount", http.StatusBadRequest)
		return
	}

	occurredAt, err := resolveDurationBasedOccurredAt(loc, r.FormValue("date"), r.FormValue("time"), r.FormValue("feed_time_basis"), durationMinutes)
	if err != nil {
		log.Printf("parse feed time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"type":             feedType,
		"duration_minutes": durationMinutes,
		"labels":           r.Form["labels"],
		"notes":            r.FormValue("notes"),
		"occurred_at":      occurredAt.Format(time.RFC3339),
	}
	if amountMl != nil {
		payload["amount_ml"] = *amountMl
	}
	if err := h.Backend.CreateEvent(r.Context(), "feeds", payload); err != nil {
		log.Printf("create feed: %v", err)
		writeBackendEventError(w, err, "failed to save feed event")
		return
	}

	h.renderTimeline(w, r, loc)
}

func (h *Handlers) CreatePump(w http.ResponseWriter, r *http.Request) {
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

	durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
	if err != nil {
		http.Error(w, "invalid duration", http.StatusBadRequest)
		return
	}

	occurredAt, err := resolveDurationBasedOccurredAt(loc, r.FormValue("date"), r.FormValue("time"), r.FormValue("pump_time_basis"), durationMinutes)
	if err != nil {
		log.Printf("parse pump time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	amountMl, err := parseRequiredPositiveInt(r.FormValue("amount_ml"))
	if err != nil {
		http.Error(w, "invalid amount", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"amount_ml":        amountMl,
		"duration_minutes": durationMinutes,
		"notes":            r.FormValue("notes"),
		"occurred_at":      occurredAt.Format(time.RFC3339),
	}
	if durationMinutes == nil {
		payload["ongoing"] = true
	}
	if err := h.Backend.CreateEvent(r.Context(), "pumps", payload); err != nil {
		log.Printf("create pump: %v", err)
		writeBackendEventError(w, err, "failed to save pump event")
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

	durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
	if err != nil {
		http.Error(w, "invalid duration", http.StatusBadRequest)
		return
	}

	occurredAt, err := resolveDurationBasedOccurredAt(loc, r.FormValue("date"), r.FormValue("time"), r.FormValue("bath_time_basis"), durationMinutes)
	if err != nil {
		log.Printf("parse bath time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
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
		writeBackendEventError(w, err, "failed to save bath event")
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

	durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
	if err != nil {
		http.Error(w, "invalid duration", http.StatusBadRequest)
		return
	}

	occurredAt, err := resolveDurationBasedOccurredAt(loc, r.FormValue("date"), r.FormValue("time"), r.FormValue("sleep_time_basis"), durationMinutes)
	if err != nil {
		log.Printf("parse sleep time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"notes":            r.FormValue("notes"),
		"duration_minutes": durationMinutes,
		"occurred_at":      occurredAt.Format(time.RFC3339),
	}
	if err := h.Backend.CreateEvent(r.Context(), "sleeps", payload); err != nil {
		log.Printf("create sleep: %v", err)
		writeBackendEventError(w, err, "failed to save sleep event")
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
		writeBackendEventError(w, err, "failed to save observation event")
		return
	}

	h.renderTimeline(w, r, loc)
}

func (h *Handlers) CreateTemperature(w http.ResponseWriter, r *http.Request) {
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

	temperatureC, err := parseRequiredFloat(r.FormValue("temperature_c"))
	if err != nil {
		http.Error(w, "invalid temperature", http.StatusBadRequest)
		return
	}

	occurredAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse temperature time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"temperature_c": temperatureC,
		"method":        r.FormValue("method"),
		"notes":         r.FormValue("notes"),
		"occurred_at":   occurredAt.Format(time.RFC3339),
	}
	if err := h.Backend.CreateEvent(r.Context(), "temperatures", payload); err != nil {
		log.Printf("create temperature: %v", err)
		writeBackendEventError(w, err, "failed to save temperature event")
		return
	}

	h.renderTimeline(w, r, loc)
}

func (h *Handlers) CreateGrowthMeasurement(w http.ResponseWriter, r *http.Request) {
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

	weightGrams, err := weightGramsFromKgForm(r.FormValue("weight_kg"))
	if err != nil {
		http.Error(w, "invalid weight", http.StatusBadRequest)
		return
	}
	lengthCM, err := parseOptionalFloat(r.FormValue("length_cm"))
	if err != nil {
		http.Error(w, "invalid length", http.StatusBadRequest)
		return
	}
	headCircumferenceCM, err := parseOptionalFloat(r.FormValue("head_circumference_cm"))
	if err != nil {
		http.Error(w, "invalid head circumference", http.StatusBadRequest)
		return
	}

	occurredAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse growth measurement time: %v", err)
		http.Error(w, "invalid date/time", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"weight_grams":          weightGrams,
		"length_cm":             lengthCM,
		"head_circumference_cm": headCircumferenceCM,
		"notes":                 r.FormValue("notes"),
		"occurred_at":           occurredAt.Format(time.RFC3339),
	}
	if err := h.Backend.CreateEvent(r.Context(), "growth-measurements", payload); err != nil {
		log.Printf("create growth measurement: %v", err)
		writeBackendEventError(w, err, "failed to save growth measurement")
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
		writeBackendEventError(w, err, "failed to update event")
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
		attributes["poo_size"] = r.FormValue("poo_size")
		attributes["labels"] = r.Form["labels"]
		attributes["notes"] = r.FormValue("notes")
	case "feed":
		durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
		if err != nil {
			return nil, err
		}
		feedType := r.FormValue("type")
		amountMl, err := feedAmountFromForm(feedType, r.FormValue("amount_ml"))
		if err != nil {
			return nil, err
		}
		occurredAt, err = resolveDurationBasedOccurredAt(loc, r.FormValue("date"), r.FormValue("time"), r.FormValue("feed_time_basis"), durationMinutes)
		if err != nil {
			return nil, err
		}
		attributes["type"] = feedType
		attributes["duration_minutes"] = durationMinutes
		attributes["labels"] = r.Form["labels"]
		attributes["notes"] = r.FormValue("notes")
		if amountMl != nil {
			attributes["amount_ml"] = *amountMl
		}
	case "pump":
		amountMl, err := parseRequiredPositiveInt(r.FormValue("amount_ml"))
		if err != nil {
			return nil, err
		}
		durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
		if err != nil {
			return nil, err
		}
		attributes["amount_ml"] = amountMl
		attributes["notes"] = r.FormValue("notes")
		if durationMinutes != nil {
			attributes["duration_minutes"] = durationMinutes
		} else if r.FormValue("ongoing") == "true" {
			attributes["ongoing"] = true
		}
	case "bath":
		durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
		if err != nil {
			return nil, err
		}
		occurredAt, err = resolveDurationBasedOccurredAt(loc, r.FormValue("date"), r.FormValue("time"), r.FormValue("bath_time_basis"), durationMinutes)
		if err != nil {
			return nil, err
		}
		attributes["type"] = r.FormValue("type")
		attributes["notes"] = r.FormValue("notes")
		attributes["duration_minutes"] = durationMinutes
	case "sleep":
		durationMinutes, err := parseOptionalInt(r.FormValue("duration_minutes"))
		if err != nil {
			return nil, err
		}
		occurredAt, err = resolveDurationBasedOccurredAt(loc, r.FormValue("date"), r.FormValue("time"), r.FormValue("sleep_time_basis"), durationMinutes)
		if err != nil {
			return nil, err
		}
		attributes["type"] = r.FormValue("type")
		attributes["notes"] = r.FormValue("notes")
		attributes["duration_minutes"] = durationMinutes
	case "observation":
		attributes["text"] = r.FormValue("text")
		attributes["category"] = r.FormValue("category")
	case "temperature":
		temperatureC, err := parseRequiredFloat(r.FormValue("temperature_c"))
		if err != nil {
			return nil, err
		}
		attributes["temperature_c"] = temperatureC
		attributes["method"] = r.FormValue("method")
		attributes["notes"] = r.FormValue("notes")
	case "growth_measurement":
		weightGrams, err := weightGramsFromKgForm(r.FormValue("weight_kg"))
		if err != nil {
			return nil, err
		}
		lengthCM, err := parseOptionalFloat(r.FormValue("length_cm"))
		if err != nil {
			return nil, err
		}
		headCircumferenceCM, err := parseOptionalFloat(r.FormValue("head_circumference_cm"))
		if err != nil {
			return nil, err
		}
		attributes["weight_grams"] = weightGrams
		attributes["length_cm"] = lengthCM
		attributes["head_circumference_cm"] = headCircumferenceCM
		attributes["notes"] = r.FormValue("notes")
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

// renderTimeline re-fetches the selected day's report and combined timeline partial.
// It's the shared tail of every Create*, Update*, and Delete* handler, since
// event changes can affect both the visible event list and day summary.
func (h *Handlers) renderTimeline(w http.ResponseWriter, r *http.Request, loc *time.Location) {
	selectedDate := selectedTimelineDate(r, loc)
	timeline, err := h.loadTimeline(r.Context(), loc, selectedDate)
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load events", http.StatusBadGateway)
		return
	}

	h.renderTimelineWorkspace(w, timeline, h.loadDailyReport(r.Context(), selectedDate))
}

// renderTimelineWorkspace writes the "timeline-workspace" partial (daily
// report + event list) directly, for callers that already have both pieces
// in hand and would otherwise re-fetch them redundantly.
func (h *Handlers) renderTimelineWorkspace(w http.ResponseWriter, timeline TimelineViewData, dailyReport *backendclient.DailyReport) {
	data := timelineWorkspaceData{Timeline: timeline, DailyReport: dailyReport}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "timeline-workspace", data); err != nil {
		log.Printf("render timeline workspace template: %v", err)
	}
}

func (h *Handlers) loadDailyReport(ctx context.Context, date string) *backendclient.DailyReport {
	report, err := h.Backend.GetDailyReport(ctx, date)
	if err != nil {
		log.Printf("load daily report: %v", err)
		return nil
	}
	return &report
}

// FinishSleepNow completes an ongoing sleep from its existing start time to
// the current time, so parents can end a sleep from the timeline without
// picking date/time fields.
func (h *Handlers) FinishSleepNow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	startedAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse sleep start: %v", err)
		http.Error(w, "invalid sleep start", http.StatusBadRequest)
		return
	}

	durationMinutes := int(time.Since(startedAt).Minutes())
	if durationMinutes < 1 {
		durationMinutes = 1
	}

	payload := map[string]any{
		"event_type": "sleep",
		"attributes": map[string]any{
			"type":             r.FormValue("type"),
			"notes":            r.FormValue("notes"),
			"duration_minutes": durationMinutes,
		},
		"occurred_at": startedAt.Format(time.RFC3339),
	}

	if err := h.Backend.UpdateEvent(r.Context(), id, payload); err != nil {
		log.Printf("finish sleep: %v", err)
		writeBackendEventError(w, err, "failed to finish sleep")
		return
	}

	h.renderTimeline(w, r, loc)
}

// FinishFeedNow completes an ongoing feed from its existing start time to the
// current time. Any recorded bottle amount is preserved; breast feeds keep
// amount absent.
func (h *Handlers) FinishFeedNow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	startedAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse feed start: %v", err)
		http.Error(w, "invalid feed start", http.StatusBadRequest)
		return
	}

	durationMinutes := int(time.Since(startedAt).Minutes())
	if durationMinutes < 1 {
		durationMinutes = 1
	}

	feedType := r.FormValue("type")
	attributes := map[string]any{
		"type":             feedType,
		"duration_minutes": durationMinutes,
	}
	if amountMl, err := feedAmountFromForm(feedType, r.FormValue("amount_ml")); err != nil {
		http.Error(w, "invalid amount", http.StatusBadRequest)
		return
	} else if amountMl != nil {
		attributes["amount_ml"] = *amountMl
	}
	if labels := strings.Split(r.FormValue("label_values"), ","); len(labels) > 0 && labels[0] != "" {
		attributes["labels"] = labels
	}
	if notes := r.FormValue("notes"); notes != "" {
		attributes["notes"] = notes
	}

	payload := map[string]any{
		"event_type":  "feed",
		"attributes":  attributes,
		"occurred_at": startedAt.Format(time.RFC3339),
	}

	if err := h.Backend.UpdateEvent(r.Context(), id, payload); err != nil {
		log.Printf("finish feed: %v", err)
		writeBackendEventError(w, err, "failed to finish feed")
		return
	}

	h.renderTimeline(w, r, loc)
}

// FinishPumpNow completes an ongoing pump from its existing start time to the
// current time.
func (h *Handlers) FinishPumpNow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	_, loc, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	startedAt, err := parseEventTime(loc, r.FormValue("date"), r.FormValue("time"))
	if err != nil {
		log.Printf("parse pump start: %v", err)
		http.Error(w, "invalid pump start", http.StatusBadRequest)
		return
	}

	durationMinutes := int(time.Since(startedAt).Minutes())
	if durationMinutes < 1 {
		durationMinutes = 1
	}

	amountMl, err := parseRequiredPositiveInt(r.FormValue("amount_ml"))
	if err != nil {
		http.Error(w, "invalid amount", http.StatusBadRequest)
		return
	}

	attributes := map[string]any{
		"amount_ml":        amountMl,
		"duration_minutes": durationMinutes,
	}
	if notes := r.FormValue("notes"); notes != "" {
		attributes["notes"] = notes
	}

	payload := map[string]any{
		"event_type":  "pump",
		"attributes":  attributes,
		"occurred_at": startedAt.Format(time.RFC3339),
	}

	if err := h.Backend.UpdateEvent(r.Context(), id, payload); err != nil {
		log.Printf("finish pump: %v", err)
		writeBackendEventError(w, err, "failed to finish pump")
		return
	}

	h.renderTimeline(w, r, loc)
}

// loadTimeline fetches the most recent events across every type from
// backend-api's combined /events endpoint — already merged and ordered
// newest-first by the store — and flattens each into a TimelineEvent.
func (h *Handlers) loadTimeline(ctx context.Context, loc *time.Location, selectedDate string) (TimelineViewData, error) {
	var events []backendclient.Event
	if err := h.Backend.ListEvents(ctx, "events", selectedDate, &events); err != nil {
		return TimelineViewData{}, fmt.Errorf("list events: %w", err)
	}

	now := time.Now().In(loc)
	timeline := make([]TimelineEvent, 0, len(events))
	for _, ev := range events {
		te, ok := timelineEvent(ev, loc, now)
		if !ok {
			slog.Warn("skipping event with unknown type", "event_id", ev.ID, "event_type", ev.EventType)
			continue
		}
		timeline = append(timeline, te)
	}

	return TimelineViewData{
		Events:             timeline,
		Ranges:             timelineRangeOptions(selectedDate, now),
		SelectedDate:       selectedDate,
		EmptyMessage:       emptyTimelineMessage(selectedDate, now),
		AutoRefresh:        shouldAutoRefreshTimeline(selectedDate, now),
		AutoRefreshTrigger: timelineAutoRefreshTrigger,
	}, nil
}

// timelineRangeDays is how many days back the date nav reaches: today plus
// the six days before it.
const (
	timelineRangeDays          = 7
	timelineAutoRefreshTrigger = "every 30s"
)

func timelineDate(offset int, now time.Time) string {
	return now.AddDate(0, 0, -offset).Format(time.DateOnly)
}

func timelineDateOffset(date string, now time.Time) (int, bool) {
	for offset := 0; offset < timelineRangeDays; offset++ {
		if timelineDate(offset, now) == date {
			return offset, true
		}
	}
	return 0, false
}

func selectedTimelineDate(r *http.Request, loc *time.Location) string {
	now := time.Now().In(loc)
	raw := r.FormValue("selected_date")
	if raw == "" {
		raw = r.URL.Query().Get("date")
	}
	if raw == "" {
		return timelineDate(0, now)
	}
	if _, ok := timelineDateOffset(raw, now); ok {
		return raw
	}
	return timelineDate(0, now)
}

func timelineRangeOptions(selectedDate string, now time.Time) []TimelineRangeOption {
	options := make([]TimelineRangeOption, timelineRangeDays)
	for offset := 0; offset < timelineRangeDays; offset++ {
		date := timelineDate(offset, now)
		options[offset] = TimelineRangeOption{
			Key:    date,
			Label:  timelineRangeLabel(offset, now),
			Href:   "/app?date=" + date,
			Active: date == selectedDate,
		}
	}
	return options
}

func shouldAutoRefreshTimeline(selectedDate string, now time.Time) bool {
	return selectedDate == timelineDate(0, now)
}

// timelineRangeLabel names each pill: "Today" and "Yesterday" for the most
// recent two days, then the short weekday name (e.g. "Thu") for the five
// days before that.
func timelineRangeLabel(offset int, now time.Time) string {
	switch offset {
	case 0:
		return "Today"
	case 1:
		return "Yesterday"
	default:
		return now.AddDate(0, 0, -offset).Format("Mon")
	}
}

func emptyTimelineMessage(date string, now time.Time) string {
	offset, ok := timelineDateOffset(date, now)
	if !ok {
		offset = 0
	}
	switch offset {
	case 0:
		return "No events logged today. Tap \"Add Event\" to log the first one."
	case 1:
		return "No events logged yesterday."
	default:
		return fmt.Sprintf("No events logged on %s.", now.AddDate(0, 0, -offset).Format("Monday"))
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
	case "pump":
		te = pumpTimelineEvent(ev, loc, now)
	case "bath":
		te = bathTimelineEvent(ev, loc, now)
	case "sleep":
		te = sleepTimelineEvent(ev, loc, now)
	case "observation":
		te = observationTimelineEvent(ev, loc, now)
	case "temperature":
		te = temperatureTimelineEvent(ev, loc, now)
	case "growth_measurement":
		te = growthMeasurementTimelineEvent(ev, loc, now)
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
	pooSize := attributeString(ev.Attributes, "poo_size")
	labels := attributeStringSlice(ev.Attributes, "labels")
	notes := attributeString(ev.Attributes, "notes")
	if notes == "" {
		notes = attributeString(ev.Attributes, "colour")
	}
	detailParts := []string{}
	if pooSize != "" {
		detailParts = append(detailParts, pooSizeLabel(pooSize))
	}
	for _, label := range labels {
		detailParts = append(detailParts, nappyLabelText(label))
	}
	if notes != "" {
		detailParts = append(detailParts, notes)
	}

	return TimelineEvent{
		CSSClass:     "nappy",
		TypeLabel:    nappyTypeLabel(kind),
		Detail:       strings.Join(detailParts, " · "),
		Time:         formatEventTime(occurredAt, now),
		KindValue:    kind,
		PooSizeValue: pooSize,
		LabelValues:  strings.Join(labels, ","),
		Notes:        notes,
	}
}

func nappyTypeLabel(kind string) string {
	switch kind {
	case "wet":
		return "Wee"
	case "both":
		return "Wee Poo"
	case "poo":
		return "Poo"
	default:
		return "Nappy"
	}
}

func pooSizeLabel(size string) string {
	switch size {
	case "smear":
		return "Smear"
	case "small":
		return "Small"
	case "medium":
		return "Medium"
	case "large":
		return "Large"
	case "blowout":
		return "Blowout"
	default:
		return titleCase(size)
	}
}

func nappyLabelText(label string) string {
	switch label {
	case "mustard_yellow":
		return "Mustard yellow"
	case "green":
		return "Green"
	case "brown":
		return "Brown"
	case "black":
		return "Black"
	case "red_blood":
		return "Red / blood"
	case "pale_white":
		return "Pale / white"
	case "seedy":
		return "Seedy"
	case "runny":
		return "Runny"
	case "sticky":
		return "Sticky"
	case "hard":
		return "Hard"
	case "mucus":
		return "Mucus"
	case "smelly":
		return "Smelly"
	case "rash":
		return "Rash"
	default:
		return titleCase(label)
	}
}

func feedTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)
	feedType := attributeString(ev.Attributes, "type")
	labels := attributeStringSlice(ev.Attributes, "labels")

	detail := amountAndDuration(ev.Attributes, "amount_ml", "ml")
	statusLabel := ""
	if _, ok := attributeInt(ev.Attributes, "duration_minutes"); !ok {
		if detail != "" {
			detail += " · "
		}
		detail += "Feed in progress"
		statusLabel = "Ongoing"
	}
	for _, label := range labels {
		if detail != "" {
			detail += " · "
		}
		detail += feedLabelText(label)
	}
	notes := attributeString(ev.Attributes, "notes")
	if notes != "" {
		if detail != "" {
			detail += " · "
		}
		detail += notes
	}
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
		TypeLabel:       "Feed",
		Kind:            titleCase(feedType),
		Detail:          detail,
		StatusLabel:     statusLabel,
		CanFinishFeed:   statusLabel != "",
		Time:            formatEventTime(occurredAt, now),
		TypeValue:       feedType,
		AmountMl:        amountMl,
		DurationMinutes: durationMinutes,
		LabelValues:     strings.Join(labels, ","),
		Notes:           notes,
	}
}

func feedLabelText(label string) string {
	switch label {
	case "burped_halfway":
		return "Burped halfway"
	case "burped_after":
		return "Burped after"
	case "spit_up":
		return "Spit-up"
	case "fussy":
		return "Fussy"
	case "sleepy":
		return "Sleepy"
	case "settled_after":
		return "Settled after"
	default:
		return titleCase(label)
	}
}

func pumpTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)
	notes := attributeString(ev.Attributes, "notes")

	inlineDetail := ""
	amountMl := ""
	if amount, ok := attributeInt(ev.Attributes, "amount_ml"); ok {
		amountMl = strconv.Itoa(amount)
		inlineDetail = fmt.Sprintf("%dml", amount)
	}

	detail := amountAndDuration(ev.Attributes, "amount_ml", "ml")
	statusLabel := ""
	ongoing := attributeBool(ev.Attributes, "ongoing")
	if ongoing {
		if detail != "" {
			detail += " · "
		}
		detail += "Pumping in progress"
		statusLabel = "Ongoing"
	}
	if notes != "" {
		if detail != "" {
			detail += " · "
		}
		detail += notes
	}
	durationMinutes := ""
	if duration, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		durationMinutes = strconv.Itoa(duration)
	}

	return TimelineEvent{
		CSSClass:        "pump",
		TypeLabel:       "Pump",
		InlineDetail:    inlineDetail,
		Detail:          detail,
		StatusLabel:     statusLabel,
		CanFinishPump:   statusLabel != "",
		Ongoing:         ongoing,
		Time:            formatEventTime(occurredAt, now),
		AmountMl:        amountMl,
		DurationMinutes: durationMinutes,
		Notes:           notes,
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
	statusLabel := ""
	durationMinutes := ""
	if durationMinutes, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		if detail != "" {
			detail += " · "
		}
		detail += fmt.Sprintf("%d min", durationMinutes)
	} else {
		if detail != "" {
			detail += " · "
		}
		detail += "Baby is asleep"
		statusLabel = "Ongoing"
	}
	if duration, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
		durationMinutes = strconv.Itoa(duration)
	}
	sleepType := attributeString(ev.Attributes, "type")
	notes := attributeString(ev.Attributes, "notes")

	return TimelineEvent{
		CSSClass:        "sleep",
		TypeLabel:       sleepTypeLabel(sleepType),
		Detail:          detail,
		StatusLabel:     statusLabel,
		CanFinishSleep:  statusLabel != "",
		Time:            formatEventTime(occurredAt, now),
		TypeValue:       sleepType,
		Notes:           notes,
		DurationMinutes: durationMinutes,
	}
}

func sleepTypeLabel(sleepType string) string {
	if sleepType == "nap" {
		return "Nap"
	}
	return titleCase(sleepType)
}

func observationTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)
	text := attributeString(ev.Attributes, "text")
	category := attributeString(ev.Attributes, "category")

	return TimelineEvent{
		CSSClass:  "observation",
		TypeLabel: "Observation",
		Kind:      titleCase(category),
		Detail:    text,
		Time:      formatEventTime(occurredAt, now),
		Text:      text,
		Category:  category,
	}
}

func temperatureTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)
	temperatureC, _ := attributeFloat(ev.Attributes, "temperature_c")
	method := attributeString(ev.Attributes, "method")
	notes := attributeString(ev.Attributes, "notes")

	detail := ""
	if method != "" {
		detail = titleCase(method)
	}
	if notes != "" {
		if detail != "" {
			detail += " · "
		}
		detail += notes
	}

	return TimelineEvent{
		CSSClass:     "temperature",
		TypeLabel:    "Temperature",
		InlineDetail: formatTemperatureC(temperatureC),
		Detail:       detail,
		Time:         formatEventTime(occurredAt, now),
		TemperatureC: formatTemperatureInput(temperatureC),
		Method:       method,
		Notes:        notes,
	}
}

func growthMeasurementTimelineEvent(ev backendclient.Event, loc *time.Location, now time.Time) TimelineEvent {
	occurredAt := ev.OccurredAt.In(loc)
	notes := attributeString(ev.Attributes, "notes")

	var detailParts []string
	weightKg := ""
	if weightGrams, ok := attributeInt(ev.Attributes, "weight_grams"); ok {
		weightKg = formatWeightKgInput(weightGrams)
		detailParts = append(detailParts, formatWeightKg(weightGrams))
	}
	lengthCM := ""
	if value, ok := attributeFloat(ev.Attributes, "length_cm"); ok {
		lengthCM = formatDecimalInput(value)
		detailParts = append(detailParts, fmt.Sprintf("Length %s cm", formatDecimal(value)))
	}
	headCircumferenceCM := ""
	if value, ok := attributeFloat(ev.Attributes, "head_circumference_cm"); ok {
		headCircumferenceCM = formatDecimalInput(value)
		detailParts = append(detailParts, fmt.Sprintf("Head %s cm", formatDecimal(value)))
	}
	if notes != "" {
		detailParts = append(detailParts, notes)
	}

	return TimelineEvent{
		CSSClass:            "growth",
		TypeLabel:           "Growth",
		Detail:              strings.Join(detailParts, " · "),
		Time:                formatEventTime(occurredAt, now),
		WeightKg:            weightKg,
		LengthCM:            lengthCM,
		HeadCircumferenceCM: headCircumferenceCM,
		Notes:               notes,
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
// returning "" if the key is absent (an optional field, like event notes, that
// wasn't recorded).
func attributeString(attributes map[string]any, key string) string {
	s, _ := attributes[key].(string)
	return s
}

func attributeStringSlice(attributes map[string]any, key string) []string {
	raw, ok := attributes[key].([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		s, ok := value.(string)
		if ok {
			values = append(values, s)
		}
	}
	return values
}

func attributeFloat(attributes map[string]any, key string) (float64, bool) {
	switch v := attributes[key].(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

// attributeInt reads an int field out of an event's attributes map. Timeline
// attributes usually arrive from JSON as float64, but update responses and
// tests can carry concrete integer values.
func attributeInt(attributes map[string]any, key string) (int, bool) {
	switch v := attributes[key].(type) {
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func attributeBool(attributes map[string]any, key string) bool {
	value, _ := attributes[key].(bool)
	return value
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

func resolveDurationBasedOccurredAt(loc *time.Location, date, hhmm, basis string, durationMinutes *int) (time.Time, error) {
	occurredAt, err := parseEventTime(loc, date, hhmm)
	if err != nil {
		return time.Time{}, err
	}

	switch basis {
	case "", "start":
		return occurredAt, nil
	case "end":
		if durationMinutes == nil {
			return occurredAt, nil
		}
		return occurredAt.Add(-time.Duration(*durationMinutes) * time.Minute), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time basis %q", basis)
	}
}

// writeBackendEventError responds to a failed create/update-event call. When
// err is a backendclient.APIError from a 4xx response, its message (e.g.
// "occurred_at cannot be in the future") is a validation problem the user
// caused and can fix, so it's shown as-is with a matching 400. Anything else
// (a network failure, a 5xx) is an upstream problem, not the user's fault, so
// fallback is shown with 502 instead of leaking an internal error string.
func writeBackendEventError(w http.ResponseWriter, err error, fallback string) {
	var apiErr *backendclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		http.Error(w, apiErr.Message, http.StatusBadRequest)
		return
	}
	http.Error(w, fallback, http.StatusBadGateway)
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

func feedAmountFromForm(feedType, raw string) (*int, error) {
	if feedType == "breast" {
		return nil, nil
	}
	value, err := parseRequiredPositiveInt(raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseRequiredPositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parsing int %q: %w", raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("expected positive int, got %d", value)
	}
	return value, nil
}

func parseRequiredFloat(raw string) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing float %q: %w", raw, err)
	}
	return value, nil
}

func parseOptionalFloat(raw string) (*float64, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing float %q: %w", raw, err)
	}
	return &value, nil
}

func weightGramsFromKgForm(raw string) (*int, error) {
	if raw == "" {
		return nil, nil
	}
	weightKg, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing weight kg %q: %w", raw, err)
	}
	weightGrams := int(math.Round(weightKg * 1000))
	return &weightGrams, nil
}

func formatTemperatureC(value float64) string {
	return fmt.Sprintf("%.1f °C", value)
}

func formatTemperatureInput(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64)
}

func formatWeightKg(valueGrams int) string {
	return fmt.Sprintf("%.3f kg", float64(valueGrams)/1000)
}

func formatWeightKgInput(valueGrams int) string {
	return strconv.FormatFloat(float64(valueGrams)/1000, 'f', 3, 64)
}

func formatDecimal(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func formatDecimalInput(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
