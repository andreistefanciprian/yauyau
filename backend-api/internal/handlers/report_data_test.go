package handlers

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestReportDataWindowForDefaultsToToday(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, loc)

	window, err := reportDataWindowFor("", "", loc, now)
	if err != nil {
		t.Fatalf("reportDataWindowFor returned error: %v", err)
	}

	if window.StartDate != "2026-07-13" || window.EndDate != "2026-07-13" {
		t.Fatalf("dates = %s to %s, want 2026-07-13 to 2026-07-13", window.StartDate, window.EndDate)
	}
	if !window.RangeEnd.Equal(now) {
		t.Fatalf("RangeEnd = %s, want now %s", window.RangeEnd, now)
	}
	if !window.IncludesToday || !window.IsPartial {
		t.Fatalf("IncludesToday/IsPartial = %v/%v, want true/true", window.IncludesToday, window.IsPartial)
	}
}

func TestReportDataWindowForRejectsInvalidRanges(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, loc)

	tests := []struct {
		name      string
		startDate string
		endDate   string
	}{
		{name: "missing end", startDate: "2026-07-13"},
		{name: "end before start", startDate: "2026-07-13", endDate: "2026-07-12"},
		{name: "future end", startDate: "2026-07-13", endDate: "2026-07-14"},
		{name: "too many days", startDate: "2026-06-12", endDate: "2026-07-13"},
		{name: "bad date", startDate: "day-2", endDate: "2026-07-13"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reportDataWindowFor(tt.startDate, tt.endDate, loc, now)
			if !errors.Is(err, errInvalidReportDataRange) {
				t.Fatalf("error = %v, want errInvalidReportDataRange", err)
			}
		})
	}
}

func TestBuildReportDataGroupsDaysAndNormalizesEvents(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, loc)
	window, err := reportDataWindowFor("2026-07-12", "2026-07-13", loc, now)
	if err != nil {
		t.Fatalf("reportDataWindowFor returned error: %v", err)
	}
	babyID := uuid.New()
	baby := store.Baby{
		ID:        babyID,
		FamilyID:  uuid.New(),
		Name:      "YauYau",
		Timezone:  "Australia/Adelaide",
		BirthDate: "2026-01-01",
	}

	later := store.Event{
		ID:         uuid.New(),
		BabyID:     babyID,
		EventType:  eventTypeFeed,
		OccurredAt: time.Date(2026, 7, 13, 8, 20, 0, 0, loc),
		Attributes: map[string]any{
			"type":             "expressed",
			"amount_ml":        float64(80),
			"duration_minutes": float64(10),
			"labels":           []any{"sleepy"},
			"notes":            "needed top-up",
		},
	}
	earlier := store.Event{
		ID:         uuid.New(),
		BabyID:     babyID,
		EventType:  eventTypeNappy,
		OccurredAt: time.Date(2026, 7, 12, 7, 10, 0, 0, loc),
		Attributes: map[string]any{
			"kind":     "both",
			"poo_size": "medium",
			"labels":   []any{"mustard_yellow"},
		},
	}

	resp := buildReportData(baby, window, loc, []store.Event{later, earlier})

	if resp.Baby.ID != babyID || resp.Baby.Name != "YauYau" {
		t.Fatalf("Baby = %#v", resp.Baby)
	}
	if resp.Baby.AgeDays == nil || *resp.Baby.AgeDays != 193 {
		t.Fatalf("AgeDays = %v, want 193", resp.Baby.AgeDays)
	}
	if resp.Range.DaysIncluded != 2 || !resp.Range.IncludesToday || !resp.Range.IsPartial {
		t.Fatalf("Range = %#v", resp.Range)
	}
	if len(resp.Days) != 2 {
		t.Fatalf("len(Days) = %d, want 2", len(resp.Days))
	}
	if resp.Days[0].LocalDate != "2026-07-12" || resp.Days[1].LocalDate != "2026-07-13" {
		t.Fatalf("day dates = %s, %s", resp.Days[0].LocalDate, resp.Days[1].LocalDate)
	}
	if len(resp.Days[0].Events) != 1 || resp.Days[0].Events[0].ID != earlier.ID {
		t.Fatalf("first day events = %#v, want earlier event", resp.Days[0].Events)
	}
	if len(resp.Days[1].Events) != 1 || resp.Days[1].Events[0].ID != later.ID {
		t.Fatalf("second day events = %#v, want later event", resp.Days[1].Events)
	}
	if resp.Totals.EventCount != 2 || resp.Totals.Feeds.Count != 1 || resp.Totals.Nappies.Count != 1 {
		t.Fatalf("range totals = %#v, want one feed and one nappy", resp.Totals)
	}
	if resp.Days[0].Totals.EventCount != 1 || resp.Days[0].Totals.Nappies.MixedCount != 1 {
		t.Fatalf("first day totals = %#v, want one mixed nappy", resp.Days[0].Totals)
	}
	if resp.Days[1].Totals.EventCount != 1 || resp.Days[1].Totals.Feeds.ExpressedMl != 80 {
		t.Fatalf("second day totals = %#v, want one 80ml expressed feed", resp.Days[1].Totals)
	}

	feed := resp.Days[1].Events[0]
	if feed.LocalDate != "2026-07-13" || feed.LocalTime != "08:20" {
		t.Fatalf("feed local date/time = %s %s", feed.LocalDate, feed.LocalTime)
	}
	if feed.Notes != "needed top-up" {
		t.Fatalf("feed Notes = %q", feed.Notes)
	}
	if len(feed.Labels) != 1 || feed.Labels[0] != "sleepy" {
		t.Fatalf("feed Labels = %#v", feed.Labels)
	}
	if feed.Attributes["amount_ml"] != 80 || feed.Attributes["duration_minutes"] != 10 {
		t.Fatalf("feed Attributes = %#v", feed.Attributes)
	}
	if _, ok := feed.Attributes["notes"]; ok {
		t.Fatalf("feed Attributes should not include notes: %#v", feed.Attributes)
	}
}

func TestBuildReportDataOrdersEqualTimestampsByEventID(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, loc)
	window, err := reportDataWindowFor("2026-07-13", "2026-07-13", loc, now)
	if err != nil {
		t.Fatalf("reportDataWindowFor returned error: %v", err)
	}
	occurredAt := time.Date(2026, 7, 13, 8, 20, 0, 0, loc)
	firstID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	secondID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	resp := buildReportData(store.Baby{
		ID:       uuid.New(),
		Name:     "YauYau",
		Timezone: "Australia/Adelaide",
	}, window, loc, []store.Event{
		{ID: secondID, EventType: eventTypeNappy, OccurredAt: occurredAt, Attributes: map[string]any{"kind": "wet"}},
		{ID: firstID, EventType: eventTypeFeed, OccurredAt: occurredAt, Attributes: map[string]any{"type": "expressed"}},
	})

	if len(resp.Days[0].Events) != 2 || resp.Days[0].Events[0].ID != firstID || resp.Days[0].Events[1].ID != secondID {
		t.Fatalf("events = %#v, want equal timestamps ordered by event ID", resp.Days[0].Events)
	}
}

func TestReportEventAttributesPreservesTemperaturePrecision(t *testing.T) {
	ev := store.Event{
		EventType: eventTypeTemperature,
		Attributes: map[string]any{
			"temperature_c": 37.2,
			"method":        "ear",
		},
	}

	attributes := reportEventAttributes(ev)
	if attributes["temperature_c"] != 37.2 {
		t.Fatalf("temperature_c = %#v, want 37.2", attributes["temperature_c"])
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}
