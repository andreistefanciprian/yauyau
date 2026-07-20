package handlers

import (
	"encoding/json"
	"strings"
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

func TestBuildDailyReportCardPrioritisesMetricsAndTellsSecondaryStory(t *testing.T) {
	period := dailyReportPeriodFor(time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC))
	events := []store.Event{
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(80)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(80)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "expressed", "amount_ml": float64(80)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "expressed", "amount_ml": float64(80)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(120)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(180)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(139)}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(140)}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "wet"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(150)}},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(175)}},
		{EventType: eventTypeBath, Attributes: map[string]any{}},
		{EventType: eventTypeTemperature, Attributes: map[string]any{}},
		{EventType: eventTypeGrowthMeasurement, Attributes: map[string]any{"weight_grams": float64(7200)}},
	}

	card := buildDailyReportCard(events, period, "Dad")
	if len(card.PrimaryMetrics) != 2 {
		t.Fatalf("PrimaryMetrics = %#v, want feed and sleep", card.PrimaryMetrics)
	}
	if got := card.PrimaryMetrics[0]; got.Count != "4 feeds" || got.Total != "320 ml" {
		t.Fatalf("feed metric = %#v", got)
	}
	if got := card.PrimaryMetrics[1]; got.Count != "4 sleeps" || got.Total != "9 hr 39 min" {
		t.Fatalf("sleep metric = %#v", got)
	}
	wantStory := "The day also included plenty of nappy changes, two pumping sessions totalling 325 ml, a bath, and a temperature check. A new growth check recorded 7.2 kg, a lovely milestone to remember."
	if card.Body != wantStory {
		t.Fatalf("Body = %q, want %q", card.Body, wantStory)
	}
	if strings.Contains(card.Body, "2 nappy") || strings.Contains(card.Body, "mixed") || strings.Contains(card.Body, "wet") {
		t.Fatalf("Body exposes nappy detail: %q", card.Body)
	}
	if card.Closing != "You've got this, Dad." {
		t.Fatalf("Closing = %q", card.Closing)
	}
}

func TestBuildHistoricalDailyReportCardOmitsFillerAndUsesLatestGrowthValues(t *testing.T) {
	day := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	period := dailyReportPeriodFor(day, day.AddDate(0, 0, 2))
	events := []store.Event{
		{
			EventType:  eventTypeGrowthMeasurement,
			OccurredAt: day.Add(8 * time.Hour),
			Attributes: map[string]any{
				"weight_grams":          float64(3400),
				"length_cm":             float64(51.8),
				"head_circumference_cm": float64(35.9),
			},
		},
		{
			EventType:  eventTypeGrowthMeasurement,
			OccurredAt: day.Add(12 * time.Hour),
			Attributes: map[string]any{
				"weight_grams":          float64(3500),
				"length_cm":             float64(52.4),
				"head_circumference_cm": float64(36.1),
			},
		},
	}

	card := buildDailyReportCard(events, period, "Father")

	wantStory := "A new growth check recorded 3.5 kg, a length of 52.4 cm, and a head circumference of 36.1 cm, a lovely milestone to remember."
	if card.Body != wantStory {
		t.Fatalf("Body = %q, want %q", card.Body, wantStory)
	}
	if card.Closing != "" {
		t.Fatalf("Closing = %q, want empty historical closing", card.Closing)
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal historical card: %v", err)
	}
	if strings.Contains(string(raw), "closing") {
		t.Fatalf("historical card includes today-only fields: %s", raw)
	}
}

func TestBuildHistoricalDailyReportCardKeepsEmptyDayMessage(t *testing.T) {
	day := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	period := dailyReportPeriodFor(day, day.AddDate(0, 0, 2))

	card := buildDailyReportCard(nil, period, "Father")

	if card.Body != "No events were logged on Wednesday." {
		t.Fatalf("Body = %q, want historical empty-day message", card.Body)
	}
	if card.Closing != "" {
		t.Fatalf("historical closing = %q, want empty", card.Closing)
	}
}

func TestBuildDailyReportCardHandlesMissingMeasurementsAndHistory(t *testing.T) {
	tests := []struct {
		name        string
		events      []store.Event
		period      dailyReportPeriod
		wantMetrics []dailyReportPrimaryMetric
	}{
		{
			name:        "breast feed has no invented volume or sleep",
			events:      []store.Event{{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast"}}},
			period:      dailyReportPeriodFor(time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)),
			wantMetrics: []dailyReportPrimaryMetric{{Count: "1 feed"}},
		},
		{
			name:        "historical day and missing name",
			events:      []store.Event{{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(60)}}},
			period:      dailyReportPeriodFor(time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)),
			wantMetrics: []dailyReportPrimaryMetric{{Count: "1 sleep", Total: "1 hr"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := buildDailyReportCard(tt.events, tt.period, "")
			if len(card.PrimaryMetrics) != len(tt.wantMetrics) {
				t.Fatalf("PrimaryMetrics = %#v, want %#v", card.PrimaryMetrics, tt.wantMetrics)
			}
			for i, want := range tt.wantMetrics {
				if card.PrimaryMetrics[i] != want {
					t.Fatalf("PrimaryMetrics[%d] = %#v, want %#v", i, card.PrimaryMetrics[i], want)
				}
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
