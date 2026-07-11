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

	report := buildDailyReport([]store.Event{
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "formula", "amount_ml": float64(70)}},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": "breast"}},
		{EventType: eventTypeNappy, Attributes: map[string]any{"kind": "both"}},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(95)}},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(80)}},
		{EventType: eventTypeBath, Attributes: map[string]any{"type": "whole_body"}},
		{EventType: eventTypeObservation, Attributes: map[string]any{"text": "smiley"}},
	}, window, window.To)

	if report.Title != "Today so far" {
		t.Fatalf("Title = %q, want Today so far", report.Title)
	}
	if report.Summary != "Today has 2 feeds, 1 wet nappy/1 poo nappy, and 1 sleep (1 hour 35 minutes) logged so far." {
		t.Fatalf("Summary = %q", report.Summary)
	}

	wantHighlights := []string{
		"2 feeds with 70 ml recorded and 1 breast feed.",
		"1 wet nappy and 1 poo nappy logged.",
		"1 sleep totalling 1 hour 35 minutes.",
		"1 pump recorded totalling 80 ml.",
		"1 bath logged.",
		"1 observation captured.",
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

	report := buildDailyReport(nil, window, window.To)

	if report.Summary != "No events have been logged yet today." {
		t.Fatalf("Summary = %q", report.Summary)
	}
	if len(report.Highlights) != 1 || report.Highlights[0] != "Log the first event to start building today's report." {
		t.Fatalf("Highlights = %#v", report.Highlights)
	}
}
