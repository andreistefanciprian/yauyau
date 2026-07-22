package handlers

import (
	"testing"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestBuildDailyReportSummarizesTimelineEvents(t *testing.T) {
	window := timelineDayWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 11, 12, 30, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(window.From, window.From)

	report := buildDailyReport([]store.Event{
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(70)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(95)}},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(80)}},
		{EventType: eventTypeBath, Attributes: map[string]any{"type": "whole_body"}},
		{EventType: eventTypeObservation, Attributes: map[string]any{"text": "smiley"}},
		{EventType: eventTypeTemperature, Attributes: map[string]any{"temperature_c": float64(37.2)}},
		{EventType: eventTypeGrowthMeasurement, Attributes: map[string]any{"weight_grams": float64(4200)}},
	}, window, window.To, period)

	if report.Title != "Today so far" {
		t.Fatalf("Title = %q, want Today so far", report.Title)
	}
	if report.Summary != "Today has feeding, nappies, and sleep logged so far." {
		t.Fatalf("Summary = %q", report.Summary)
	}

	wantHighlights := []string{
		"2 feeds with 70 ml recorded and 1 breast feed.",
		"1 nappy change: 1 mixed.",
		"1 sleep totalling 1 hour 35 minutes.",
		"1 pump recorded totalling 80 ml.",
		"1 bath logged.",
		"1 observation captured.",
		"1 temperature recorded.",
		"1 growth measurement recorded.",
	}
	if len(report.Highlights) != len(wantHighlights) {
		t.Fatalf("len(Highlights) = %d, want %d: %#v", len(report.Highlights), len(wantHighlights), report.Highlights)
	}
	for i, want := range wantHighlights {
		if report.Highlights[i] != want {
			t.Fatalf("Highlights[%d] = %q, want %q", i, report.Highlights[i], want)
		}
	}
}

func TestBuildDailyReportCardReturnsFourKPIs(t *testing.T) {
	events := []store.Event{
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(80), "duration_minutes": float64(10)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(80), "duration_minutes": float64(12)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "expressed", "amount_ml": float64(80), "duration_minutes": float64(8)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast", "duration_minutes": float64(12)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(120)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(180)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(139)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(140)}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "wet"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(150), "duration_minutes": float64(60)}},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(175), "duration_minutes": float64(35)}},
		{EventType: eventTypeBath, Attributes: map[string]any{}},
		{EventType: eventTypeTemperature, Attributes: map[string]any{}},
	}

	card := buildDailyReportCard(events)
	want := []dailyReportMetric{
		{Key: "feed", Count: 4, Label: "Feeds", Detail: "240 ml · 42 m"},
		{Key: "sleep", Count: 4, Label: "Sleep", Detail: "9 h 39 m"},
		{Key: "pump", Count: 2, Label: "Pump", Detail: "325 ml · 1 h 35 m"},
		{Key: "nappy", Count: 4, Label: "Nappies"},
	}
	if len(card.Metrics) != len(want) {
		t.Fatalf("Metrics = %#v, want %#v", card.Metrics, want)
	}
	for i := range want {
		if card.Metrics[i] != want[i] {
			t.Fatalf("Metrics[%d] = %#v, want %#v", i, card.Metrics[i], want[i])
		}
	}
}

func TestBuildDailyReportCardReturnsZeroTotalsForEmptyDay(t *testing.T) {
	card := buildDailyReportCard(nil)
	want := []dailyReportMetric{
		{Key: "feed", Count: 0, Label: "Feeds", Detail: "0 ml · 0 m"},
		{Key: "sleep", Count: 0, Label: "Sleep", Detail: "0 m"},
		{Key: "pump", Count: 0, Label: "Pump", Detail: "0 ml · 0 m"},
		{Key: "nappy", Count: 0, Label: "Nappies"},
	}
	for i := range want {
		if card.Metrics[i] != want[i] {
			t.Fatalf("Metrics[%d] = %#v, want %#v", i, card.Metrics[i], want[i])
		}
	}
}

