package handlers

import (
	"testing"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestBuildDailyReportSummarizesTimelineEvents(t *testing.T) {
	window := timelineRangeWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 11, 12, 30, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(0, window.From)

	report := buildDailyReport([]store.Event{
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(70)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(95)}},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(80)}},
		{EventType: eventTypeBath, Attributes: map[string]any{"type": "whole_body"}},
		{EventType: eventTypeObservation, Attributes: map[string]any{"text": "smiley"}},
		{EventType: eventTypeTemperature, Attributes: map[string]any{"temperature_c": float64(37.2)}},
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

func TestBuildDailyReportHandlesEmptyDay(t *testing.T) {
	window := timelineRangeWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(0, window.From)

	report := buildDailyReport(nil, window, window.To, period)

	if report.Summary != "No events have been logged yet today." {
		t.Fatalf("Summary = %q", report.Summary)
	}
	if len(report.Highlights) != 1 || report.Highlights[0] != "Log the first event to start building today's report." {
		t.Fatalf("Highlights = %#v", report.Highlights)
	}
}

func TestBuildDailyReportUsesPastDayWording(t *testing.T) {
	window := timelineRangeWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(1, window.From)

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
	window := timelineRangeWindow{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 11, 23, 59, 0, 0, time.UTC),
	}
	period := dailyReportPeriodFor(1, window.From)

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
