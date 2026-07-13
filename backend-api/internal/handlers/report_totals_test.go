package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestBuildReportTotalsCountsFeedsAndNotes(t *testing.T) {
	events := []store.Event{
		{
			ID:        uuid.New(),
			EventType: eventTypeFeed,
			Attributes: map[string]any{
				"type":             "breast",
				"amount_ml":        float64(20),
				"duration_minutes": float64(15),
				"notes":            "short top-up",
			},
		},
		{
			ID:        uuid.New(),
			EventType: eventTypeFeed,
			Attributes: map[string]any{
				"type":      "formula",
				"amount_ml": float64(80),
			},
		},
		{
			ID:        uuid.New(),
			EventType: eventTypeFeed,
			Attributes: map[string]any{
				"type":             "expressed",
				"amount_ml":        float64(60),
				"duration_minutes": float64(10),
			},
		},
	}

	totals := buildReportTotals(events)

	if totals.EventCount != 3 {
		t.Fatalf("EventCount = %d, want 3", totals.EventCount)
	}
	if totals.Feeds.Count != 3 || totals.Feeds.BreastCount != 1 || totals.Feeds.FormulaCount != 1 || totals.Feeds.ExpressedCount != 1 {
		t.Fatalf("feed counts = %#v, want one feed of each type", totals.Feeds)
	}
	if totals.Feeds.TotalMl != 160 || totals.Feeds.BreastMl != 20 || totals.Feeds.FormulaMl != 80 || totals.Feeds.ExpressedMl != 60 {
		t.Fatalf("feed ml totals = %#v, want total 160 split by type", totals.Feeds)
	}
	if totals.Feeds.TotalDurationMinutes != 25 {
		t.Fatalf("TotalDurationMinutes = %d, want 25", totals.Feeds.TotalDurationMinutes)
	}
	if totals.Notes.EventsWithNotesCount != 1 || totals.Notes.ByEventType[eventTypeFeed] != 1 {
		t.Fatalf("note totals = %#v, want one feed note", totals.Notes)
	}
}

func TestBuildReportTotalsCountsCareEvents(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	events := []store.Event{
		{ID: uuid.New(), EventType: eventTypeNappy, OccurredAt: now, Attributes: map[string]any{"kind": "wet"}},
		{ID: uuid.New(), EventType: eventTypeNappy, OccurredAt: now.Add(time.Minute), Attributes: map[string]any{"kind": "poo", "poo_size": "small", "labels": []any{"mustard_yellow"}}},
		{ID: uuid.New(), EventType: eventTypeNappy, OccurredAt: now.Add(2 * time.Minute), Attributes: map[string]any{"kind": "both", "poo_size": "medium", "labels": []any{"mustard_yellow"}}},
		{ID: uuid.New(), EventType: eventTypeSleep, OccurredAt: now.Add(3 * time.Minute), Attributes: map[string]any{"duration_minutes": float64(90)}},
		{ID: uuid.New(), EventType: eventTypeSleep, OccurredAt: now.Add(4 * time.Minute), Attributes: map[string]any{}},
		{ID: uuid.New(), EventType: eventTypePump, OccurredAt: now.Add(5 * time.Minute), Attributes: map[string]any{"amount_ml": float64(120)}},
		{ID: uuid.New(), EventType: eventTypeBath, OccurredAt: now.Add(6 * time.Minute), Attributes: map[string]any{"type": "whole_body", "duration_minutes": float64(10)}},
		{ID: uuid.New(), EventType: eventTypeObservation, OccurredAt: now.Add(7 * time.Minute), Attributes: map[string]any{"category": "mood"}},
		{ID: uuid.New(), EventType: eventTypeTemperature, OccurredAt: now.Add(8 * time.Minute), Attributes: map[string]any{"temperature_c": float64(37.2), "method": "ear"}},
		{ID: uuid.New(), EventType: eventTypeTemperature, OccurredAt: now.Add(9 * time.Minute), Attributes: map[string]any{"temperature_c": float64(36.8), "method": "forehead"}},
	}

	totals := buildReportTotals(events)

	if totals.Nappies.Count != 3 || totals.Nappies.WetOnlyCount != 1 || totals.Nappies.PooOnlyCount != 1 || totals.Nappies.MixedCount != 1 {
		t.Fatalf("nappy totals = %#v, want one wet, one poo, one mixed", totals.Nappies)
	}
	if totals.Nappies.PooSizes["small"] != 1 || totals.Nappies.PooSizes["medium"] != 1 || totals.Nappies.Labels["mustard_yellow"] != 2 {
		t.Fatalf("nappy detail totals = %#v, want poo sizes and labels counted", totals.Nappies)
	}
	if totals.Sleeps.Count != 2 || totals.Sleeps.CompletedCount != 1 || totals.Sleeps.OngoingCount != 1 || totals.Sleeps.TotalDurationMinutes != 90 {
		t.Fatalf("sleep totals = %#v, want one completed 90m sleep and one ongoing sleep", totals.Sleeps)
	}
	if totals.Pumps.Count != 1 || totals.Pumps.TotalMl != 120 {
		t.Fatalf("pump totals = %#v, want one 120ml pump", totals.Pumps)
	}
	if totals.Baths.Count != 1 || totals.Baths.WholeBodyCount != 1 || totals.Baths.TotalDurationMinutes != 10 {
		t.Fatalf("bath totals = %#v, want one 10m whole-body bath", totals.Baths)
	}
	if totals.Observations.Count != 1 || totals.Observations.Categories["mood"] != 1 {
		t.Fatalf("observation totals = %#v, want one mood observation", totals.Observations)
	}
	if totals.Temperatures.Count != 2 || *totals.Temperatures.MinC != 36.8 || *totals.Temperatures.MaxC != 37.2 || *totals.Temperatures.LatestC != 36.8 {
		t.Fatalf("temperature totals = %#v, want min 36.8, max 37.2, latest 36.8", totals.Temperatures)
	}
	if totals.Temperatures.Methods["ear"] != 1 || totals.Temperatures.Methods["forehead"] != 1 {
		t.Fatalf("temperature methods = %#v, want ear and forehead counted", totals.Temperatures.Methods)
	}
}
