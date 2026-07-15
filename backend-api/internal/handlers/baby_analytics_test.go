package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestBuildBabyAnalyticsTimelineAndChronology(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	events := []store.Event{
		{
			ID:         uuid.New(),
			EventType:  eventTypeFeed,
			OccurredAt: time.Date(2026, 7, 13, 7, 10, 0, 0, loc),
			Attributes: map[string]any{"type": string(FeedTypeBreast)},
		},
		{
			ID:         uuid.New(),
			EventType:  eventTypeNappy,
			OccurredAt: time.Date(2026, 7, 13, 8, 5, 0, 0, loc),
			Attributes: map[string]any{"kind": string(NappyKindWet)},
		},
		{
			ID:         uuid.New(),
			EventType:  eventTypeNappy,
			OccurredAt: time.Date(2026, 7, 13, 12, 30, 0, 0, loc),
			Attributes: map[string]any{"kind": string(NappyKindBoth)},
		},
		{
			ID:         uuid.New(),
			EventType:  eventTypeSleep,
			OccurredAt: time.Date(2026, 7, 13, 19, 10, 0, 0, loc),
			Attributes: map[string]any{"duration_minutes": 90},
		},
		{
			ID:         uuid.New(),
			EventType:  eventTypeFeed,
			OccurredAt: time.Date(2026, 7, 13, 21, 15, 0, 0, loc),
			Attributes: map[string]any{"type": string(FeedTypeFormula), "amount_ml": 70},
		},
	}

	analytics := BuildBabyAnalytics(events, loc)

	assertTimeEqual(t, analytics.Timeline.FirstEventAt, events[0].OccurredAt)
	assertTimeEqual(t, analytics.Timeline.LastEventAt, events[4].OccurredAt)
	if analytics.Timeline.SpanMinutes == nil || *analytics.Timeline.SpanMinutes != 845 {
		t.Fatalf("SpanMinutes = %v, want 845", analytics.Timeline.SpanMinutes)
	}
	assertTimeEqual(t, analytics.Chronology.FirstFeedAt, events[0].OccurredAt)
	assertTimeEqual(t, analytics.Chronology.LastFeedAt, events[4].OccurredAt)
	assertTimeEqual(t, analytics.Chronology.LastNappyAt, events[2].OccurredAt)
	assertTimeEqual(t, analytics.Chronology.LastPooAt, events[2].OccurredAt)
	assertTimeEqual(t, analytics.Chronology.LastSleepStartAt, events[3].OccurredAt)
	if analytics.Timeline.FirstEventAt.Location().String() != "Australia/Adelaide" {
		t.Fatalf("FirstEventAt location = %s, want Australia/Adelaide", analytics.Timeline.FirstEventAt.Location())
	}
}

func TestBuildBabyAnalyticsSortsEventsOldestFirst(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	firstID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	secondID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	firstAt := time.Date(2026, 7, 13, 7, 0, 0, 0, loc)
	secondAt := time.Date(2026, 7, 13, 10, 0, 0, 0, loc)

	analytics := BuildBabyAnalytics([]store.Event{
		{ID: secondID, EventType: eventTypeFeed, OccurredAt: secondAt, Attributes: map[string]any{"type": string(FeedTypeBreast)}},
		{ID: firstID, EventType: eventTypeFeed, OccurredAt: firstAt, Attributes: map[string]any{"type": string(FeedTypeBreast)}},
	}, loc)

	assertTimeEqual(t, analytics.Timeline.FirstEventAt, firstAt)
	assertTimeEqual(t, analytics.Timeline.LastEventAt, secondAt)
	if analytics.Intervals.Feeds.GapCount != 1 || analytics.Intervals.Feeds.AverageGapMinutes == nil || *analytics.Intervals.Feeds.AverageGapMinutes != 180 {
		t.Fatalf("feed gaps = %#v, want one 180 minute gap", analytics.Intervals.Feeds)
	}
}

