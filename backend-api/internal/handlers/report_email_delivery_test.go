package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aireport"
	"github.com/andreistefanciprian/yauli/backend-api/internal/reportemail"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestSendDueDailyReportEmailsSendsAndMarksDeliverySent(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 5, 0, 0, time.UTC)
	job := testDailyReportEmailJob(now)
	fakeStore := &aiReportFakeStore{
		dailyReportEmailJobs: []store.DailyReportEmailJob{job},
		cacheErr:             nil,
		cachedContent:        testAIReportOutputJSON(t, "Daily report"),
	}
	sender := &fakeReportEmailSender{messageID: "mailgun-message-id"}
	h := &Handlers{Store: fakeStore, ReportEmailSender: sender}

	result, err := h.SendDueDailyReportEmails(context.Background(), now)
	if err != nil {
		t.Fatalf("send due daily report emails: %v", err)
	}

	if result.DueJobs != 1 || result.Sent != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("result = %+v, want one sent", result)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent email count = %d, want 1", len(sender.sent))
	}
	if sender.sent[0].RecipientEmail != job.RecipientEmail || sender.sent[0].BabyName != job.BabyName {
		t.Fatalf("sent email = %+v, want job recipient/baby", sender.sent[0])
	}
	if len(fakeStore.sentDeliveries) != 1 {
		t.Fatalf("sent deliveries = %d, want 1", len(fakeStore.sentDeliveries))
	}
	sent := fakeStore.sentDeliveries[0]
	if sent.ProviderMessageID != "mailgun-message-id" {
		t.Fatalf("provider message ID = %q", sent.ProviderMessageID)
	}
	if sent.AIReportCacheID == nil {
		t.Fatal("sent delivery cache ID is nil")
	}
}

func TestSendDueDailyReportEmailsSkipsAlreadySentDelivery(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 5, 0, 0, time.UTC)
	job := testDailyReportEmailJob(now)
	existing := store.AIReportEmailDelivery{
		ID:              uuid.New(),
		FamilyID:        job.FamilyID,
		BabyID:          job.BabyID,
		RecipientUserID: job.RecipientUserID,
		RecipientEmail:  job.RecipientEmail,
		ReportType:      job.ReportType,
		RangeStart:      job.RangeStart,
		RangeEnd:        job.RangeEnd,
		ScheduledFor:    job.ScheduledFor,
		Status:          store.AIReportEmailDeliveryStatusSent,
	}
	fakeStore := &aiReportFakeStore{
		dailyReportEmailJobs: []store.DailyReportEmailJob{job},
		deliveries: map[string]store.AIReportEmailDelivery{
			fakeDeliveryKey(job.FamilyID, job.BabyID, job.RecipientUserID, job.ReportType, job.RangeStart, job.RangeEnd, job.ScheduledFor): existing,
		},
	}
	sender := &fakeReportEmailSender{}
	h := &Handlers{Store: fakeStore, ReportEmailSender: sender}

	result, err := h.SendDueDailyReportEmails(context.Background(), now)
	if err != nil {
		t.Fatalf("send due daily report emails: %v", err)
	}

	if result.DueJobs != 1 || result.Skipped != 1 || result.Sent != 0 || result.Failed != 0 {
		t.Fatalf("result = %+v, want one skipped", result)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sent email count = %d, want 0", len(sender.sent))
	}
}

func TestSendDueDailyReportEmailsSkipsAlreadyClaimedDelivery(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 5, 0, 0, time.UTC)
	job := testDailyReportEmailJob(now)
	existing := store.AIReportEmailDelivery{
		ID:              uuid.New(),
		FamilyID:        job.FamilyID,
		BabyID:          job.BabyID,
		RecipientUserID: job.RecipientUserID,
		RecipientEmail:  job.RecipientEmail,
		ReportType:      job.ReportType,
		RangeStart:      job.RangeStart,
		RangeEnd:        job.RangeEnd,
		ScheduledFor:    job.ScheduledFor,
		Status:          store.AIReportEmailDeliveryStatusSending,
	}
	fakeStore := &aiReportFakeStore{
		dailyReportEmailJobs: []store.DailyReportEmailJob{job},
		deliveries: map[string]store.AIReportEmailDelivery{
			fakeDeliveryKey(job.FamilyID, job.BabyID, job.RecipientUserID, job.ReportType, job.RangeStart, job.RangeEnd, job.ScheduledFor): existing,
		},
	}
	sender := &fakeReportEmailSender{}
	h := &Handlers{Store: fakeStore, ReportEmailSender: sender}

	result, err := h.SendDueDailyReportEmails(context.Background(), now)
	if err != nil {
		t.Fatalf("send due daily report emails: %v", err)
	}

	if result.DueJobs != 1 || result.Skipped != 1 || result.Sent != 0 || result.Failed != 0 {
		t.Fatalf("result = %+v, want one skipped", result)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sent email count = %d, want 0", len(sender.sent))
	}
}

