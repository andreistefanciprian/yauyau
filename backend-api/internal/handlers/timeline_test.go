package handlers

import (
	"errors"
	"testing"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestOrderTimelineEventsFloatsOngoingFeedsPumpsAndSleeps(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	events := []store.Event{
		{EventType: eventTypeNappy, OccurredAt: now},
		{EventType: eventTypeSleep, Attributes: map[string]any{"duration_minutes": float64(60)}, OccurredAt: now.Add(-time.Hour)},
		{EventType: eventTypeFeed, Attributes: map[string]any{"type": string(FeedTypeBreast)}, OccurredAt: now.Add(-2 * time.Hour)},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(80), "ongoing": true}, OccurredAt: now.Add(-3 * time.Hour)},
		{EventType: eventTypeSleep, Attributes: map[string]any{}, OccurredAt: now.Add(-4 * time.Hour)},
		{EventType: eventTypePump, Attributes: map[string]any{"amount_ml": float64(70)}, OccurredAt: now.Add(-5 * time.Hour)},
		{EventType: eventTypeFeed, Attributes: map[string]any{"duration_minutes": float64(10)}, OccurredAt: now.Add(-6 * time.Hour)},
	}

	orderTimelineEvents(events)

	if events[0].EventType != eventTypeFeed || !isOngoingFeed(events[0]) {
		t.Fatalf("first event = %#v, want ongoing feed", events[0])
	}
	if events[1].EventType != eventTypePump || !isOngoingPump(events[1]) {
		t.Fatalf("second event = %#v, want ongoing pump", events[1])
	}
	if events[2].EventType != eventTypeSleep || !isOngoingSleep(events[2]) {
		t.Fatalf("third event = %#v, want ongoing sleep", events[2])
	}
	if events[3].EventType != eventTypeNappy {
		t.Fatalf("fourth event = %s, want nappy to preserve non-ongoing order", events[3].EventType)
	}
	if events[4].EventType != eventTypeSleep || isOngoingSleep(events[4]) {
		t.Fatalf("fifth event = %#v, want completed sleep to stay with regular events", events[4])
	}
	if events[5].EventType != eventTypePump || isOngoingPump(events[5]) {
		t.Fatalf("sixth event = %#v, want legacy pump to stay with regular events", events[5])
	}
}

func TestTimelineDayWindowForExplicitDateUsesBabyTimezone(t *testing.T) {
	window, err := timelineDayWindowFor("2026-07-11", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("timelineDayWindowFor returned error: %v", err)
	}

	loc, err := time.LoadLocation("Australia/Adelaide")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	wantFrom := time.Date(2026, 7, 11, 0, 0, 0, 0, loc)
	wantTo := wantFrom.AddDate(0, 0, 1)
	if !window.From.Equal(wantFrom) || !window.To.Equal(wantTo) {
		t.Fatalf("window = %s to %s, want %s to %s", window.From, window.To, wantFrom, wantTo)
	}
}

func TestTimelineDayWindowForRejectsInvalidDate(t *testing.T) {
	_, err := timelineDayWindowFor("day-2", "Australia/Adelaide")
	if !errors.Is(err, errInvalidTimelineDate) {
		t.Fatalf("error = %v, want errInvalidTimelineDate", err)
	}
}

func TestTimelineDayWindowForRejectsFutureDate(t *testing.T) {
	loc, err := time.LoadLocation("Australia/Adelaide")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format(time.DateOnly)

	_, err = timelineDayWindowFor(tomorrow, "Australia/Adelaide")
	if !errors.Is(err, errInvalidTimelineDate) {
		t.Fatalf("error = %v, want errInvalidTimelineDate", err)
	}
}