func TestBuildBabyAnalyticsFeedGapsDoNotCrossLocalDates(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	events := []store.Event{
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 13, 7, 0, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeBreast)}},
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 13, 10, 0, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeFormula), "amount_ml": 70}},
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 13, 14, 0, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeExpressed), "amount_ml": 80}},
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 13, 23, 50, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeBreast)}},
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 14, 0, 10, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeBreast)}},
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 14, 3, 10, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeBreast)}},
	}

	analytics := BuildBabyAnalytics(events, loc)

	if analytics.Intervals.Feeds.GapCount != 4 {
		t.Fatalf("GapCount = %d, want 4", analytics.Intervals.Feeds.GapCount)
	}
	if analytics.Intervals.Feeds.ShortestGapMinutes == nil || *analytics.Intervals.Feeds.ShortestGapMinutes != 180 {
		t.Fatalf("ShortestGapMinutes = %v, want 180", analytics.Intervals.Feeds.ShortestGapMinutes)
	}
	if analytics.Intervals.Feeds.LongestGapMinutes == nil || *analytics.Intervals.Feeds.LongestGapMinutes != 590 {
		t.Fatalf("LongestGapMinutes = %v, want 590", analytics.Intervals.Feeds.LongestGapMinutes)
	}
	if analytics.Intervals.Feeds.AverageGapMinutes == nil || *analytics.Intervals.Feeds.AverageGapMinutes != 298 {
		t.Fatalf("AverageGapMinutes = %v, want 298", analytics.Intervals.Feeds.AverageGapMinutes)
	}
}

func TestBuildBabyAnalyticsSleepDurations(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	events := []store.Event{
		{ID: uuid.New(), EventType: eventTypeSleep, OccurredAt: time.Date(2026, 7, 13, 9, 0, 0, 0, loc), Attributes: map[string]any{"duration_minutes": 35}},
		{ID: uuid.New(), EventType: eventTypeSleep, OccurredAt: time.Date(2026, 7, 13, 12, 0, 0, 0, loc), Attributes: map[string]any{"duration_minutes": 90}},
		{ID: uuid.New(), EventType: eventTypeSleep, OccurredAt: time.Date(2026, 7, 13, 16, 0, 0, 0, loc), Attributes: map[string]any{"duration_minutes": 140}},
		{ID: uuid.New(), EventType: eventTypeSleep, OccurredAt: time.Date(2026, 7, 13, 20, 0, 0, 0, loc), Attributes: map[string]any{}},
	}

	analytics := BuildBabyAnalytics(events, loc)

	if analytics.Intervals.Sleeps.CompletedCount != 3 || analytics.Intervals.Sleeps.OngoingCount != 1 {
		t.Fatalf("sleep counts = %#v, want 3 completed and 1 ongoing", analytics.Intervals.Sleeps)
	}
	if analytics.Intervals.Sleeps.AverageDurationMinutes == nil || *analytics.Intervals.Sleeps.AverageDurationMinutes != 88 {
		t.Fatalf("AverageDurationMinutes = %v, want 88", analytics.Intervals.Sleeps.AverageDurationMinutes)
	}
	if analytics.Intervals.Sleeps.ShortestDurationMinutes == nil || *analytics.Intervals.Sleeps.ShortestDurationMinutes != 35 {
		t.Fatalf("ShortestDurationMinutes = %v, want 35", analytics.Intervals.Sleeps.ShortestDurationMinutes)
	}
	if analytics.Intervals.Sleeps.LongestDurationMinutes == nil || *analytics.Intervals.Sleeps.LongestDurationMinutes != 140 {
		t.Fatalf("LongestDurationMinutes = %v, want 140", analytics.Intervals.Sleeps.LongestDurationMinutes)
	}
}