func TestSendDueDailyReportEmailsMarksFailedWhenAIIsUnconfigured(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 5, 0, 0, time.UTC)
	job := testDailyReportEmailJob(now)
	fakeStore := &aiReportFakeStore{
		dailyReportEmailJobs: []store.DailyReportEmailJob{job},
		cacheErr:             store.ErrNotFound,
	}
	h := &Handlers{Store: fakeStore, ReportEmailSender: &fakeReportEmailSender{}}

	result, err := h.SendDueDailyReportEmails(context.Background(), now)
	if err != nil {
		t.Fatalf("send due daily report emails: %v", err)
	}

	if result.DueJobs != 1 || result.Failed != 1 || result.Sent != 0 || result.Skipped != 0 {
		t.Fatalf("result = %+v, want one failed", result)
	}
	if len(fakeStore.failedDeliveries) != 1 {
		t.Fatalf("failed deliveries = %d, want 1", len(fakeStore.failedDeliveries))
	}
	if !strings.Contains(fakeStore.failedDeliveries[0].ErrorMessage, "not configured") {
		t.Fatalf("failure message = %q, want not configured", fakeStore.failedDeliveries[0].ErrorMessage)
	}
}

func TestSendDueDailyReportEmailsMarksFailedWhenSenderFails(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 5, 0, 0, time.UTC)
	job := testDailyReportEmailJob(now)
	fakeStore := &aiReportFakeStore{
		dailyReportEmailJobs: []store.DailyReportEmailJob{job},
		cachedContent:        testAIReportOutputJSON(t, "Daily report"),
	}
	h := &Handlers{
		Store:             fakeStore,
		ReportEmailSender: &fakeReportEmailSender{err: errors.New("mailgun unavailable")},
	}

	result, err := h.SendDueDailyReportEmails(context.Background(), now)
	if err != nil {
		t.Fatalf("send due daily report emails: %v", err)
	}

	if result.DueJobs != 1 || result.Failed != 1 || result.Sent != 0 || result.Skipped != 0 {
		t.Fatalf("result = %+v, want one failed", result)
	}
	if len(fakeStore.failedDeliveries) != 1 {
		t.Fatalf("failed deliveries = %d, want 1", len(fakeStore.failedDeliveries))
	}
	if !strings.Contains(fakeStore.failedDeliveries[0].ErrorMessage, "mailgun unavailable") {
		t.Fatalf("failure message = %q, want mailgun unavailable", fakeStore.failedDeliveries[0].ErrorMessage)
	}
}

type fakeReportEmailSender struct {
	messageID string
	err       error
	sent      []reportemail.Report
}

func (s *fakeReportEmailSender) SendReportEmail(_ context.Context, report reportemail.Report) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.sent = append(s.sent, report)
	return s.messageID, nil
}

func testDailyReportEmailJob(now time.Time) store.DailyReportEmailJob {
	familyID := uuid.New()
	babyID := uuid.New()
	return store.DailyReportEmailJob{
		FamilyID:        familyID,
		BabyID:          babyID,
		BabyName:        "YauYau",
		BabyTimezone:    "Australia/Adelaide",
		RecipientUserID: uuid.New(),
		RecipientEmail:  "parent@example.com",
		ReportType:      aiReportTypeDaily,
		StartDate:       "2026-07-15",
		EndDate:         "2026-07-15",
		RangeStart:      now.AddDate(0, 0, -1),
		RangeEnd:        now,
		ScheduledFor:    now.Add(-5 * time.Minute),
	}
}

func testAIReportOutputJSON(t *testing.T, title string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(aireport.Output{
		SchemaVersion:      aireport.OutputSchemaVersion,
		Title:              title,
		Summary:            "A calm day.",
		Highlights:         []string{},
		Patterns:           []string{},
		Comparison:         []string{},
		Caveats:            []string{},
		QuestionsForParent: []string{},
		DailyCard: aireport.DailyCard{
			Intro:         "Here's how YauYau's day took shape.",
			Observation:   "The day is captured here.",
			Encouragement: "You've got this.",
		},
	})
	if err != nil {
		t.Fatalf("marshal AI report output: %v", err)
	}
	return raw
}