func TestBuildDailyReportCardTotalsFeedVolumeAndDurationAcrossTypes(t *testing.T) {
	tests := []struct {
		name   string
		events []store.Event
		want   string
	}{
		{
			name: "formula feed",
			events: []store.Event{
				{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(80), "duration_minutes": float64(15)}},
			},
			want: "80 ml · 15 m",
		},
		{
			name: "breast feed",
			events: []store.Event{
				{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast", "duration_minutes": float64(35)}},
			},
			want: "0 ml · 35 m",
		},
		{
			name: "mixed feed types",
			events: []store.Event{
				{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(80), "duration_minutes": float64(20)}},
				{EventType: eventTypeFeed, Attributes: map[string]any{"type": "expressed", "amount_ml": float64(70), "duration_minutes": float64(10)}},
				{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast", "duration_minutes": float64(35)}},
			},
			want: "150 ml · 1 h 5 m",
		},
		{
			name: "feed without volume or duration",
			events: []store.Event{
				{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast"}},
			},
			want: "0 ml · 0 m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := buildDailyReportCard(tt.events)
			if got := card.Metrics[0].Detail; got != tt.want {
				t.Fatalf("feed detail = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatCompactDurationMinutes(t *testing.T) {
	tests := []struct {
		minutes int
		want    string
	}{
		{minutes: 0, want: "0 m"},
		{minutes: 42, want: "42 m"},
		{minutes: 60, want: "1 h"},
		{minutes: 87, want: "1 h 27 m"},
	}

	for _, tt := range tests {
		if got := formatCompactDurationMinutes(tt.minutes); got != tt.want {
			t.Errorf("formatCompactDurationMinutes(%d) = %q, want %q", tt.minutes, got, tt.want)
		}
	}
}

func TestDailyReportCardTitle(t *testing.T) {
	today := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		babyName string
		period   dailyReportPeriod
		want     string
	}{
		{
			name:     "today",
			babyName: "Yau Yau",
			period:   dailyReportPeriodFor(today, today),
			want:     "Yau Yau today",
		},
		{
			name:     "yesterday",
			babyName: "Yau Yau",
			period:   dailyReportPeriodFor(today.AddDate(0, 0, -1), today),
			want:     "Yau Yau yesterday",
		},
		{
			name:     "earlier day",
			babyName: "Yau Yau",
			period:   dailyReportPeriodFor(today.AddDate(0, 0, -2), today),
			want:     "Yau Yau on Wednesday",
		},
		{
			name:   "missing baby name",
			period: dailyReportPeriodFor(today, today),
			want:   "Today so far",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dailyReportCardTitle(tt.babyName, tt.period); got != tt.want {
				t.Fatalf("dailyReportCardTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDailyReportIgnoresStoredBreastAmount(t *testing.T) {
	window := timelineDayWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 11, 12, 30, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(window.From, window.From)

	report := buildDailyReport([]store.Event{
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast", "amount_ml": float64(80)}},
	}, window, window.To, period)

	if len(report.Highlights) != 1 || report.Highlights[0] != "1 breast feed." {
		t.Fatalf("Highlights = %#v, want one breast feed without ml", report.Highlights)
	}
}

func TestBuildDailyReportHandlesEmptyDay(t *testing.T) {
	window := timelineDayWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(window.From, window.From)

	report := buildDailyReport(nil, window, window.To, period)

	if report.Summary != "No events have been logged yet today." {
		t.Fatalf("Summary = %q", report.Summary)
	}
	if len(report.Highlights) != 1 || report.Highlights[0] != "Log the first event to start building today's report." {
		t.Fatalf("Highlights = %#v", report.Highlights)
	}
}

func TestBuildDailyReportUsesPastDayWording(t *testing.T) {
	window := timelineDayWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(window.From, window.From.AddDate(0, 0, 1))

	report := buildDailyReport([]store.Event{
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(70)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(120)}},
	}, window, window.To, period)

	if report.Title != "Yesterday summary" {
		t.Fatalf("Title = %q, want Yesterday summary", report.Title)
	}
	if report.Summary != "Yesterday had feeding and sleep logged." {
		t.Fatalf("Summary = %q", report.Summary)
	}
}

func TestBuildDailyReportClarifiesNappyChanges(t *testing.T) {
	window := timelineDayWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 11, 23, 59, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(window.From, window.From.AddDate(0, 0, 1))

	report := buildDailyReport([]store.Event{
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "wet"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "poo"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
	}, window, window.To, period)

	if len(report.Highlights) != 1 {
		t.Fatalf("len(Highlights) = %d, want 1: %#v", len(report.Highlights), report.Highlights)
	}
	if report.Highlights[0] != "3 nappy changes: 1 mixed, 1 wet only, 1 poo only." {
		t.Fatalf("Highlights[0] = %q", report.Highlights[0])
	}
}
