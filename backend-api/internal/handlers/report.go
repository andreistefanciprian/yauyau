package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const reportEventsLimit = 500

type dailyReportResponse struct {
	Title       string                   `json:"title"`
	Summary     string                   `json:"summary"`
	Highlights  []string                 `json:"highlights"`
	Card        *dailyReportCardResponse `json:"card,omitempty"`
	GeneratedAt time.Time                `json:"generated_at"`
	RangeStart  time.Time                `json:"range_start"`
	RangeEnd    time.Time                `json:"range_end"`
}

type dailyReportCardResponse struct {
	Metrics []dailyReportMetric `json:"metrics"`
}

type dailyReportMetric struct {
	Key    string `json:"key"`
	Count  int    `json:"count"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

type dailyReportStats struct {
	FeedCount         int
	MilkMl            int
	BreastFeeds       int
	BreastFeedMinutes int
	NappyCount        int
	WetOnlyNappies    int
	PooOnlyNappies    int
	MixedNappies      int
	SleepCount        int
	SleepMinutes      int
	PumpCount         int
	PumpMl            int
	BathCount         int
	ObservationCount  int
	TemperatureCount  int
	GrowthCount       int
}

type dailyReportPeriod struct {
	Title          string
	Subject        string
	Verb           string
	InProgress     bool
	EmptySummary   string
	EmptyHighlight string
}

// GetDailyReport returns a calendar-day report for the current baby in the
// baby's timezone. The KPI card is deterministic and backend-owned.
func (h *Handlers) GetDailyReport(w http.ResponseWriter, r *http.Request) {
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	window, generatedAt, period, err := dailyReportWindow(r.URL.Query().Get("date"), baby.Timezone)
	if err != nil {
		if errors.Is(err, errInvalidTimelineDate) {
			writeError(w, http.StatusBadRequest, "invalid timeline date")
			return
		}
		log.Printf("resolve daily report date: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve report window")
		return
	}

	events, err := h.Store.ListAllEvents(r.Context(), baby.FamilyID, baby.ID, window.From, window.To, reportEventsLimit)
	if err != nil {
		log.Printf("list daily report events: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load report")
		return
	}

	report := buildDailyReport(events, window, generatedAt, period)
	report.Title = dailyReportCardTitle(baby.Name, period)
	report.Card = buildDailyReportCard(events)
	writeJSON(w, http.StatusOK, report)
}

func dailyReportWindow(rawDate, timezone string) (timelineDayWindow, time.Time, dailyReportPeriod, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return timelineDayWindow{}, time.Time{}, dailyReportPeriod{}, fmt.Errorf("load baby timezone %q: %w", timezone, err)
	}

	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dayStart, err := timelineDateStart(rawDate, loc, now)
	if err != nil {
		return timelineDayWindow{}, time.Time{}, dailyReportPeriod{}, err
	}
	dayEnd := dayStart.AddDate(0, 0, 1)
	period := dailyReportPeriodFor(dayStart, todayStart)
	if dayStart.Equal(todayStart) {
		dayEnd = now
	}
	return timelineDayWindow{From: dayStart, To: dayEnd}, now, period, nil
}

func dailyReportPeriodFor(dayStart, todayStart time.Time) dailyReportPeriod {
	switch {
	case dayStart.Equal(todayStart):
		return dailyReportPeriod{
			Title:          "Today so far",
			Subject:        "Today",
			Verb:           "has",
			InProgress:     true,
			EmptySummary:   "No events have been logged yet today.",
			EmptyHighlight: "Log the first event to start building today's report.",
		}
	case dayStart.Equal(todayStart.AddDate(0, 0, -1)):
		return dailyReportPeriod{
			Title:          "Yesterday summary",
			Subject:        "Yesterday",
			Verb:           "had",
			EmptySummary:   "No events were logged yesterday.",
			EmptyHighlight: "Log an event for yesterday to build this summary.",
		}
	default:
		dayName := dayStart.Format("Monday")
		return dailyReportPeriod{
			Title:          dayName + " summary",
			Subject:        dayName,
			Verb:           "had",
			EmptySummary:   fmt.Sprintf("No events were logged on %s.", dayName),
			EmptyHighlight: "Log an event for this day to build this summary.",
		}
	}
}

func buildDailyReport(events []store.Event, window timelineDayWindow, generatedAt time.Time, period dailyReportPeriod) dailyReportResponse {
	stats := dailyReportStats{}
	for _, ev := range events {
		stats.add(ev)
	}

	return dailyReportResponse{
		Title:       period.Title,
		Summary:     dailyReportSummary(stats, period),
		Highlights:  dailyReportHighlights(stats, period),
		GeneratedAt: generatedAt,
		RangeStart:  window.From,
		RangeEnd:    window.To,
	}
}

func buildDailyReportCard(events []store.Event) *dailyReportCardResponse {
	stats := dailyReportStats{}
	for _, ev := range events {
		stats.add(ev)
	}

	return &dailyReportCardResponse{
		Metrics: []dailyReportMetric{
			{Key: "feed", Count: stats.FeedCount, Label: "Feeds", Detail: dailyReportFeedDetail(stats)},
			{Key: "sleep", Count: stats.SleepCount, Label: "Sleep", Detail: formatCompactDurationMinutes(stats.SleepMinutes)},
			{Key: "pump", Count: stats.PumpCount, Label: "Pump", Detail: fmt.Sprintf("%d ml", stats.PumpMl)},
			{Key: "nappy", Count: stats.NappyCount, Label: "Nappies", Detail: "changed"},
		},
	}
}

func dailyReportFeedDetail(stats dailyReportStats) string {
	parts := make([]string, 0, 2)
	if stats.MilkMl > 0 || stats.FeedCount == 0 {
		parts = append(parts, fmt.Sprintf("%d ml", stats.MilkMl))
	}
	if stats.BreastFeedMinutes > 0 {
		parts = append(parts, formatCompactDurationMinutes(stats.BreastFeedMinutes)+" breast")
	}
	if len(parts) == 0 {
		return "logged"
	}
	return strings.Join(parts, " · ")
}

func dailyReportCardTitle(babyName string, period dailyReportPeriod) string {
	babyName = strings.TrimSpace(babyName)
	if babyName == "" {
		return period.Title
	}
	if period.InProgress {
		return babyName + " today"
	}
	if period.Subject == "Yesterday" {
		return babyName + " yesterday"
	}
	return babyName + " on " + period.Subject
}

func formatCompactDurationMinutes(minutes int) string {
	hours := minutes / 60
	remainingMinutes := minutes % 60
	if hours == 0 {
		return fmt.Sprintf("%d min", remainingMinutes)
	}
	if remainingMinutes == 0 {
		return fmt.Sprintf("%d hr", hours)
	}
	return fmt.Sprintf("%d hr %d min", hours, remainingMinutes)
}

func (s *dailyReportStats) add(ev store.Event) {
	switch ev.EventType {
	case eventTypeFeed:
		s.FeedCount++
		feedType, _ := ev.Attributes["type"].(string)
		if feedType == string(FeedTypeBreast) {
			s.BreastFeeds++
			if duration, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
				s.BreastFeedMinutes += duration
			}
		} else if amount, ok := attributeInt(ev.Attributes, "amount_ml"); ok {
			s.MilkMl += amount
		}
	case eventTypeNappy:
		s.NappyCount++
		if kind, ok := ev.Attributes["kind"].(string); ok {
			switch NappyKind(kind) {
			case NappyKindWet:
				s.WetOnlyNappies++
			case NappyKindPoo:
				s.PooOnlyNappies++
			case NappyKindBoth:
				s.MixedNappies++
			}
		}
	case eventTypeSleep:
		s.SleepCount++
		if duration, ok := attributeInt(ev.Attributes, "duration_minutes"); ok {
			s.SleepMinutes += duration
		}
	case eventTypePump:
		s.PumpCount++
		if amount, ok := attributeInt(ev.Attributes, "amount_ml"); ok {
			s.PumpMl += amount
		}
	case eventTypeBath:
		s.BathCount++
	case eventTypeObservation:
		s.ObservationCount++
	case eventTypeTemperature:
		s.TemperatureCount++
	case eventTypeGrowthMeasurement:
		s.GrowthCount++
	}
}

func dailyReportSummary(stats dailyReportStats, period dailyReportPeriod) string {
	if stats.totalEvents() == 0 {
		return period.EmptySummary
	}

	parts := activeReportAreas(stats)
	suffix := "logged."
	if period.InProgress {
		suffix = "logged so far."
	}
	switch len(parts) {
	case 0:
		return period.Subject + " is starting to take shape."
	case 1:
		return fmt.Sprintf("%s %s %s %s", period.Subject, period.Verb, parts[0], suffix)
	case 2:
		return fmt.Sprintf("%s %s %s and %s %s", period.Subject, period.Verb, parts[0], parts[1], suffix)
	default:
		return fmt.Sprintf("%s %s %s, %s, and %s %s", period.Subject, period.Verb, parts[0], parts[1], parts[2], suffix)
	}
}

func dailyReportHighlights(stats dailyReportStats, period dailyReportPeriod) []string {
	if stats.totalEvents() == 0 {
		return []string{period.EmptyHighlight}
	}

	var highlights []string
	if stats.FeedCount > 0 {
		highlights = append(highlights, feedHighlight(stats))
	}
	if stats.NappyCount > 0 {
		highlights = append(highlights, nappyHighlight(stats))
	}
	if stats.SleepCount > 0 {
		highlights = append(highlights, sleepHighlight(stats))
	}
	if stats.PumpCount > 0 {
		highlights = append(highlights, fmt.Sprintf("%s recorded%s.", pluralize(stats.PumpCount, "pump", "pumps"), amountSuffix(stats.PumpMl)))
	}
	if stats.BathCount > 0 {
		highlights = append(highlights, pluralize(stats.BathCount, "bath", "baths")+" logged.")
	}
	if stats.ObservationCount > 0 {
		highlights = append(highlights, pluralize(stats.ObservationCount, "observation", "observations")+" captured.")
	}
	if stats.TemperatureCount > 0 {
		highlights = append(highlights, pluralize(stats.TemperatureCount, "temperature", "temperatures")+" recorded.")
	}
	if stats.GrowthCount > 0 {
		highlights = append(highlights, pluralize(stats.GrowthCount, "growth measurement", "growth measurements")+" recorded.")
	}
	return highlights
}

func (s dailyReportStats) totalEvents() int {
	return s.FeedCount + s.NappyCount + s.SleepCount + s.PumpCount + s.BathCount + s.ObservationCount + s.TemperatureCount + s.GrowthCount
}

func activeReportAreas(stats dailyReportStats) []string {
	var areas []string
	if stats.FeedCount > 0 {
		areas = append(areas, "feeding")
	}
	if stats.NappyCount > 0 {
		areas = append(areas, "nappies")
	}
	if stats.SleepCount > 0 {
		areas = append(areas, "sleep")
	}
	if stats.PumpCount > 0 {
		areas = append(areas, "pumping")
	}
	if stats.BathCount > 0 {
		areas = append(areas, "baths")
	}
	if stats.ObservationCount > 0 {
		areas = append(areas, "observations")
	}
	if stats.TemperatureCount > 0 {
		areas = append(areas, "temperatures")
	}
	if stats.GrowthCount > 0 {
		areas = append(areas, "growth")
	}
	return areas
}

func feedHighlight(stats dailyReportStats) string {
	if stats.BreastFeeds == stats.FeedCount && stats.MilkMl == 0 {
		return pluralize(stats.BreastFeeds, "breast feed", "breast feeds") + "."
	}
	detail := pluralize(stats.FeedCount, "feed", "feeds")
	if stats.MilkMl > 0 {
		detail += fmt.Sprintf(" with %d ml recorded", stats.MilkMl)
	}
	if stats.BreastFeeds > 0 {
		detail += fmt.Sprintf(" and %s", pluralize(stats.BreastFeeds, "breast feed", "breast feeds"))
	}
	return detail + "."
}

func nappyHighlight(stats dailyReportStats) string {
	detail := pluralize(stats.NappyCount, "nappy change", "nappy changes")
	var parts []string
	if stats.MixedNappies > 0 {
		parts = append(parts, pluralize(stats.MixedNappies, "mixed", "mixed"))
	}
	if stats.WetOnlyNappies > 0 {
		parts = append(parts, pluralize(stats.WetOnlyNappies, "wet only", "wet only"))
	}
	if stats.PooOnlyNappies > 0 {
		parts = append(parts, pluralize(stats.PooOnlyNappies, "poo only", "poo only"))
	}
	if len(parts) == 0 {
		return detail + " logged."
	}
	return fmt.Sprintf("%s: %s.", detail, strings.Join(parts, ", "))
}

func sleepHighlight(stats dailyReportStats) string {
	if stats.SleepMinutes == 0 {
		return pluralize(stats.SleepCount, "sleep", "sleeps") + " logged."
	}
	return fmt.Sprintf("%s totalling %s.", pluralize(stats.SleepCount, "sleep", "sleeps"), formatDurationMinutes(stats.SleepMinutes))
}

func amountSuffix(amount int) string {
	if amount == 0 {
		return ""
	}
	return fmt.Sprintf(" totalling %d ml", amount)
}

func formatDurationMinutes(minutes int) string {
	hours := minutes / 60
	remainingMinutes := minutes % 60
	if hours == 0 {
		return pluralize(remainingMinutes, "minute", "minutes")
	}
	if remainingMinutes == 0 {
		return pluralize(hours, "hour", "hours")
	}
	return fmt.Sprintf("%s %s", pluralize(hours, "hour", "hours"), pluralize(remainingMinutes, "minute", "minutes"))
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}
