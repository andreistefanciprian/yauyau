package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/reportemail"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

// reportEmailTrendDays is the number of calendar days shown in the report
// email's "Last 7 days" trend charts, including the report day itself.
const reportEmailTrendDays = 7

// dailyReportEmailTrend computes the last 7 calendar days of daily totals
// (feeds/sleep/nappies/pump), ending with the report day, for the email's
// trend charts. It fails soft: if events cannot be loaded, the email still
// sends, just without the trend section.
func (h *Handlers) dailyReportEmailTrend(ctx context.Context, job store.DailyReportEmailJob) []reportemail.TrendDay {
	loc, err := time.LoadLocation(job.BabyTimezone)
	if err != nil {
		slog.Warn("daily report email: failed to load baby timezone for trend", "error", err)
		return nil
	}

	reportDayStart := job.RangeStart.In(loc)
	windowStart := reportDayStart.AddDate(0, 0, -(reportEmailTrendDays - 1))
	windowEnd := job.RangeEnd

	events, err := h.Store.ListAllEvents(ctx, job.FamilyID, job.BabyID, windowStart, windowEnd, reportEventsLimit*reportEmailTrendDays)
	if err != nil {
		slog.Warn("daily report email: failed to load events for trend", "error", err)
		return nil
	}

	trend := make([]reportemail.TrendDay, 0, reportEmailTrendDays)
	for dayStart := windowStart; !dayStart.After(reportDayStart); dayStart = dayStart.AddDate(0, 0, 1) {
		dayEnd := dayStart.AddDate(0, 0, 1)
		if dayEnd.After(windowEnd) {
			dayEnd = windowEnd
		}

		totals := buildReportTotals(eventsForWindow(events, dayStart, dayEnd))
		trend = append(trend, reportemail.TrendDay{
			Label:               dayStart.Format("Mon"),
			SleepHours:          float64(totals.Sleeps.TotalDurationMinutes) / 60,
			FeedCount:           totals.Feeds.Count,
			FeedDurationMinutes: totals.Feeds.TotalDurationMinutes,
			FeedBottleMl:        totals.Feeds.TotalMl,
			NappyCount:          totals.Nappies.Count,
			PumpMl:              totals.Pumps.TotalMl,
			PumpDurationMinutes: totals.Pumps.TotalDurationMinutes,
		})
	}
	return trend
}
