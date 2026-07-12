package handlers

import (
	"testing"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aiclient"
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

func TestDailyReportInputHashIgnoresCurrentTime(t *testing.T) {
	input := aiclient.DailyReportInput{
		ReportLabel: "Today so far",
		LocalDate:   "2026-07-12",
		Timezone:    "Australia/Adelaide",
		CurrentTime: time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC),
		RangeStart:  time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
		RangeEnd:    time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC),
		Summary:     "Today has 1 feed, no nappies, and no sleep logged so far.",
		Highlights:  []string{"1 feed with 70 ml recorded."},
		Totals:      aiclient.DailyReportTotals{Feeds: 1, MilkMl: 70},
		Events: []aiclient.DailyReportEvent{
			{
				ID:         "event-1",
				Type:       "feed",
				OccurredAt: time.Date(2026, 7, 12, 7, 30, 0, 0, time.UTC),
				Attributes: map[string]any{"type": "formula", "amount_ml": float64(70)},
			},
		},
	}

	firstHash, err := dailyReportInputHash(input)
	if err != nil {
		t.Fatalf("dailyReportInputHash first: %v", err)
	}

	input.CurrentTime = input.CurrentTime.Add(30 * time.Minute)
	input.RangeEnd = input.RangeEnd.Add(30 * time.Minute)
	secondHash, err := dailyReportInputHash(input)
	if err != nil {
		t.Fatalf("dailyReportInputHash second: %v", err)
	}

	if firstHash != secondHash {
		t.Fatalf("hash changed when only current time changed: %s != %s", firstHash, secondHash)
	}

	input.Events[0].Attributes["amount_ml"] = float64(90)
	thirdHash, err := dailyReportInputHash(input)
	if err != nil {
		t.Fatalf("dailyReportInputHash third: %v", err)
	}
	if thirdHash == firstHash {
		t.Fatalf("hash did not change when event input changed: %s", thirdHash)
	}
}

func TestDailyReportAIOutputFromContent(t *testing.T) {
	output, ok := dailyReportAIOutputFromContent(map[string]any{
		"ai_summary":          " A calm morning so far. ",
		"pattern_notes":       []any{" Feeds are clustered early. ", ""},
		"suggested_questions": []any{"When was the last feed?"},
	})

	if !ok {
		t.Fatal("dailyReportAIOutputFromContent ok = false")
	}
	if output.AISummary != "A calm morning so far." {
		t.Fatalf("AISummary = %q", output.AISummary)
	}
	if len(output.PatternNotes) != 1 || output.PatternNotes[0] != "Feeds are clustered early." {
		t.Fatalf("PatternNotes = %#v", output.PatternNotes)
	}
	if len(output.SuggestedQuestions) != 1 || output.SuggestedQuestions[0] != "When was the last feed?" {
		t.Fatalf("SuggestedQuestions = %#v", output.SuggestedQuestions)
	}
}
