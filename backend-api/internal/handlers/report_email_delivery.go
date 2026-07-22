package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aireport"
	"github.com/andreistefanciprian/yauli/backend-api/internal/reportemail"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

// DailyReportEmailRunResult summarizes one scheduler-triggered run. It is
// returned to future callers/loggers without exposing recipient addresses.
type DailyReportEmailRunResult struct {
	DueJobs int
	Skipped int
	Sent    int
	Failed  int
}

// SendDueDailyReportEmails processes report-email jobs that are due as of now.
// It has no HTTP route yet; a later Railway scheduler trigger can call this
// method through a tiny endpoint or worker command.
func (h *Handlers) SendDueDailyReportEmails(ctx context.Context, now time.Time) (DailyReportEmailRunResult, error) {
	jobs, err := h.Store.ListDueDailyReportEmailJobs(ctx, now)
	if err != nil {
		return DailyReportEmailRunResult{}, fmt.Errorf("listing due daily report email jobs: %w", err)
	}

	result := DailyReportEmailRunResult{DueJobs: len(jobs)}
	for _, job := range jobs {
		outcome, err := h.sendDailyReportEmail(ctx, job, now)
		if err != nil {
			return result, err
		}
		switch outcome {
		case dailyReportEmailOutcomeSkipped:
			result.Skipped++
		case dailyReportEmailOutcomeSent:
			result.Sent++
		case dailyReportEmailOutcomeFailed:
			result.Failed++
		}
	}

	return result, nil
}

type dailyReportEmailOutcome string

const (
	dailyReportEmailOutcomeSkipped dailyReportEmailOutcome = "skipped"
	dailyReportEmailOutcomeSent    dailyReportEmailOutcome = "sent"
	dailyReportEmailOutcomeFailed  dailyReportEmailOutcome = "failed"
)

// sendDailyReportEmail owns one recipient/window attempt. It creates the
// duplicate-send guard before doing expensive AI or provider work.
func (h *Handlers) sendDailyReportEmail(ctx context.Context, job store.DailyReportEmailJob, now time.Time) (dailyReportEmailOutcome, error) {
	delivery, err := h.Store.CreateAIReportEmailDelivery(ctx, store.AIReportEmailDelivery{
		FamilyID:        job.FamilyID,
		BabyID:          job.BabyID,
		RecipientUserID: job.RecipientUserID,
		RecipientEmail:  job.RecipientEmail,
		ReportType:      job.ReportType,
		RangeStart:      job.RangeStart,
		RangeEnd:        job.RangeEnd,
		ScheduledFor:    job.ScheduledFor,
	})
	if err != nil {
		return "", fmt.Errorf("creating AI report email delivery: %w", err)
	}
	if delivery.Status == store.AIReportEmailDeliveryStatusSent {
		return dailyReportEmailOutcomeSkipped, nil
	}
	delivery, err = h.Store.ClaimAIReportEmailDelivery(ctx, delivery.ID, now)
	if errors.Is(err, store.ErrNotFound) {
		return dailyReportEmailOutcomeSkipped, nil
	}
	if err != nil {
		return "", fmt.Errorf("claiming AI report email delivery: %w", err)
	}

	cached, output, err := h.dailyReportEmailContent(ctx, job, now)
	if err != nil {
		return h.markDailyReportEmailFailed(ctx, delivery.ID, err, now)
	}

	if h.ReportEmailSender == nil {
		return h.markDailyReportEmailFailed(ctx, delivery.ID, errors.New("report email sender is not configured"), now)
	}
	providerMessageID, err := h.ReportEmailSender.SendReportEmail(ctx, reportemail.Report{
		RecipientEmail: job.RecipientEmail,
		BabyName:       job.BabyName,
		ReportType:     job.ReportType,
		StartDate:      job.StartDate,
		EndDate:        job.EndDate,
		Output:         output,
		Card:           h.dailyReportEmailCard(ctx, job),
	})
	if err != nil {
		return h.markDailyReportEmailFailed(ctx, delivery.ID, err, now)
	}

	if _, err := h.Store.MarkAIReportEmailDeliverySent(ctx, delivery.ID, cached.ID, providerMessageID, now); err != nil {
		return "", fmt.Errorf("marking AI report email delivery sent: %w", err)
	}
	return dailyReportEmailOutcomeSent, nil
}

// dailyReportEmailContent gets the channel-neutral AI report for this job and
// decodes it into the renderer contract used by reportemail.
func (h *Handlers) dailyReportEmailContent(ctx context.Context, job store.DailyReportEmailJob, now time.Time) (store.AIReportCache, aireport.Output, error) {
	baby := store.Baby{
		ID:       job.BabyID,
		FamilyID: job.FamilyID,
		Name:     job.BabyName,
		Timezone: job.BabyTimezone,
	}
	result, err := h.loadOrCreateAIReport(ctx, baby, aiReportRequest{
		ReportType: job.ReportType,
		StartDate:  job.StartDate,
		EndDate:    job.EndDate,
		Locale:     defaultAIReportLocale,
	}, now)
	if err != nil {
		return store.AIReportCache{}, aireport.Output{}, err
	}

	contentJSON, err := validateAIReportOutput(result.Cache.ContentJSON)
	if err != nil {
		return store.AIReportCache{}, aireport.Output{}, err
	}
	var output aireport.Output
	if err := json.Unmarshal(contentJSON, &output); err != nil {
		return store.AIReportCache{}, aireport.Output{}, fmt.Errorf("decoding AI report output: %w", err)
	}

	return result.Cache, output, nil
}

// dailyReportEmailCard computes the same deterministic KPI counts the web
// app's daily report card shows (feeds/sleep/pump/nappies), for the email's
// summary card. It fails soft: if events cannot be loaded, the email still
// sends with its AI-generated content, just without the KPI card.
func (h *Handlers) dailyReportEmailCard(ctx context.Context, job store.DailyReportEmailJob) []reportemail.CardMetric {
	events, err := h.Store.ListAllEvents(ctx, job.FamilyID, job.BabyID, job.RangeStart, job.RangeEnd, reportEventsLimit)
	if err != nil {
		slog.Warn("daily report email: failed to load events for KPI card", "error", err)
		return nil
	}

	stats := dailyReportStats{}
	for _, ev := range events {
		stats.add(ev)
	}

	return []reportemail.CardMetric{
		{Label: "Feeds", Count: stats.FeedCount, Detail: dailyReportFeedDetail(stats)},
		{Label: "Sleep", Count: stats.SleepCount, Detail: formatCompactDurationMinutes(stats.SleepMinutes)},
		{Label: "Pump", Count: stats.PumpCount, Detail: fmt.Sprintf("%d ml", stats.PumpMl)},
		{Label: "Nappies", Count: stats.NappyCount},
	}
}

// markDailyReportEmailFailed records recoverable per-recipient failures so a
// later scheduler run can retry the same delivery row.
func (h *Handlers) markDailyReportEmailFailed(ctx context.Context, deliveryID uuid.UUID, cause error, attemptedAt time.Time) (dailyReportEmailOutcome, error) {
	slog.Error("daily report email delivery failed",
		"delivery_id", deliveryID,
		"error", cause,
	)
	if _, err := h.Store.MarkAIReportEmailDeliveryFailed(ctx, deliveryID, deliveryErrorMessage(cause), attemptedAt); err != nil {
		return "", fmt.Errorf("marking AI report email delivery failed: %w", err)
	}
	return dailyReportEmailOutcomeFailed, nil
}

// deliveryErrorMessage keeps provider/model errors useful but bounded for the
// delivery table.
func deliveryErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}
