package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const reportEventsLimit = 500

type dailyReportResponse struct {
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Highlights  []string  `json:"highlights"`
	GeneratedAt time.Time `json:"generated_at"`
	RangeStart  time.Time `json:"range_start"`
	RangeEnd    time.Time `json:"range_end"`
}

type dailyReportStats struct {
	FeedCount        int
	MilkMl           int
	BreastFeeds      int
	WetNappies       int
	PooNappies       int
	SleepCount       int
	SleepMinutes     int
	PumpCount        int
	PumpMl           int
	BathCount        int
	ObservationCount int
}

// GetDailyReport returns a calendar-day report for the current baby in the
// baby's timezone. This first version is deterministic and backend-owned; an
// AI client can later enrich the same response shape without moving report
// logic into thin clients.
func (h *Handlers) GetDailyReport(w http.ResponseWriter, r *http.Request) {
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	window, generatedAt, err := dailyReportWindow(baby.Timezone)
	if err != nil {
		log.Printf("resolve daily report window: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve report window")
		return
	}

	events, err := h.Store.ListAllEvents(r.Context(), baby.FamilyID, baby.ID, window.From, window.To, reportEventsLimit)
	if err != nil {
		log.Printf("list daily report events: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load report")
		return
	}

	writeJSON(w, http.StatusOK, buildDailyReport(events, window, generatedAt))
}

func dailyReportWindow(timezone string) (timelineRangeWindow, time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return timelineRangeWindow{}, time.Time{}, fmt.Errorf("load baby timezone %q: %w", timezone, err)
	}

	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return timelineRangeWindow{From: start, To: now}, now, nil
}

func buildDailyReport(events []store.Event, window timelineRangeWindow, generatedAt time.Time) dailyReportResponse {
	stats := dailyReportStats{}
	for _, ev := range events {
		stats.add(ev)
	}

	return dailyReportResponse{
		Title:       "Today so far",
		Summary:     dailyReportSummary(stats),
		Highlights:  dailyReportHighlights(stats),
		GeneratedAt: generatedAt,
		RangeStart:  window.From,
		RangeEnd:    window.To,
	}
}

func (s *dailyReportStats) add(ev store.Event) {
	switch ev.EventType {
	case eventTypeFeed:
		s.FeedCount++
		if amount, ok := attributeInt(ev.Attributes, "amount_ml"); ok {
			s.MilkMl += amount
		}
		if feedType, ok := ev.Attributes["type"].(string); ok && feedType == string(FeedTypeBreast) {
			s.BreastFeeds++
		}
	case eventTypeNappy:
		if kind, ok := ev.Attributes["kind"].(string); ok {
			switch NappyKind(kind) {
			case NappyKindWet:
				s.WetNappies++
			case NappyKindPoo:
				s.PooNappies++
			case NappyKindBoth:
				s.WetNappies++
				s.PooNappies++
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
	}
}

func dailyReportSummary(stats dailyReportStats) string {
	if stats.totalEvents() == 0 {
		return "No events have been logged yet today."
	}

	return fmt.Sprintf(
		"Today has %s, %s, and %s logged so far.",
		pluralize(stats.FeedCount, "feed", "feeds"),
		nappySummary(stats),
		sleepSummary(stats),
	)
}

func dailyReportHighlights(stats dailyReportStats) []string {
	if stats.totalEvents() == 0 {
		return []string{"Log the first event to start building today's report."}
	}

	highlights := []string{
		feedHighlight(stats),
		nappyHighlight(stats),
		sleepHighlight(stats),
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
	return highlights
}

func (s dailyReportStats) totalEvents() int {
	return s.FeedCount + s.WetNappies + s.PooNappies + s.SleepCount + s.PumpCount + s.BathCount + s.ObservationCount
}

func feedHighlight(stats dailyReportStats) string {
	if stats.FeedCount == 0 {
		return "No feeds logged yet."
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
	if stats.WetNappies == 0 && stats.PooNappies == 0 {
		return "No nappies logged yet."
	}
	return fmt.Sprintf("%s and %s logged.", pluralize(stats.WetNappies, "wet nappy", "wet nappies"), pluralize(stats.PooNappies, "poo nappy", "poo nappies"))
}

func sleepHighlight(stats dailyReportStats) string {
	if stats.SleepCount == 0 {
		return "No sleep logged yet."
	}
	if stats.SleepMinutes == 0 {
		return pluralize(stats.SleepCount, "sleep", "sleeps") + " logged."
	}
	return fmt.Sprintf("%s totalling %s.", pluralize(stats.SleepCount, "sleep", "sleeps"), formatDurationMinutes(stats.SleepMinutes))
}

func nappySummary(stats dailyReportStats) string {
	if stats.WetNappies == 0 && stats.PooNappies == 0 {
		return "no nappies"
	}
	return fmt.Sprintf("%s/%s", pluralize(stats.WetNappies, "wet nappy", "wet nappies"), pluralize(stats.PooNappies, "poo nappy", "poo nappies"))
}

func sleepSummary(stats dailyReportStats) string {
	if stats.SleepCount == 0 {
		return "no sleep"
	}
	if stats.SleepMinutes == 0 {
		return pluralize(stats.SleepCount, "sleep", "sleeps")
	}
	return fmt.Sprintf("%s (%s)", pluralize(stats.SleepCount, "sleep", "sleeps"), formatDurationMinutes(stats.SleepMinutes))
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
