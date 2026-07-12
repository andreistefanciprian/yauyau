package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aiclient"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const reportEventsLimit = 500
const dailyReportType = "daily"
const dailyReportAIStaleAfter = 2 * time.Hour

type dailyReportResponse struct {
	Title              string    `json:"title"`
	Summary            string    `json:"summary"`
	Highlights         []string  `json:"highlights"`
	AISummary          string    `json:"ai_summary,omitempty"`
	PatternNotes       []string  `json:"pattern_notes,omitempty"`
	SuggestedQuestions []string  `json:"suggested_questions,omitempty"`
	GeneratedAt        time.Time `json:"generated_at"`
	RangeStart         time.Time `json:"range_start"`
	RangeEnd           time.Time `json:"range_end"`
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
	report, baby, events, stats, window, generatedAt, ok := h.dailyReportForRequest(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("include_ai") == "true" {
		h.applyCachedDailyReportAI(r.Context(), baby, events, stats, window, generatedAt, &report)
	}

	writeJSON(w, http.StatusOK, report)
}

func (h *Handlers) GenerateDailyReportAI(w http.ResponseWriter, r *http.Request) {
	report, baby, events, stats, window, generatedAt, ok := h.dailyReportForRequest(w, r)
	if !ok {
		return
	}
	h.generateDailyReportAI(r.Context(), baby, events, stats, window, generatedAt, &report)

	writeJSON(w, http.StatusOK, report)
}

func (h *Handlers) dailyReportForRequest(w http.ResponseWriter, r *http.Request) (dailyReportResponse, store.Baby, []store.Event, dailyReportStats, timelineRangeWindow, time.Time, bool) {
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return dailyReportResponse{}, store.Baby{}, nil, dailyReportStats{}, timelineRangeWindow{}, time.Time{}, false
	}

	window, generatedAt, err := dailyReportWindow(baby.Timezone)
	if err != nil {
		log.Printf("resolve daily report window: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve report window")
		return dailyReportResponse{}, store.Baby{}, nil, dailyReportStats{}, timelineRangeWindow{}, time.Time{}, false
	}

	events, err := h.Store.ListAllEvents(r.Context(), baby.FamilyID, baby.ID, window.From, window.To, reportEventsLimit)
	if err != nil {
		log.Printf("list daily report events: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load report")
		return dailyReportResponse{}, store.Baby{}, nil, dailyReportStats{}, timelineRangeWindow{}, time.Time{}, false
	}

	stats := summarizeDailyReport(events)
	report := buildDailyReportWithStats(stats, window, generatedAt)
	return report, baby, events, stats, window, generatedAt, true
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
	return buildDailyReportWithStats(summarizeDailyReport(events), window, generatedAt)
}

func summarizeDailyReport(events []store.Event) dailyReportStats {
	stats := dailyReportStats{}
	for _, ev := range events {
		stats.add(ev)
	}
	return stats
}

func buildDailyReportWithStats(stats dailyReportStats, window timelineRangeWindow, generatedAt time.Time) dailyReportResponse {
	return dailyReportResponse{
		Title:       "Today so far",
		Summary:     dailyReportSummary(stats),
		Highlights:  dailyReportHighlights(stats),
		GeneratedAt: generatedAt,
		RangeStart:  window.From,
		RangeEnd:    window.To,
	}
}

func (h *Handlers) applyCachedDailyReportAI(ctx context.Context, baby store.Baby, events []store.Event, stats dailyReportStats, window timelineRangeWindow, generatedAt time.Time, report *dailyReportResponse) {
	if len(events) == 0 {
		return
	}

	input := buildDailyReportAIInput(baby, events, stats, *report, window, generatedAt)
	inputHash, err := dailyReportInputHash(input)
	if err != nil {
		log.Printf("hash daily report AI input: %v", err)
		return
	}

	cacheRangeEnd := window.From.AddDate(0, 0, 1)
	cached, err := h.Store.GetAIReport(ctx, baby.FamilyID, baby.ID, dailyReportType, window.From, cacheRangeEnd, inputHash)
	if err == nil && generatedAt.Sub(cached.CreatedAt) < dailyReportAIStaleAfter {
		if output, ok := dailyReportAIOutputFromContent(cached.Content); ok {
			applyDailyReportAIOutput(report, output)
		}
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Printf("get daily AI report cache: %v", err)
	}
}

