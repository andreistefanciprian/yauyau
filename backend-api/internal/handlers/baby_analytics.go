package handlers

import (
	"math"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

type BabyAnalytics struct {
	Timeline      TimelineAnalytics       `json:"timeline"`
	Chronology    ChronologyAnalytics     `json:"chronology"`
	Intervals     IntervalAnalytics       `json:"intervals"`
	Relationships []RelationshipAnalytics `json:"relationships"`
	Comparison    *ComparisonAnalytics    `json:"comparison,omitempty"`
}

type TimelineAnalytics struct {
	FirstEventAt *time.Time `json:"first_event_at,omitempty"`
	LastEventAt  *time.Time `json:"last_event_at,omitempty"`
	SpanMinutes  *int       `json:"span_minutes,omitempty"`
}

type ChronologyAnalytics struct {
	FirstFeedAt      *time.Time `json:"first_feed_at,omitempty"`
	LastFeedAt       *time.Time `json:"last_feed_at,omitempty"`
	LastNappyAt      *time.Time `json:"last_nappy_at,omitempty"`
	LastPooAt        *time.Time `json:"last_poo_at,omitempty"`
	LastSleepStartAt *time.Time `json:"last_sleep_start_at,omitempty"`
}

type IntervalAnalytics struct {
	Feeds  FeedIntervalAnalytics  `json:"feeds"`
	Sleeps SleepIntervalAnalytics `json:"sleeps"`
}

type FeedIntervalAnalytics struct {
	GapCount           int  `json:"gap_count"`
	AverageGapMinutes  *int `json:"average_gap_minutes,omitempty"`
	LongestGapMinutes  *int `json:"longest_gap_minutes,omitempty"`
	ShortestGapMinutes *int `json:"shortest_gap_minutes,omitempty"`
}

type SleepIntervalAnalytics struct {
	CompletedCount          int  `json:"completed_count"`
	OngoingCount            int  `json:"ongoing_count"`
	AverageDurationMinutes  *int `json:"average_duration_minutes,omitempty"`
	LongestDurationMinutes  *int `json:"longest_duration_minutes,omitempty"`
	ShortestDurationMinutes *int `json:"shortest_duration_minutes,omitempty"`
}

type RelationshipAnalytics struct {
	Key           string `json:"key"`
	From          string `json:"from"`
	To            string `json:"to"`
	WindowMinutes int    `json:"window_minutes"`
	Count         int    `json:"count"`
}

type ComparisonAnalytics struct {
	SelectedDaysIncluded int `json:"selected_days_included"`
	BaselineDaysIncluded int `json:"baseline_days_included"`

	SelectedAverageDailyFeedCount                    float64 `json:"selected_average_daily_feed_count"`
	BaselineAverageDailyFeedCount                    float64 `json:"baseline_average_daily_feed_count"`
	FeedCountDeltaFromBaselineDailyAverage           float64 `json:"feed_count_delta_from_baseline_daily_average"`
	SelectedAverageDailyNappyCount                   float64 `json:"selected_average_daily_nappy_count"`
	BaselineAverageDailyNappyCount                   float64 `json:"baseline_average_daily_nappy_count"`
	NappyCountDeltaFromBaselineDailyAverage          float64 `json:"nappy_count_delta_from_baseline_daily_average"`
	SelectedAverageDailyCompletedSleepCount          float64 `json:"selected_average_daily_completed_sleep_count"`
	BaselineAverageDailyCompletedSleepCount          float64 `json:"baseline_average_daily_completed_sleep_count"`
	CompletedSleepCountDeltaFromBaselineDailyAverage float64 `json:"completed_sleep_count_delta_from_baseline_daily_average"`
	SelectedAverageDailySleepMinutes                 float64 `json:"selected_average_daily_sleep_minutes"`
	BaselineAverageDailySleepMinutes                 float64 `json:"baseline_average_daily_sleep_minutes"`
	SleepMinutesDeltaFromBaselineDailyAverage        float64 `json:"sleep_minutes_delta_from_baseline_daily_average"`
}

type relationshipDefinition struct {
	Key    string
	From   string
	To     string
	Window time.Duration
}

var relationshipDefinitions = []relationshipDefinition{
	{Key: "feed_then_nappy", From: eventTypeFeed, To: eventTypeNappy, Window: 30 * time.Minute},
	{Key: "feed_then_sleep", From: eventTypeFeed, To: eventTypeSleep, Window: 45 * time.Minute},
	{Key: "bath_then_sleep", From: eventTypeBath, To: eventTypeSleep, Window: 60 * time.Minute},
}

func BuildBabyAnalytics(events []store.Event, loc *time.Location) BabyAnalytics {
	events = sortedAnalyticsEvents(events)
	return BabyAnalytics{
		Timeline:      buildTimelineAnalytics(events, loc),
		Chronology:    buildChronologyAnalytics(events, loc),
		Intervals:     buildIntervalAnalytics(events, loc),
		Relationships: buildRelationshipAnalytics(events, loc),
	}
}

func buildComparisonAnalytics(selectedDaysIncluded, baselineDaysIncluded int, selectedTotals, baselineTotals reportTotalsResponse) *ComparisonAnalytics {
	selectedFeedAverage := dailyAverage(selectedTotals.Feeds.Count, selectedDaysIncluded)
	baselineFeedAverage := dailyAverage(baselineTotals.Feeds.Count, baselineDaysIncluded)
	selectedNappyAverage := dailyAverage(selectedTotals.Nappies.Count, selectedDaysIncluded)
	baselineNappyAverage := dailyAverage(baselineTotals.Nappies.Count, baselineDaysIncluded)
	selectedCompletedSleepAverage := dailyAverage(selectedTotals.Sleeps.CompletedCount, selectedDaysIncluded)
	baselineCompletedSleepAverage := dailyAverage(baselineTotals.Sleeps.CompletedCount, baselineDaysIncluded)
	selectedSleepMinutesAverage := dailyAverage(selectedTotals.Sleeps.TotalDurationMinutes, selectedDaysIncluded)
	baselineSleepMinutesAverage := dailyAverage(baselineTotals.Sleeps.TotalDurationMinutes, baselineDaysIncluded)

	return &ComparisonAnalytics{
		SelectedDaysIncluded: selectedDaysIncluded,
		BaselineDaysIncluded: baselineDaysIncluded,

		SelectedAverageDailyFeedCount:                    selectedFeedAverage,
		BaselineAverageDailyFeedCount:                    baselineFeedAverage,
		FeedCountDeltaFromBaselineDailyAverage:           deltaFromBaselineAverage(selectedFeedAverage, baselineFeedAverage),
		SelectedAverageDailyNappyCount:                   selectedNappyAverage,
		BaselineAverageDailyNappyCount:                   baselineNappyAverage,
		NappyCountDeltaFromBaselineDailyAverage:          deltaFromBaselineAverage(selectedNappyAverage, baselineNappyAverage),
		SelectedAverageDailyCompletedSleepCount:          selectedCompletedSleepAverage,
		BaselineAverageDailyCompletedSleepCount:          baselineCompletedSleepAverage,
		CompletedSleepCountDeltaFromBaselineDailyAverage: deltaFromBaselineAverage(selectedCompletedSleepAverage, baselineCompletedSleepAverage),
		SelectedAverageDailySleepMinutes:                 selectedSleepMinutesAverage,
		BaselineAverageDailySleepMinutes:                 baselineSleepMinutesAverage,
		SleepMinutesDeltaFromBaselineDailyAverage:        deltaFromBaselineAverage(selectedSleepMinutesAverage, baselineSleepMinutesAverage),
	}
}

func sortedAnalyticsEvents(events []store.Event) []store.Event {
	sorted := append([]store.Event(nil), events...)
	sortEventsOldestFirst(sorted)
	return sorted
}

func buildTimelineAnalytics(events []store.Event, loc *time.Location) TimelineAnalytics {
	if len(events) == 0 {
		return TimelineAnalytics{}
	}

	first := localTimePtr(events[0].OccurredAt, loc)
	last := localTimePtr(events[len(events)-1].OccurredAt, loc)
	analytics := TimelineAnalytics{
		FirstEventAt: first,
		LastEventAt:  last,
	}
	if len(events) > 1 {
		spanMinutes := int(events[len(events)-1].OccurredAt.Sub(events[0].OccurredAt).Minutes())
		analytics.SpanMinutes = &spanMinutes
	}
	return analytics
}

func buildChronologyAnalytics(events []store.Event, loc *time.Location) ChronologyAnalytics {
	var analytics ChronologyAnalytics
	for _, ev := range events {
		switch ev.EventType {
		case eventTypeFeed:
			if analytics.FirstFeedAt == nil {
				analytics.FirstFeedAt = localTimePtr(ev.OccurredAt, loc)
			}
			analytics.LastFeedAt = localTimePtr(ev.OccurredAt, loc)
		case eventTypeNappy:
			analytics.LastNappyAt = localTimePtr(ev.OccurredAt, loc)
			if isPooNappy(ev) {
				analytics.LastPooAt = localTimePtr(ev.OccurredAt, loc)
			}
		case eventTypeSleep:
			analytics.LastSleepStartAt = localTimePtr(ev.OccurredAt, loc)
		}
	}
	return analytics
}

func buildIntervalAnalytics(events []store.Event, loc *time.Location) IntervalAnalytics {
	return IntervalAnalytics{
		Feeds:  buildFeedIntervalAnalytics(events, loc),
		Sleeps: buildSleepIntervalAnalytics(events),
	}
}

func buildFeedIntervalAnalytics(events []store.Event, loc *time.Location) FeedIntervalAnalytics {
	var gaps []int
	var previousFeedByLocalDate map[string]store.Event

	for _, ev := range events {
		if ev.EventType != eventTypeFeed {
			continue
		}
		localDate := ev.OccurredAt.In(loc).Format(time.DateOnly)
		if previousFeedByLocalDate == nil {
			previousFeedByLocalDate = map[string]store.Event{}
		}
		if previous, ok := previousFeedByLocalDate[localDate]; ok {
			gaps = append(gaps, int(ev.OccurredAt.Sub(previous.OccurredAt).Minutes()))
		}
		previousFeedByLocalDate[localDate] = ev
	}

	analytics := FeedIntervalAnalytics{GapCount: len(gaps)}
	if len(gaps) == 0 {
		return analytics
	}

	average, longest, shortest := summarizeMinutes(gaps)
	analytics.AverageGapMinutes = &average
	analytics.LongestGapMinutes = &longest
	analytics.ShortestGapMinutes = &shortest
	return analytics
}

func buildSleepIntervalAnalytics(events []store.Event) SleepIntervalAnalytics {
	var durations []int
	analytics := SleepIntervalAnalytics{}
	for _, ev := range events {
		if ev.EventType != eventTypeSleep {
			continue
		}
		durationMinutes, ok := attributeOptionalInt(ev.Attributes, "duration_minutes")
		if !ok {
			analytics.OngoingCount++
			continue
		}
		analytics.CompletedCount++
		durations = append(durations, durationMinutes)
	}
	if len(durations) == 0 {
		return analytics
	}

	average, longest, shortest := summarizeMinutes(durations)
	analytics.AverageDurationMinutes = &average
	analytics.LongestDurationMinutes = &longest
	analytics.ShortestDurationMinutes = &shortest
	return analytics
}

func buildRelationshipAnalytics(events []store.Event, loc *time.Location) []RelationshipAnalytics {
	analytics := make([]RelationshipAnalytics, 0, len(relationshipDefinitions))
	for _, definition := range relationshipDefinitions {
		analytics = append(analytics, RelationshipAnalytics{
			Key:           definition.Key,
			From:          definition.From,
			To:            definition.To,
			WindowMinutes: int(definition.Window.Minutes()),
			Count:         countRelationships(events, loc, definition),
		})
	}
	return analytics
}

func countRelationships(events []store.Event, loc *time.Location, definition relationshipDefinition) int {
	count := 0
	for sourceIndex, source := range events {
		if source.EventType != definition.From {
			continue
		}
		sourceLocalDate := source.OccurredAt.In(loc).Format(time.DateOnly)
		windowEnd := source.OccurredAt.Add(definition.Window)
		for _, target := range events[sourceIndex+1:] {
			if target.OccurredAt.After(windowEnd) {
				break
			}
			if !target.OccurredAt.After(source.OccurredAt) {
				continue
			}
			if target.EventType != definition.To {
				continue
			}
			if target.OccurredAt.In(loc).Format(time.DateOnly) != sourceLocalDate {
				continue
			}
			count++
			break
		}
	}
	return count
}

func isPooNappy(ev store.Event) bool {
	kind, ok := ev.Attributes["kind"].(string)
	if !ok {
		return false
	}
	return NappyKind(kind) == NappyKindPoo || NappyKind(kind) == NappyKindBoth
}

func localTimePtr(t time.Time, loc *time.Location) *time.Time {
	local := t.In(loc)
	return &local
}

func summarizeMinutes(values []int) (average int, longest int, shortest int) {
	total := 0
	longest = values[0]
	shortest = values[0]
	for _, value := range values {
		total += value
		if value > longest {
			longest = value
		}
		if value < shortest {
			shortest = value
		}
	}
	average = int(float64(total)/float64(len(values)) + 0.5)
	return average, longest, shortest
}

func roundComparisonValue(value float64) float64 {
	return math.Round(value*10) / 10
}

func dailyAverage(total, daysIncluded int) float64 {
	if daysIncluded <= 0 {
		return 0
	}
	return roundComparisonValue(float64(total) / float64(daysIncluded))
}

func deltaFromBaselineAverage(selectedAverage, baselineAverage float64) float64 {
	return roundComparisonValue(selectedAverage - baselineAverage)
}
