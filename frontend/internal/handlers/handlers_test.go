package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
)

func TestDailyReportAIUsesGeneratedCopyOrDeterministicFallback(t *testing.T) {
	tests := []struct {
		name      string
		aiCard    backendclient.AIDailyCard
		aiErr     error
		wantIntro string
	}{
		{
			name: "valid generated copy",
			aiCard: backendclient.AIDailyCard{
				SchemaVersion: "daily_card_output.v1",
				Opening:       "Here's how YauYau's day is taking shape.",
				Story:         "The day also included plenty of nappy changes.",
				Observation:   "The day is still unfolding.",
				Encouragement: "You've got this, Dad.",
			},
			wantIntro: "Here's how YauYau's day is taking shape.",
		},
		{
			name:      "provider failure keeps fallback",
			aiErr:     errors.New("provider unavailable"),
			wantIntro: "Deterministic intro.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &dailyReportAIBackend{
				report: backendclient.DailyReport{
					Title: "Today so far",
					Card: &backendclient.DailyReportCard{
						Intro:         "Deterministic intro.",
						Observation:   "Deterministic observation.",
						Encouragement: "You've got this.",
					},
				},
				aiCard: tt.aiCard,
				aiErr:  tt.aiErr,
			}
			templates := template.Must(template.New("root").Parse(`{{define "daily-report"}}{{.Card.Intro}}|{{.Card.Observation}}|{{.Card.Encouragement}}{{end}}`))
			h := &Handlers{Backend: backend, Templates: templates}

			rec := httptest.NewRecorder()
			loc, err := time.LoadLocation("Australia/Adelaide")
			if err != nil {
				t.Fatalf("load timezone: %v", err)
			}
			today := time.Now().In(loc).Format(time.DateOnly)
			req := httptest.NewRequest("GET", "/daily-report/ai?date="+today, nil)
			h.DailyReportAI(rec, req)

			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(html.UnescapeString(rec.Body.String()), tt.wantIntro) {
				t.Fatalf("body = %q, want intro %q", rec.Body.String(), tt.wantIntro)
			}
			if backend.reportDate != today || !backend.aiCalled {
				t.Fatalf("report date = %q, AI called = %v", backend.reportDate, backend.aiCalled)
			}
		})
	}
}

func TestDailyReportAIKeepsHistoricalDayDeterministic(t *testing.T) {
	backend := &dailyReportAIBackend{
		report: backendclient.DailyReport{
			Title: "Yesterday",
			Card: &backendclient.DailyReportCard{
				Intro:         "Deterministic intro.",
				Observation:   "Deterministic observation.",
				Encouragement: "You've got this.",
			},
		},
	}
	templates := template.Must(template.New("root").Parse(`{{define "daily-report"}}{{.Card.Intro}}{{end}}`))
	h := &Handlers{Backend: backend, Templates: templates}

	loc, err := time.LoadLocation("Australia/Adelaide")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	yesterday := time.Now().In(loc).AddDate(0, 0, -1).Format(time.DateOnly)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/daily-report/ai?date="+yesterday, nil)
	h.DailyReportAI(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if backend.aiCalled {
		t.Fatal("historical daily report called the AI endpoint")
	}
	if !strings.Contains(rec.Body.String(), "Deterministic intro.") {
		t.Fatalf("body = %q, want deterministic intro", rec.Body.String())
	}
}

func TestDailyReportVisibilityDefaultsOnAndHonoursCookie(t *testing.T) {
	tests := []struct {
		name    string
		cookie  *http.Cookie
		visible bool
	}{
		{name: "default", visible: true},
		{name: "shown", cookie: &http.Cookie{Name: dailyReportVisibilityCookieName, Value: "1"}, visible: true},
		{name: "hidden", cookie: &http.Cookie{Name: dailyReportVisibilityCookieName, Value: "0"}, visible: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/app", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			if got := dailyReportVisible(req); got != tt.visible {
				t.Fatalf("dailyReportVisible() = %v, want %v", got, tt.visible)
			}
		})
	}
}

func TestLoadDailyReportIfVisibleSkipsHiddenReport(t *testing.T) {
	backend := &dailyReportAIBackend{
		report: backendclient.DailyReport{Title: "Today so far"},
	}
	h := &Handlers{Backend: backend}

	if got := h.loadDailyReportIfVisible(context.Background(), "2026-07-18", time.UTC, false); got != nil {
		t.Fatalf("hidden report = %#v, want nil", got)
	}
	if backend.reportCalls != 0 {
		t.Fatalf("hidden report made %d backend calls, want 0", backend.reportCalls)
	}

	if got := h.loadDailyReportIfVisible(context.Background(), "2026-07-18", time.UTC, true); got == nil {
		t.Fatal("visible report = nil")
	}
	if backend.reportCalls != 1 {
		t.Fatalf("visible report made %d backend calls, want 1", backend.reportCalls)
	}
}