func (h *Handlers) generateDailyReportAI(ctx context.Context, baby store.Baby, events []store.Event, stats dailyReportStats, window timelineRangeWindow, generatedAt time.Time, report *dailyReportResponse) {
	if h.AI == nil || len(events) == 0 {
		return
	}

	input := buildDailyReportAIInput(baby, events, stats, *report, window, generatedAt)
	inputHash, err := dailyReportInputHash(input)
	if err != nil {
		log.Printf("hash daily report AI input: %v", err)
		return
	}

	cacheRangeEnd := window.From.AddDate(0, 0, 1)
	cached, err := h.Store.GetAIReport(ctx, baby.FamilyID, baby.ID, dailyReportType, window.From, cacheRangeEnd, inputHash)
	if err == nil && generatedAt.Sub(cached.CreatedAt) < dailyReportAIStaleAfter {
		if output, ok := dailyReportAIOutputFromContent(cached.Content); ok {
			applyDailyReportAIOutput(report, output)
			return
		}
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Printf("get daily AI report cache: %v", err)
	}

	output, model, err := h.AI.GenerateDailyReport(ctx, input)
	if err != nil {
		log.Printf("generate daily AI report: %v", err)
		return
	}

	if _, err := h.Store.SaveAIReport(ctx, baby.FamilyID, baby.ID, dailyReportType, window.From, cacheRangeEnd, inputHash, model, dailyReportAIContent(output)); err != nil {
		log.Printf("save daily AI report cache: %v", err)
	}
	applyDailyReportAIOutput(report, output)
}

func buildDailyReportAIInput(baby store.Baby, events []store.Event, stats dailyReportStats, report dailyReportResponse, window timelineRangeWindow, generatedAt time.Time) aiclient.DailyReportInput {
	return aiclient.DailyReportInput{
		ReportLabel: report.Title,
		LocalDate:   window.From.Format("2006-01-02"),
		Timezone:    baby.Timezone,
		CurrentTime: generatedAt,
		RangeStart:  window.From,
		RangeEnd:    generatedAt,
		Summary:     report.Summary,
		Highlights:  report.Highlights,
		Totals: aiclient.DailyReportTotals{
			Feeds:        stats.FeedCount,
			MilkMl:       stats.MilkMl,
			BreastFeeds:  stats.BreastFeeds,
			WetNappies:   stats.WetNappies,
			PooNappies:   stats.PooNappies,
			Sleeps:       stats.SleepCount,
			SleepMinutes: stats.SleepMinutes,
			Pumps:        stats.PumpCount,
			PumpMl:       stats.PumpMl,
			Baths:        stats.BathCount,
			Observations: stats.ObservationCount,
		},
		Events: dailyReportAIEvents(events),
	}
}

func dailyReportAIEvents(events []store.Event) []aiclient.DailyReportEvent {
	sorted := append([]store.Event(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].OccurredAt.Equal(sorted[j].OccurredAt) {
			return sorted[i].ID.String() < sorted[j].ID.String()
		}
		return sorted[i].OccurredAt.Before(sorted[j].OccurredAt)
	})

	aiEvents := make([]aiclient.DailyReportEvent, len(sorted))
	for i, ev := range sorted {
		aiEvents[i] = aiclient.DailyReportEvent{
			ID:         ev.ID.String(),
			Type:       ev.EventType,
			OccurredAt: ev.OccurredAt,
			Attributes: ev.Attributes,
		}
	}
	return aiEvents
}

func dailyReportInputHash(input aiclient.DailyReportInput) (string, error) {
	hashInput := struct {
		ReportLabel string                      `json:"report_label"`
		LocalDate   string                      `json:"local_date"`
		Timezone    string                      `json:"timezone"`
		RangeStart  time.Time                   `json:"range_start"`
		Summary     string                      `json:"summary"`
		Highlights  []string                    `json:"highlights"`
		Totals      aiclient.DailyReportTotals  `json:"totals"`
		Events      []aiclient.DailyReportEvent `json:"events"`
	}{
		ReportLabel: input.ReportLabel,
		LocalDate:   input.LocalDate,
		Timezone:    input.Timezone,
		RangeStart:  input.RangeStart,
		Summary:     input.Summary,
		Highlights:  input.Highlights,
		Totals:      input.Totals,
		Events:      input.Events,
	}

	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return "", fmt.Errorf("encoding hash input: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func applyDailyReportAIOutput(report *dailyReportResponse, output aiclient.DailyReportOutput) {
	report.AISummary = strings.TrimSpace(output.AISummary)
	report.PatternNotes = trimNonEmpty(output.PatternNotes)
	report.SuggestedQuestions = trimNonEmpty(output.SuggestedQuestions)
}

func dailyReportAIContent(output aiclient.DailyReportOutput) map[string]any {
	return map[string]any{
		"ai_summary":          output.AISummary,
		"pattern_notes":       output.PatternNotes,
		"suggested_questions": output.SuggestedQuestions,
	}
}

func dailyReportAIOutputFromContent(content map[string]any) (aiclient.DailyReportOutput, bool) {
	output := aiclient.DailyReportOutput{
		AISummary:          stringFromContent(content, "ai_summary"),
		PatternNotes:       stringSliceFromContent(content, "pattern_notes"),
		SuggestedQuestions: stringSliceFromContent(content, "suggested_questions"),
	}
	output.AISummary = strings.TrimSpace(output.AISummary)
	output.PatternNotes = trimNonEmpty(output.PatternNotes)
	output.SuggestedQuestions = trimNonEmpty(output.SuggestedQuestions)
	return output, output.AISummary != ""
}

func stringFromContent(content map[string]any, key string) string {
	value, _ := content[key].(string)
	return value
}

func stringSliceFromContent(content map[string]any, key string) []string {
	switch values := content[key].(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func trimNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
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
