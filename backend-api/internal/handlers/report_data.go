package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const maxReportDataDays = 31

var errInvalidReportDataRange = errors.New("invalid report date range")

type reportDataResponse struct {
	Baby      reportBabyResponse     `json:"baby"`
	Range     reportRangeResponse    `json:"range"`
	Totals    reportTotalsResponse   `json:"totals"`
	Analytics BabyAnalytics          `json:"analytics"`
	Baseline  reportBaselineResponse `json:"baseline"`
	Days      []reportDayResponse    `json:"days"`
}

type reportBabyResponse struct {
	ID           uuid.UUID                   `json:"id"`
	Name         string                      `json:"name"`
	Timezone     string                      `json:"timezone"`
	BirthDate    string                      `json:"birth_date,omitempty"`
	AgeDays      *int                        `json:"age_days,omitempty"`
	LatestGrowth *reportLatestGrowthResponse `json:"latest_growth,omitempty"`
}

type reportLatestGrowthResponse struct {
	Weight            *reportLatestGrowthIntMeasurement   `json:"weight,omitempty"`
	Length            *reportLatestGrowthFloatMeasurement `json:"length,omitempty"`
	HeadCircumference *reportLatestGrowthFloatMeasurement `json:"head_circumference,omitempty"`
}

type reportLatestGrowthIntMeasurement struct {
	Grams      int       `json:"grams"`
	MeasuredAt time.Time `json:"measured_at"`
	AgeDays    *int      `json:"age_days,omitempty"`
}

type reportLatestGrowthFloatMeasurement struct {
	CM         float64   `json:"cm"`
	MeasuredAt time.Time `json:"measured_at"`
	AgeDays    *int      `json:"age_days,omitempty"`
}

type reportRangeResponse struct {
	StartDate     string    `json:"start_date"`
	EndDate       string    `json:"end_date"`
	DaysIncluded  int       `json:"days_included"`
	IncludesToday bool      `json:"includes_today"`
	IsPartial     bool      `json:"is_partial"`
	RangeStart    time.Time `json:"range_start"`
	RangeEnd      time.Time `json:"range_end"`
	GeneratedAt   time.Time `json:"generated_at"`
}

type reportDayResponse struct {
	LocalDate  string                `json:"local_date"`
	Label      string                `json:"label"`
	RangeStart time.Time             `json:"range_start"`
	RangeEnd   time.Time             `json:"range_end"`
	IsToday    bool                  `json:"is_today"`
	IsPartial  bool                  `json:"is_partial"`
	Report     dailyReportResponse   `json:"report"`
	Totals     reportTotalsResponse  `json:"totals"`
	Analytics  BabyAnalytics         `json:"analytics"`
	Events     []reportEventResponse `json:"events"`
}

type reportBaselineResponse struct {
	Range     reportRangeResponse  `json:"range"`
	Totals    reportTotalsResponse `json:"totals"`
	Analytics BabyAnalytics        `json:"analytics"`
}