func TestDailyReportVisibilityCookie(t *testing.T) {
	h := &Handlers{SecureCookies: true}
	cookie := h.dailyReportVisibilityCookie(false)
	if cookie.Name != dailyReportVisibilityCookieName || cookie.Value != "0" || cookie.Path != "/" {
		t.Fatalf("cookie = %#v", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge <= 0 {
		t.Fatalf("cookie attributes = %#v", cookie)
	}
}

func TestUpdateTimelineDailyReportPreferenceRefreshesWorkspace(t *testing.T) {
	for _, tt := range []struct {
		name            string
		visible         bool
		wantBody        string
		wantReportCalls int
		wantCookie      string
	}{
		{name: "hide", wantBody: "hidden", wantCookie: "0"},
		{name: "show", visible: true, wantBody: "report", wantReportCalls: 1, wantCookie: "1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := &dailyReportAIBackend{
				report: backendclient.DailyReport{Title: "Today so far"},
			}
			templates := template.Must(template.New("root").Parse(`{{define "timeline-workspace"}}{{if .DailyReport}}report{{else}}hidden{{end}}{{end}}`))
			h := &Handlers{Backend: backend, Templates: templates, SecureCookies: true}

			form := url.Values{"selected_date": {time.Now().Format(time.DateOnly)}}
			if tt.visible {
				form.Set("show_daily_report", "1")
			}
			req := httptest.NewRequest(http.MethodPost, "/timeline/preferences/daily-report", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()

			h.UpdateTimelineDailyReportPreference(rec, req)

			if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
			}
			if backend.reportCalls != tt.wantReportCalls {
				t.Fatalf("report calls = %d, want %d", backend.reportCalls, tt.wantReportCalls)
			}
			cookies := rec.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Name != dailyReportVisibilityCookieName || cookies[0].Value != tt.wantCookie {
				t.Fatalf("cookies = %#v", cookies)
			}
		})
	}
}

type dailyReportAIBackend struct {
	Backend
	report      backendclient.DailyReport
	aiCard      backendclient.AIDailyCard
	aiErr       error
	reportDate  string
	reportCalls int
	aiCalled    bool
}

func (b *dailyReportAIBackend) GetCurrentBaby(context.Context) (backendclient.Baby, error) {
	return backendclient.Baby{Timezone: "Australia/Adelaide"}, nil
}

func (b *dailyReportAIBackend) ListEvents(_ context.Context, _, _ string, out any) error {
	events, ok := out.(*[]backendclient.Event)
	if !ok {
		return errors.New("unexpected event output type")
	}
	*events = nil
	return nil
}

func (b *dailyReportAIBackend) GetDailyReport(_ context.Context, date string) (backendclient.DailyReport, error) {
	b.reportDate = date
	b.reportCalls++
	return b.report, nil
}

func (b *dailyReportAIBackend) CreateTodayAIDailyCard(context.Context) (backendclient.AIDailyCard, error) {
	b.aiCalled = true
	return b.aiCard, b.aiErr
}

func TestFeedAmountFromFormIgnoresBreastAmount(t *testing.T) {
	amount, err := feedAmountFromForm("breast", "80")
	if err != nil {
		t.Fatalf("feedAmountFromForm returned error: %v", err)
	}
	if amount != nil {
		t.Fatalf("feedAmountFromForm breast amount = %v, want nil", *amount)
	}
}

func TestFeedAmountFromFormRequiresBottleAmount(t *testing.T) {
	for _, feedType := range []string{"formula", "expressed"} {
		t.Run(feedType, func(t *testing.T) {
			if _, err := feedAmountFromForm(feedType, ""); err == nil {
				t.Fatalf("feedAmountFromForm accepted empty %s amount", feedType)
			}
			if _, err := feedAmountFromForm(feedType, "0"); err == nil {
				t.Fatalf("feedAmountFromForm accepted zero %s amount", feedType)
			}
		})
	}
}

func TestFeedTimelineEventMarksMissingDurationOngoing(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "feed",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"type":      "expressed",
			"amount_ml": float64(80),
			"labels":    []any{"burped_after"},
		},
	}

	timelineEvent := feedTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))
	if timelineEvent.StatusLabel != "Ongoing" {
		t.Fatalf("StatusLabel = %q, want Ongoing", timelineEvent.StatusLabel)
	}
	if !timelineEvent.CanFinishFeed {
		t.Fatal("CanFinishFeed = false, want true")
	}
	if timelineEvent.DurationMinutes != "" {
		t.Fatalf("DurationMinutes = %q, want empty", timelineEvent.DurationMinutes)
	}
	if timelineEvent.AmountMl != "80" {
		t.Fatalf("AmountMl = %q, want 80", timelineEvent.AmountMl)
	}
}