func TestBuildBabyAnalyticsRelationshipsArePartitionedByLocalDate(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	events := []store.Event{
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 13, 8, 0, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeBreast)}},
		{ID: uuid.New(), EventType: eventTypeNappy, OccurredAt: time.Date(2026, 7, 13, 8, 20, 0, 0, loc), Attributes: map[string]any{"kind": string(NappyKindWet)}},
		{ID: uuid.New(), EventType: eventTypeNappy, OccurredAt: time.Date(2026, 7, 13, 8, 25, 0, 0, loc), Attributes: map[string]any{"kind": string(NappyKindWet)}},
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 13, 23, 50, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeBreast)}},
		{ID: uuid.New(), EventType: eventTypeNappy, OccurredAt: time.Date(2026, 7, 14, 0, 10, 0, 0, loc), Attributes: map[string]any{"kind": string(NappyKindWet)}},
		{ID: uuid.New(), EventType: eventTypeFeed, OccurredAt: time.Date(2026, 7, 14, 7, 0, 0, 0, loc), Attributes: map[string]any{"type": string(FeedTypeBreast)}},
		{ID: uuid.New(), EventType: eventTypeSleep, OccurredAt: time.Date(2026, 7, 14, 7, 30, 0, 0, loc), Attributes: map[string]any{"duration_minutes": 60}},
		{ID: uuid.New(), EventType: eventTypeBath, OccurredAt: time.Date(2026, 7, 14, 18, 0, 0, 0, loc), Attributes: map[string]any{"type": string(BathTypeWholeBody)}},
		{ID: uuid.New(), EventType: eventTypeSleep, OccurredAt: time.Date(2026, 7, 14, 18, 50, 0, 0, loc), Attributes: map[string]any{"duration_minutes": 45}},
	}

	analytics := BuildBabyAnalytics(events, loc)

	if got := relationshipCount(t, analytics.Relationships, "feed_then_nappy"); got != 1 {
		t.Fatalf("feed_then_nappy count = %d, want 1", got)
	}
	if got := relationshipCount(t, analytics.Relationships, "feed_then_sleep"); got != 1 {
		t.Fatalf("feed_then_sleep count = %d, want 1", got)
	}
	if got := relationshipCount(t, analytics.Relationships, "bath_then_sleep"); got != 1 {
		t.Fatalf("bath_then_sleep count = %d, want 1", got)
	}
}

func TestBuildComparisonAnalyticsNormalizesDailyAverages(t *testing.T) {
	comparison := buildComparisonAnalytics(
		2,
		7,
		reportTotalsResponse{
			Feeds:   reportFeedTotals{Count: 4},
			Nappies: reportNappyTotals{Count: 5},
			Sleeps:  reportSleepTotals{CompletedCount: 3, TotalDurationMinutes: 250},
		},
		reportTotalsResponse{
			Feeds:   reportFeedTotals{Count: 7},
			Nappies: reportNappyTotals{Count: 14},
			Sleeps:  reportSleepTotals{CompletedCount: 7, TotalDurationMinutes: 700},
		},
	)

	if comparison.SelectedDaysIncluded != 2 || comparison.BaselineDaysIncluded != 7 {
		t.Fatalf("days = %d/%d, want 2/7", comparison.SelectedDaysIncluded, comparison.BaselineDaysIncluded)
	}
	if comparison.SelectedAverageDailyFeedCount != 2 || comparison.BaselineAverageDailyFeedCount != 1 || comparison.FeedCountDeltaFromBaselineDailyAverage != 1 {
		t.Fatalf("feed comparison = %#v, want selected 2, baseline 1, delta 1", comparison)
	}
	if comparison.SelectedAverageDailyNappyCount != 2.5 || comparison.BaselineAverageDailyNappyCount != 2 || comparison.NappyCountDeltaFromBaselineDailyAverage != 0.5 {
		t.Fatalf("nappy comparison = %#v, want selected 2.5, baseline 2, delta 0.5", comparison)
	}
	if comparison.SelectedAverageDailyCompletedSleepCount != 1.5 || comparison.BaselineAverageDailyCompletedSleepCount != 1 || comparison.CompletedSleepCountDeltaFromBaselineDailyAverage != 0.5 {
		t.Fatalf("completed sleep comparison = %#v, want selected 1.5, baseline 1, delta 0.5", comparison)
	}
	if comparison.SelectedAverageDailySleepMinutes != 125 || comparison.BaselineAverageDailySleepMinutes != 100 || comparison.SleepMinutesDeltaFromBaselineDailyAverage != 25 {
		t.Fatalf("sleep minutes comparison = %#v, want selected 125, baseline 100, delta 25", comparison)
	}
}

func assertTimeEqual(t *testing.T, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("time = nil, want %s", want)
	}
	if !got.Equal(want) {
		t.Fatalf("time = %s, want %s", got, want)
	}
}

func relationshipCount(t *testing.T, relationships []RelationshipAnalytics, key string) int {
	t.Helper()
	for _, relationship := range relationships {
		if relationship.Key == key {
			return relationship.Count
		}
	}
	t.Fatalf("missing relationship %q in %#v", key, relationships)
	return 0
}