type reportEventResponse struct {
	ID         uuid.UUID      `json:"id"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	LocalDate  string         `json:"local_date"`
	LocalTime  string         `json:"local_time"`
	Notes      string         `json:"notes,omitempty"`
	Labels     []string       `json:"labels,omitempty"`
	Attributes map[string]any `json:"attributes"`
}

type reportDataWindow struct {
	StartDate     string
	EndDate       string
	RangeStart    time.Time
	EndStart      time.Time
	RangeEnd      time.Time
	GeneratedAt   time.Time
	TodayStart    time.Time
	DaysIncluded  int
	IncludesToday bool
	IsPartial     bool
}

// GetReportData returns the canonical factual report-data payload for one to
// 31 local calendar days. It is deterministic backend-owned input for later
// email, MCP, and AI reporting; it does not call AI.
func (h *Handlers) GetReportData(w http.ResponseWriter, r *http.Request) {
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	reportData, _, err := h.buildReportDataForBaby(r.Context(), baby, r.URL.Query().Get("start_date"), r.URL.Query().Get("end_date"), time.Now())
	if err != nil {
		if errors.Is(err, errInvalidReportDataRange) {
			writeError(w, http.StatusBadRequest, "invalid report date range")
			return
		}
		log.Printf("build report data: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load report data")
		return
	}

	writeJSON(w, http.StatusOK, reportData)
}

func (h *Handlers) buildReportDataForBaby(ctx context.Context, baby store.Baby, rawStartDate, rawEndDate string, now time.Time) (reportDataResponse, reportDataWindow, error) {
	loc, err := time.LoadLocation(baby.Timezone)
	if err != nil {
		return reportDataResponse{}, reportDataWindow{}, fmt.Errorf("load baby timezone %q: %w", baby.Timezone, err)
	}

	window, err := reportDataWindowFor(rawStartDate, rawEndDate, loc, now.In(loc))
	if err != nil {
		return reportDataResponse{}, reportDataWindow{}, err
	}

	events, err := h.Store.ListAllEvents(ctx, baby.FamilyID, baby.ID, window.RangeStart, window.RangeEnd, reportEventsLimit*window.DaysIncluded)
	if err != nil {
		return reportDataResponse{}, reportDataWindow{}, fmt.Errorf("list report data events: %w", err)
	}

	baselineWindow := reportDataBaselineWindowFor(window)
	baselineEvents, err := h.Store.ListAllEvents(ctx, baby.FamilyID, baby.ID, baselineWindow.RangeStart, baselineWindow.RangeEnd, reportEventsLimit*baselineWindow.DaysIncluded)
	if err != nil {
		return reportDataResponse{}, reportDataWindow{}, fmt.Errorf("list report baseline events: %w", err)
	}

	latestGrowth, err := h.Store.GetBabyLatestGrowth(ctx, baby.FamilyID, baby.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return reportDataResponse{}, reportDataWindow{}, fmt.Errorf("get baby latest growth: %w", err)
	}

	return buildReportData(baby, latestGrowth, window, loc, events, baselineWindow, baselineEvents), window, nil
}

func reportDataWindowFor(rawStartDate, rawEndDate string, loc *time.Location, now time.Time) (reportDataWindow, error) {
	if rawStartDate == "" && rawEndDate == "" {
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		rawStartDate = todayStart.Format(time.DateOnly)
		rawEndDate = rawStartDate
	}
	if rawStartDate == "" || rawEndDate == "" {
		return reportDataWindow{}, errInvalidReportDataRange
	}

	start, err := timelineDateStart(rawStartDate, loc, now)
	if err != nil {
		return reportDataWindow{}, fmt.Errorf("%w: %w", errInvalidReportDataRange, err)
	}
	end, err := timelineDateStart(rawEndDate, loc, now)
	if err != nil {
		return reportDataWindow{}, fmt.Errorf("%w: %w", errInvalidReportDataRange, err)
	}
	if end.Before(start) {
		return reportDataWindow{}, errInvalidReportDataRange
	}

	daysIncluded := 1
	for cursor := start; cursor.Before(end); cursor = cursor.AddDate(0, 0, 1) {
		daysIncluded++
	}
	if daysIncluded > maxReportDataDays {
		return reportDataWindow{}, errInvalidReportDataRange
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	includesToday := !todayStart.Before(start) && !todayStart.After(end)
	rangeEnd := end.AddDate(0, 0, 1)
	if includesToday && end.Equal(todayStart) {
		rangeEnd = now
	}

	return reportDataWindow{
		StartDate:     start.Format(time.DateOnly),
		EndDate:       end.Format(time.DateOnly),
		RangeStart:    start,
		EndStart:      end,
		RangeEnd:      rangeEnd,
		GeneratedAt:   now,
		TodayStart:    todayStart,
		DaysIncluded:  daysIncluded,
		IncludesToday: includesToday,
		IsPartial:     includesToday,
	}, nil
}

func reportDataBaselineWindowFor(window reportDataWindow) reportDataWindow {
	start := window.RangeStart.AddDate(0, 0, -7)
	endStart := window.RangeStart.AddDate(0, 0, -1)
	return reportDataWindow{
		StartDate:     start.Format(time.DateOnly),
		EndDate:       endStart.Format(time.DateOnly),
		RangeStart:    start,
		EndStart:      endStart,
		RangeEnd:      window.RangeStart,
		GeneratedAt:   window.GeneratedAt,
		TodayStart:    window.TodayStart,
		DaysIncluded:  7,
		IncludesToday: false,
		IsPartial:     false,
	}
}

func buildReportData(baby store.Baby, latestGrowth store.BabyLatestGrowth, window reportDataWindow, loc *time.Location, events []store.Event, baselineWindow reportDataWindow, baselineEvents []store.Event) reportDataResponse {
	sortEventsOldestFirst(events)
	sortEventsOldestFirst(baselineEvents)

	days := make([]reportDayResponse, 0, window.DaysIncluded)
	for cursor := window.RangeStart; !cursor.After(window.EndStart); cursor = cursor.AddDate(0, 0, 1) {
		dayStart := cursor
		dayEnd := dayStart.AddDate(0, 0, 1)
		isToday := dayStart.Equal(window.TodayStart)
		if isToday {
			dayEnd = window.GeneratedAt
		}

		dayEvents := eventsForWindow(events, dayStart, dayEnd)
		reportEvents := make([]reportEventResponse, 0, len(dayEvents))
		for _, ev := range dayEvents {
			mapped := reportEventFromStore(ev, loc)
			reportEvents = append(reportEvents, mapped)
		}

		days = append(days, reportDayResponse{
			LocalDate:  dayStart.Format(time.DateOnly),
			Label:      reportDayLabel(dayStart, window.TodayStart),
			RangeStart: dayStart,
			RangeEnd:   dayEnd,
			IsToday:    isToday,
			IsPartial:  isToday,
			Report:     buildDailyReport(dayEvents, timelineDayWindow{From: dayStart, To: dayEnd}, window.GeneratedAt, dailyReportPeriodFor(dayStart, window.TodayStart)),
			Totals:     buildReportTotals(dayEvents),
			Analytics:  BuildBabyAnalytics(dayEvents, loc),
			Events:     reportEvents,
		})
	}

	totals := buildReportTotals(events)
	analytics := BuildBabyAnalytics(events, loc)
	baseline := buildReportBaseline(baselineWindow, loc, baselineEvents)
	analytics.Comparison = buildComparisonAnalytics(window.DaysIncluded, baselineWindow.DaysIncluded, totals, baseline.Totals)

	return reportDataResponse{
		Baby: reportBabyFromStore(baby, latestGrowth, window.GeneratedAt, loc),
		Range: reportRangeResponse{
			StartDate:     window.StartDate,
			EndDate:       window.EndDate,
			DaysIncluded:  window.DaysIncluded,
			IncludesToday: window.IncludesToday,
			IsPartial:     window.IsPartial,
			RangeStart:    window.RangeStart,
			RangeEnd:      window.RangeEnd,
			GeneratedAt:   window.GeneratedAt,
		},
		Totals:    totals,
		Analytics: analytics,
		Baseline:  baseline,
		Days:      days,
	}
}

func sortEventsOldestFirst(events []store.Event) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].ID.String() < events[j].ID.String()
		}
		return events[i].OccurredAt.Before(events[j].OccurredAt)
	})
}

func buildReportBaseline(window reportDataWindow, loc *time.Location, events []store.Event) reportBaselineResponse {
	return reportBaselineResponse{
		Range: reportRangeResponse{
			StartDate:     window.StartDate,
			EndDate:       window.EndDate,
			DaysIncluded:  window.DaysIncluded,
			IncludesToday: window.IncludesToday,
			IsPartial:     window.IsPartial,
			RangeStart:    window.RangeStart,
			RangeEnd:      window.RangeEnd,
			GeneratedAt:   window.GeneratedAt,
		},
		Totals:    buildReportTotals(events),
		Analytics: BuildBabyAnalytics(events, loc),
	}
}

func eventsForWindow(events []store.Event, from, to time.Time) []store.Event {
	var matched []store.Event
	for _, ev := range events {
		if !ev.OccurredAt.Before(from) && ev.OccurredAt.Before(to) {
			matched = append(matched, ev)
		}
	}
	return matched
}

func reportBabyFromStore(baby store.Baby, latestGrowth store.BabyLatestGrowth, generatedAt time.Time, loc *time.Location) reportBabyResponse {
	resp := reportBabyResponse{
		ID:        baby.ID,
		Name:      baby.Name,
		Timezone:  baby.Timezone,
		BirthDate: baby.BirthDate,
	}
	resp.LatestGrowth = reportLatestGrowthFromStore(baby, latestGrowth, loc)
	if baby.BirthDate != "" {
		if ageDays := babyAgeDaysAt(baby, generatedAt, loc); ageDays != nil {
			resp.AgeDays = ageDays
		}
	}
	return resp
}

func reportLatestGrowthFromStore(baby store.Baby, latestGrowth store.BabyLatestGrowth, loc *time.Location) *reportLatestGrowthResponse {
	resp := reportLatestGrowthResponse{}
	if latestGrowth.WeightGrams != nil && latestGrowth.WeightMeasuredAt != nil {
		measuredAt := latestGrowth.WeightMeasuredAt.In(loc)
		resp.Weight = &reportLatestGrowthIntMeasurement{
			Grams:      *latestGrowth.WeightGrams,
			MeasuredAt: measuredAt,
			AgeDays:    babyAgeDaysAt(baby, measuredAt, loc),
		}
	}
	if latestGrowth.LengthCM != nil && latestGrowth.LengthMeasuredAt != nil {
		measuredAt := latestGrowth.LengthMeasuredAt.In(loc)
		resp.Length = &reportLatestGrowthFloatMeasurement{
			CM:         *latestGrowth.LengthCM,
			MeasuredAt: measuredAt,
			AgeDays:    babyAgeDaysAt(baby, measuredAt, loc),
		}
	}
	if latestGrowth.HeadCircumferenceCM != nil && latestGrowth.HeadCircumferenceMeasuredAt != nil {
		measuredAt := latestGrowth.HeadCircumferenceMeasuredAt.In(loc)
		resp.HeadCircumference = &reportLatestGrowthFloatMeasurement{
			CM:         *latestGrowth.HeadCircumferenceCM,
			MeasuredAt: measuredAt,
			AgeDays:    babyAgeDaysAt(baby, measuredAt, loc),
		}
	}
	if resp.Weight == nil && resp.Length == nil && resp.HeadCircumference == nil {
		return nil
	}
	return &resp
}

func babyAgeDaysAt(baby store.Baby, at time.Time, loc *time.Location) *int {
	if baby.BirthDate == "" {
		return nil
	}
	birthDate, err := time.Parse(time.DateOnly, baby.BirthDate)
	if err != nil {
		return nil
	}
	localAt := at.In(loc)
	day := time.Date(localAt.Year(), localAt.Month(), localAt.Day(), 0, 0, 0, 0, loc)
	birthDay := time.Date(birthDate.Year(), birthDate.Month(), birthDate.Day(), 0, 0, 0, 0, loc)
	if birthDay.After(day) {
		return nil
	}
	ageDays := 0
	for cursor := birthDay; cursor.Before(day); cursor = cursor.AddDate(0, 0, 1) {
		ageDays++
	}
	return &ageDays
}

func reportDayLabel(dayStart, todayStart time.Time) string {
	switch {
	case dayStart.Equal(todayStart):
		return "Today"
	case dayStart.Equal(todayStart.AddDate(0, 0, -1)):
		return "Yesterday"
	default:
		return dayStart.Format("Monday")
	}
}

func reportEventFromStore(ev store.Event, loc *time.Location) reportEventResponse {
	localOccurredAt := ev.OccurredAt.In(loc)
	labels := reportEventLabels(ev)
	return reportEventResponse{
		ID:         ev.ID,
		Type:       ev.EventType,
		OccurredAt: ev.OccurredAt,
		LocalDate:  localOccurredAt.Format(time.DateOnly),
		LocalTime:  localOccurredAt.Format("15:04"),
		Notes:      reportEventNotes(ev),
		Labels:     labels,
		Attributes: reportEventAttributes(ev),
	}
}

func reportEventNotes(ev store.Event) string {
	if notes, ok := ev.Attributes["notes"].(string); ok {
		return notes
	}
	if ev.EventType == eventTypeNappy {
		if colour, ok := ev.Attributes["colour"].(string); ok {
			return colour
		}
	}
	return ""
}

func reportEventLabels(ev store.Event) []string {
	switch ev.EventType {
	case eventTypeFeed:
		labels, _ := feedLabelsFromAttribute(ev.Attributes["labels"])
		return labels
	case eventTypeNappy:
		labels, _ := nappyLabelsFromAttribute(ev.Attributes["labels"])
		return labels
	default:
		return nil
	}
}

func reportEventAttributes(ev store.Event) map[string]any {
	attributes := map[string]any{}
	for key, value := range ev.Attributes {
		if key == "notes" || key == "labels" || key == "colour" {
			continue
		}
		switch key {
		case "amount_ml", "duration_minutes":
			if key == "amount_ml" && ev.EventType == eventTypeFeed {
				if feedType, _ := ev.Attributes["type"].(string); feedType == string(FeedTypeBreast) {
					continue
				}
			}
			if intValue, ok := attributeOptionalInt(ev.Attributes, key); ok {
				attributes[key] = intValue
				continue
			}
		}
		attributes[key] = value
	}
	return attributes
}