func TestNappyTimelineEventUsesPlainPooSizeLabel(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "nappy",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"kind":     "both",
			"poo_size": "large",
		},
	}

	timelineEvent := nappyTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))
	if timelineEvent.Detail != "Large" {
		t.Fatalf("Detail = %q, want Large", timelineEvent.Detail)
	}
	if timelineEvent.PooSizeValue != "large" {
		t.Fatalf("PooSizeValue = %q, want large", timelineEvent.PooSizeValue)
	}
}

func TestNappyTimelineEventUsesKindAsLabel(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{kind: "wet", want: "Wee"},
		{kind: "both", want: "Wee Poo"},
		{kind: "poo", want: "Poo"},
	}

	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			ev := backendclient.Event{
				EventType:  "nappy",
				OccurredAt: occurredAt,
				Attributes: map[string]any{"kind": test.kind},
			}

			timelineEvent := nappyTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))
			if timelineEvent.TypeLabel != test.want {
				t.Fatalf("TypeLabel = %q, want %q", timelineEvent.TypeLabel, test.want)
			}
			if timelineEvent.Kind != "" {
				t.Fatalf("Kind = %q, want empty", timelineEvent.Kind)
			}
			if timelineEvent.KindValue != test.kind {
				t.Fatalf("KindValue = %q, want %q", timelineEvent.KindValue, test.kind)
			}
		})
	}
}

func TestSleepTimelineEventUsesSleepTypeAsLabel(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 16, 30, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "sleep",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"type":             "nap",
			"duration_minutes": float64(10),
		},
	}

	timelineEvent := sleepTimelineEvent(ev, loc, occurredAt.Add(10*time.Minute))

	if timelineEvent.TypeLabel != "Nap" {
		t.Fatalf("TypeLabel = %q, want Nap", timelineEvent.TypeLabel)
	}
	if timelineEvent.Kind != "" {
		t.Fatalf("Kind = %q, want empty", timelineEvent.Kind)
	}
	if timelineEvent.TypeValue != "nap" {
		t.Fatalf("TypeValue = %q, want nap", timelineEvent.TypeValue)
	}
}

func TestGrowthMeasurementTimelineEventPrefillsEditValues(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "growth_measurement",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"weight_grams":          float64(3135),
			"length_cm":             float64(52.4),
			"head_circumference_cm": float64(35.7),
			"notes":                 "checkup",
		},
	}

	timelineEvent := growthMeasurementTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))

	if timelineEvent.WeightKg != "3.135" {
		t.Fatalf("WeightKg = %q, want 3.135", timelineEvent.WeightKg)
	}
	if timelineEvent.LengthCM != "52.4" {
		t.Fatalf("LengthCM = %q, want 52.4", timelineEvent.LengthCM)
	}
	if timelineEvent.HeadCircumferenceCM != "35.7" {
		t.Fatalf("HeadCircumferenceCM = %q, want 35.7", timelineEvent.HeadCircumferenceCM)
	}
	if timelineEvent.Notes != "checkup" {
		t.Fatalf("Notes = %q, want checkup", timelineEvent.Notes)
	}
	if timelineEvent.Detail != "3.135 kg · Length 52.4 cm · Head 35.7 cm · checkup" {
		t.Fatalf("Detail = %q", timelineEvent.Detail)
	}
}

func TestGrowthMeasurementTimelineEventAcceptsStoredNumberTypes(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "growth_measurement",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"weight_grams":          int64(3135),
			"length_cm":             json.Number("52.4"),
			"head_circumference_cm": json.Number("35.7"),
		},
	}

	timelineEvent := growthMeasurementTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))

	if timelineEvent.WeightKg != "3.135" || timelineEvent.LengthCM != "52.4" || timelineEvent.HeadCircumferenceCM != "35.7" {
		t.Fatalf("growth edit values = weight %q length %q head %q, want 3.135/52.4/35.7", timelineEvent.WeightKg, timelineEvent.LengthCM, timelineEvent.HeadCircumferenceCM)
	}
}

func TestShouldAutoRefreshTimelineOnlyForToday(t *testing.T) {
	now := time.Date(2026, 7, 14, 22, 15, 0, 0, time.UTC)

	if !shouldAutoRefreshTimeline("2026-07-14", now) {
		t.Fatal("today timeline should auto-refresh")
	}
	if shouldAutoRefreshTimeline("2026-07-13", now) {
		t.Fatal("past timeline should not auto-refresh")
	}
}
