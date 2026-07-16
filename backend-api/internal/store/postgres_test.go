package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// These are integration tests, not pure unit tests — the store package is a
// thin SQL wrapper, so there's no meaningful logic to test without a real
// database. They connect to the local Postgres started by
// `docker compose up postgres` (or `task up`) and skip, rather than fail,
// if it isn't reachable, so `go test ./...` still works in environments
// without Docker running.
func testStore(t *testing.T) *PostgresStore {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Same default docker-compose.yml/.env.example produce for local dev,
		// kept here (rather than only reading the env var) so `go test ./...`
		// works out of the box against `docker compose up postgres` without
		// requiring DATABASE_URL to be exported manually first.
		dbURL = "postgres://postgres:postgres@localhost:5432/yauli?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping: could not connect to postgres at %s (is `docker compose up postgres` running?): %v", dbURL, err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	return NewPostgresStore(pool)
}

// testEmail returns a unique email per call so tests can run repeatedly
// (and in parallel) without colliding on the `users.email` unique
// constraint.
func testEmail(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-%s@example.com", uuid.NewString())
}

// execCleanup runs a teardown statement and reports (rather than silently
// swallows) any error, so a failed cleanup surfaces at its source instead of
// causing a confusing failure in some later, unrelated test run.
func execCleanup(t *testing.T, s *PostgresStore, query string, args ...any) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), query, args...); err != nil {
		t.Errorf("cleanup %q: %v", query, err)
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %q: %v", name, err)
	}
	return loc
}

func TestUpsertUserByEmail_Idempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)
	t.Cleanup(func() { execCleanup(t, s, `DELETE FROM users WHERE email = $1`, email) })

	first, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if first.Email != email {
		t.Fatalf("expected email %q, got %q", email, first.Email)
	}

	second, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the same user id on a repeat upsert, got %v vs %v", second.ID, first.ID)
	}
}

func TestGetUser(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)
	t.Cleanup(func() { execCleanup(t, s, `DELETE FROM users WHERE email = $1`, email) })

	created, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.GetUser(ctx, created.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected id %v, got %v", created.ID, got.ID)
	}
	if got.Email != email {
		t.Fatalf("expected email %q, got %q", email, got.Email)
	}

	if _, err := s.GetUser(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing user, got %v", err)
	}
}

func TestUpdateUserDisplayName(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)
	t.Cleanup(func() { execCleanup(t, s, `DELETE FROM users WHERE email = $1`, email) })

	created, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	updated, err := s.UpdateUserDisplayName(ctx, created.ID, "Jenny")
	if err != nil {
		t.Fatalf("update display name: %v", err)
	}
	if updated.DisplayName != "Jenny" {
		t.Fatalf("expected display name %q, got %q", "Jenny", updated.DisplayName)
	}

	cleared, err := s.UpdateUserDisplayName(ctx, created.ID, "")
	if err != nil {
		t.Fatalf("clear display name: %v", err)
	}
	if cleared.DisplayName != "" {
		t.Fatalf("expected cleared display name, got %q", cleared.DisplayName)
	}

	if _, err := s.UpdateUserDisplayName(ctx, uuid.New(), "Nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing user, got %v", err)
	}
}

func TestAIReportCacheCreateAndGet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	familyID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	babyID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	rangeStart := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	inputHash := uuid.NewString()

	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM ai_report_cache WHERE input_hash = $1`, inputHash)
	})

	content := json.RawMessage(`{"schema_version":"ai_report_output.v1","title":"Cached report"}`)
	saved, err := s.CreateAIReportCache(ctx, AIReportCache{
		FamilyID:            familyID,
		BabyID:              babyID,
		ReportType:          "daily",
		RangeStart:          rangeStart,
		RangeEnd:            rangeEnd,
		InputHash:           inputHash,
		PromptVersion:       "ai_report_prompt.v1",
		InputSchemaVersion:  "ai_report_input.v1",
		OutputSchemaVersion: "ai_report_output.v1",
		Model:               "test-model",
		ContentJSON:         content,
	})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	if saved.ID == uuid.Nil {
		t.Fatal("saved cache ID is nil")
	}
	if saved.CreatedAt.IsZero() {
		t.Fatal("saved cache CreatedAt is zero")
	}

	got, err := s.GetAIReportCache(ctx, familyID, babyID, "daily", rangeStart, rangeEnd, inputHash)
	if err != nil {
		t.Fatalf("get cache: %v", err)
	}
	if got.ID != saved.ID {
		t.Fatalf("cache ID = %v, want %v", got.ID, saved.ID)
	}
	var gotContent map[string]any
	if err := json.Unmarshal(got.ContentJSON, &gotContent); err != nil {
		t.Fatalf("unmarshal got content: %v", err)
	}
	var wantContent map[string]any
	if err := json.Unmarshal(content, &wantContent); err != nil {
		t.Fatalf("unmarshal want content: %v", err)
	}
	if gotContent["schema_version"] != wantContent["schema_version"] || gotContent["title"] != wantContent["title"] {
		t.Fatalf("content = %#v, want %#v", gotContent, wantContent)
	}

	_, err = s.GetAIReportCache(ctx, familyID, babyID, "daily", rangeStart, rangeEnd, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing cache error = %v, want ErrNotFound", err)
	}
}

func TestAIReportEmailDeliveryLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)

	owner, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	baby, err := s.CreateBaby(ctx, familyID, "YauYau", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM ai_report_email_deliveries WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM ai_report_cache WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	rangeStart := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	scheduledFor := time.Date(2026, 7, 14, 23, 30, 0, 0, time.UTC)
	created, err := s.CreateAIReportEmailDelivery(ctx, AIReportEmailDelivery{
		FamilyID:        familyID,
		BabyID:          baby.ID,
		RecipientUserID: owner.ID,
		RecipientEmail:  owner.Email,
		ReportType:      "daily",
		RangeStart:      rangeStart,
		RangeEnd:        rangeEnd,
		ScheduledFor:    scheduledFor,
	})
	if err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("created delivery ID is nil")
	}
	if created.Status != AIReportEmailDeliveryStatusPending {
		t.Fatalf("status = %q, want %q", created.Status, AIReportEmailDeliveryStatusPending)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("expected created/updated timestamps, got created=%v updated=%v", created.CreatedAt, created.UpdatedAt)
	}

	duplicate, err := s.CreateAIReportEmailDelivery(ctx, AIReportEmailDelivery{
		FamilyID:        familyID,
		BabyID:          baby.ID,
		RecipientUserID: owner.ID,
		RecipientEmail:  owner.Email,
		ReportType:      "daily",
		RangeStart:      rangeStart,
		RangeEnd:        rangeEnd,
		ScheduledFor:    scheduledFor,
	})
	if err != nil {
		t.Fatalf("create duplicate delivery: %v", err)
	}
	if duplicate.ID != created.ID {
		t.Fatalf("duplicate delivery ID = %v, want existing ID %v", duplicate.ID, created.ID)
	}

	claimedAt := scheduledFor.Add(time.Minute)
	claimed, err := s.ClaimAIReportEmailDelivery(ctx, created.ID, claimedAt)
	if err != nil {
		t.Fatalf("claim delivery: %v", err)
	}
	if claimed.Status != AIReportEmailDeliveryStatusSending {
		t.Fatalf("claimed status = %q, want %q", claimed.Status, AIReportEmailDeliveryStatusSending)
	}
	if claimed.AttemptedAt == nil || !claimed.AttemptedAt.Equal(claimedAt) {
		t.Fatalf("claimed attempted_at = %v, want %v", claimed.AttemptedAt, claimedAt)
	}
	if _, err := s.ClaimAIReportEmailDelivery(ctx, created.ID, claimedAt.Add(time.Second)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second claim error = %v, want ErrNotFound", err)
	}
	staleClaimedAt := claimedAt.Add(aiReportEmailDeliveryClaimTimeout + time.Minute)
	staleClaimed, err := s.ClaimAIReportEmailDelivery(ctx, created.ID, staleClaimedAt)
	if err != nil {
		t.Fatalf("stale claim delivery: %v", err)
	}
	if staleClaimed.Status != AIReportEmailDeliveryStatusSending {
		t.Fatalf("stale claimed status = %q, want %q", staleClaimed.Status, AIReportEmailDeliveryStatusSending)
	}
	if staleClaimed.AttemptedAt == nil || !staleClaimed.AttemptedAt.Equal(staleClaimedAt) {
		t.Fatalf("stale claimed attempted_at = %v, want %v", staleClaimed.AttemptedAt, staleClaimedAt)
	}

	cache, err := s.CreateAIReportCache(ctx, AIReportCache{
		FamilyID:            familyID,
		BabyID:              baby.ID,
		ReportType:          "daily",
		RangeStart:          rangeStart,
		RangeEnd:            rangeEnd,
		InputHash:           uuid.NewString(),
		PromptVersion:       "ai_report_prompt.v1",
		InputSchemaVersion:  "ai_report_input.v1",
		OutputSchemaVersion: "ai_report_output.v1",
		Model:               "test-model",
		ContentJSON:         json.RawMessage(`{"schema_version":"ai_report_output.v1","title":"Cached report"}`),
	})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}

	sentAt := scheduledFor.Add(2 * time.Minute)
	sent, err := s.MarkAIReportEmailDeliverySent(ctx, created.ID, cache.ID, "provider-123", sentAt)
	if err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	if sent.Status != AIReportEmailDeliveryStatusSent {
		t.Fatalf("sent status = %q, want %q", sent.Status, AIReportEmailDeliveryStatusSent)
	}
	if sent.AIReportCacheID == nil || *sent.AIReportCacheID != cache.ID {
		t.Fatalf("sent cache ID = %v, want %v", sent.AIReportCacheID, cache.ID)
	}
	if sent.ProviderMessageID != "provider-123" {
		t.Fatalf("provider message ID = %q, want provider-123", sent.ProviderMessageID)
	}
	if sent.AttemptedAt == nil || !sent.AttemptedAt.Equal(sentAt) {
		t.Fatalf("attempted_at = %v, want %v", sent.AttemptedAt, sentAt)
	}
	if sent.SentAt == nil || !sent.SentAt.Equal(sentAt) {
		t.Fatalf("sent_at = %v, want %v", sent.SentAt, sentAt)
	}
	if sent.ErrorMessage != "" {
		t.Fatalf("sent error message = %q, want empty", sent.ErrorMessage)
	}

	failedAt := sentAt.Add(3 * time.Minute)
	failed, err := s.MarkAIReportEmailDeliveryFailed(ctx, created.ID, "mail provider unavailable", failedAt)
	if err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if failed.Status != AIReportEmailDeliveryStatusFailed {
		t.Fatalf("failed status = %q, want %q", failed.Status, AIReportEmailDeliveryStatusFailed)
	}
	if failed.ErrorMessage != "mail provider unavailable" {
		t.Fatalf("failed error message = %q, want mail provider unavailable", failed.ErrorMessage)
	}
	if failed.ProviderMessageID != "" {
		t.Fatalf("failed provider message ID = %q, want empty", failed.ProviderMessageID)
	}
	if failed.AttemptedAt == nil || !failed.AttemptedAt.Equal(failedAt) {
		t.Fatalf("failed attempted_at = %v, want %v", failed.AttemptedAt, failedAt)
	}
	if failed.SentAt != nil {
		t.Fatalf("failed sent_at = %v, want nil", failed.SentAt)
	}
	if failed.AIReportCacheID == nil || *failed.AIReportCacheID != cache.ID {
		t.Fatalf("failed cache ID = %v, want retained cache ID %v", failed.AIReportCacheID, cache.ID)
	}

	if _, err := s.MarkAIReportEmailDeliverySent(ctx, uuid.New(), cache.ID, "missing", sentAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing sent delivery error = %v, want ErrNotFound", err)
	}
	if _, err := s.MarkAIReportEmailDeliveryFailed(ctx, uuid.New(), "missing", failedAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing failed delivery error = %v, want ErrNotFound", err)
	}
}

func TestDailyReportEmailJobForUsesBabyTimezoneAndYesterdayWindow(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	familyID := uuid.New()
	babyID := uuid.New()
	recipientID := uuid.New()
	candidate := dailyReportEmailCandidate{
		FamilyID:        familyID,
		BabyID:          babyID,
		BabyName:        "YauYau",
		BabyTimezone:    "Australia/Adelaide",
		RecipientUserID: recipientID,
		RecipientEmail:  "owner@example.com",
	}

	beforeSend := time.Date(2026, 7, 16, 8, 59, 0, 0, loc)
	if _, due, err := dailyReportEmailJobFor(candidate, beforeSend); err != nil || due {
		t.Fatalf("before 9am due=%v err=%v, want due=false nil err", due, err)
	}

	afterSend := time.Date(2026, 7, 16, 9, 1, 0, 0, loc)
	job, due, err := dailyReportEmailJobFor(candidate, afterSend)
	if err != nil {
		t.Fatalf("build job: %v", err)
	}
	if !due {
		t.Fatal("expected job to be due after 9am local time")
	}
	if job.FamilyID != familyID || job.BabyID != babyID || job.RecipientUserID != recipientID {
		t.Fatalf("job ids = %+v, want candidate ids", job)
	}
	if job.ReportType != "daily" {
		t.Fatalf("ReportType = %q, want daily", job.ReportType)
	}
	if job.StartDate != "2026-07-15" || job.EndDate != "2026-07-15" {
		t.Fatalf("dates = %s to %s, want yesterday 2026-07-15", job.StartDate, job.EndDate)
	}
	wantScheduledFor := time.Date(2026, 7, 16, 9, 0, 0, 0, loc)
	if !job.ScheduledFor.Equal(wantScheduledFor) {
		t.Fatalf("ScheduledFor = %s, want %s", job.ScheduledFor, wantScheduledFor)
	}
	if !job.RangeStart.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, loc)) || !job.RangeEnd.Equal(time.Date(2026, 7, 16, 0, 0, 0, 0, loc)) {
		t.Fatalf("range = %s to %s, want full previous local day", job.RangeStart, job.RangeEnd)
	}
}

func TestListDueDailyReportEmailJobs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	firstBaby, err := s.CreateBaby(ctx, familyID, "First", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create first baby: %v", err)
	}
	if _, err := s.CreateBaby(ctx, familyID, "Second", "Australia/Adelaide"); err != nil {
		t.Fatalf("create second baby: %v", err)
	}

	beforeNine := time.Date(2026, 7, 15, 23, 0, 0, 0, time.UTC) // 08:30 on 2026-07-16 in Adelaide.
	jobs, err := s.ListDueDailyReportEmailJobs(ctx, beforeNine)
	if err != nil {
		t.Fatalf("list before 9am: %v", err)
	}
	if matchingDailyReportEmailJobs(jobs, familyID) != 0 {
		t.Fatalf("before 9am jobs = %+v, want none for test family", jobs)
	}

	afterNine := time.Date(2026, 7, 15, 23, 45, 0, 0, time.UTC) // 09:15 on 2026-07-16 in Adelaide.
	jobs, err = s.ListDueDailyReportEmailJobs(ctx, afterNine)
	if err != nil {
		t.Fatalf("list after 9am: %v", err)
	}
	testJobs := dailyReportEmailJobsForFamily(jobs, familyID)
	if len(testJobs) != 1 {
		t.Fatalf("len(testJobs) = %d, want 1: %+v", len(testJobs), jobs)
	}
	if testJobs[0].BabyID != firstBaby.ID || testJobs[0].BabyName != "First" {
		t.Fatalf("job baby = %s %q, want first-created baby %s", testJobs[0].BabyID, testJobs[0].BabyName, firstBaby.ID)
	}
	if testJobs[0].RecipientUserID != owner.ID || testJobs[0].RecipientEmail != owner.Email {
		t.Fatalf("job recipient = %s %q, want owner %s %q", testJobs[0].RecipientUserID, testJobs[0].RecipientEmail, owner.ID, owner.Email)
	}
	if testJobs[0].StartDate != "2026-07-15" || testJobs[0].EndDate != "2026-07-15" {
		t.Fatalf("job dates = %s to %s, want 2026-07-15", testJobs[0].StartDate, testJobs[0].EndDate)
	}

	if _, err := s.UpdateDailyReportEmailPreference(ctx, familyID, owner.ID, false); err != nil {
		t.Fatalf("disable daily report email: %v", err)
	}
	jobs, err = s.ListDueDailyReportEmailJobs(ctx, afterNine)
	if err != nil {
		t.Fatalf("list after opt-out: %v", err)
	}
	if matchingDailyReportEmailJobs(jobs, familyID) != 0 {
		t.Fatalf("opted-out jobs = %+v, want none for test family", jobs)
	}
}

func dailyReportEmailJobsForFamily(jobs []DailyReportEmailJob, familyID uuid.UUID) []DailyReportEmailJob {
	var matched []DailyReportEmailJob
	for _, job := range jobs {
		if job.FamilyID == familyID {
			matched = append(matched, job)
		}
	}
	return matched
}

func matchingDailyReportEmailJobs(jobs []DailyReportEmailJob, familyID uuid.UUID) int {
	return len(dailyReportEmailJobsForFamily(jobs, familyID))
}

func TestGetFamilyMembership_NotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)
	t.Cleanup(func() { execCleanup(t, s, `DELETE FROM users WHERE email = $1`, email) })

	user, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	membership, err := s.GetFamilyMembership(ctx, user.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.Found {
		t.Fatalf("expected no membership for a fresh user, got %+v", membership)
	}
}

func TestCreateFamilyWithOwner(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)

	user, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	familyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, user.ID)
	})

	membership, err := s.GetFamilyMembership(ctx, user.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if !membership.Found {
		t.Fatalf("expected a membership to exist after CreateFamilyWithOwner")
	}
	if membership.FamilyID == nil || *membership.FamilyID != familyID {
		t.Fatalf("expected family id %v, got %v", familyID, membership.FamilyID)
	}
	if membership.Role != MembershipRoleOwner {
		t.Fatalf("expected role %q, got %q", MembershipRoleOwner, membership.Role)
	}
	if membership.Status != MembershipStatusActive {
		t.Fatalf("expected status %q, got %q", MembershipStatusActive, membership.Status)
	}
	if !membership.DailyReportEmailEnabled {
		t.Fatalf("expected owner daily report email to be enabled by default")
	}
}

// TestCreateFamilyWithOwner_RejectsSecondActiveMembership guards the
// idx_family_members_one_active_per_user constraint: a user who already has
// an active membership must not be able to end up with a second one (e.g. a
// retried "create family" request), rather than silently ending up owner of
// two families with GetFamilyMembership returning an arbitrary one of them.
func TestCreateFamilyWithOwner_RejectsSecondActiveMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	user, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	firstFamilyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "first family")
	if err != nil {
		t.Fatalf("create first family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, firstFamilyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, firstFamilyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, user.ID)
	})

	if _, err := s.CreateFamilyWithOwner(ctx, user.ID, "second family"); err == nil {
		t.Fatalf("expected creating a second family for an already-active user to fail, got no error")
	}

	// The rejected second attempt must not have left anything behind: the
	// user should still resolve to exactly the first family.
	membership, err := s.GetFamilyMembership(ctx, user.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.FamilyID == nil || *membership.FamilyID != firstFamilyID {
		t.Fatalf("expected membership to still point at the first family %v, got %v", firstFamilyID, membership.FamilyID)
	}
}

func TestActivateInvitedMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}

	invitee, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert invitee: %v", err)
	}

	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{owner.ID, invitee.ID})
	})

	// Simulate an invite (PR11 will do this via a real invite endpoint):
	// a pending family_members row for a user who hasn't logged in yet.
	if _, err := s.pool.Exec(ctx, `INSERT INTO family_members (family_id, user_id, role, status) VALUES ($1, $2, $3, $4)`,
		familyID, invitee.ID, MembershipRoleMember, MembershipStatusInvited); err != nil {
		t.Fatalf("insert invited row: %v", err)
	}

	if err := s.ActivateInvitedMembership(ctx, invitee.ID, familyID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	membership, err := s.GetFamilyMembership(ctx, invitee.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.Status != MembershipStatusActive {
		t.Fatalf("expected status %q after activation, got %q", MembershipStatusActive, membership.Status)
	}
	if membership.Role != MembershipRoleMember {
		t.Fatalf("expected role %q, got %q", MembershipRoleMember, membership.Role)
	}
}

func TestActivateInvitedMembership_NotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// No invited row exists for these arbitrary, never-inserted ids.
	err := s.ActivateInvitedMembership(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateInvite_CreatesUserAndMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}

	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	invitee, err := s.UpsertUserByEmail(ctx, inviteeEmail)
	if err != nil {
		t.Fatalf("resolve invitee: %v", err)
	}

	membership, err := s.GetFamilyMembership(ctx, invitee.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if !membership.Found || membership.Status != MembershipStatusInvited {
		t.Fatalf("expected an invited membership, got %+v", membership)
	}
	if membership.FamilyID == nil || *membership.FamilyID != familyID {
		t.Fatalf("expected family id %v, got %v", familyID, membership.FamilyID)
	}
	if membership.DailyReportEmailEnabled {
		t.Fatalf("expected invited member daily report email to be disabled by default")
	}
}

func TestUpdateDailyReportEmailPreferenceOwnerOnly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	updated, err := s.UpdateDailyReportEmailPreference(ctx, familyID, owner.ID, false)
	if err != nil {
		t.Fatalf("disable owner daily report email: %v", err)
	}
	if updated.DailyReportEmailEnabled {
		t.Fatalf("expected owner daily report email to be disabled")
	}
	if err := Migrate(ctx, s.pool, "../../migrations"); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}
	membership, err := s.GetFamilyMembershipForFamily(ctx, owner.ID, familyID)
	if err != nil {
		t.Fatalf("get membership after migration rerun: %v", err)
	}
	if membership.DailyReportEmailEnabled {
		t.Fatalf("expected migration rerun to preserve owner opt-out")
	}

	updated, err = s.UpdateDailyReportEmailPreference(ctx, familyID, owner.ID, true)
	if err != nil {
		t.Fatalf("enable owner daily report email: %v", err)
	}
	if !updated.DailyReportEmailEnabled {
		t.Fatalf("expected owner daily report email to be enabled")
	}

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("create invite: %v", err)
	}
	invitee, err := s.UpsertUserByEmail(ctx, inviteeEmail)
	if err != nil {
		t.Fatalf("resolve invitee: %v", err)
	}
	if _, err := s.UpdateDailyReportEmailPreference(ctx, familyID, invitee.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected member update to return ErrNotFound, got %v", err)
	}
}

// TestCreateInvite_IdempotentOnRepeat guards the ON CONFLICT DO NOTHING added
// to CreateInvite: a retried or double-sent invite for the same
// (family_id, email) pair must succeed as a no-op rather than fail on
// family_members' primary key.
func TestCreateInvite_IdempotentOnRepeat(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}

	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("first invite: %v", err)
	}
	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("expected a repeat invite for the same email/family to be a no-op, got: %v", err)
	}
}

// TestGetFamilyMembership_PrefersActiveOverInvited guards the ORDER BY added
// to GetFamilyMembership: a user who is active in one family and separately
// invited to another must resolve to the active membership, not whichever
// row Postgres happens to return first.
func TestGetFamilyMembership_PrefersActiveOverInvited(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	user, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	activeFamilyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "active family")
	if err != nil {
		t.Fatalf("create active family: %v", err)
	}

	otherOwner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert other owner: %v", err)
	}
	invitedFamilyID, err := s.CreateFamilyWithOwner(ctx, otherOwner.ID, "invited family")
	if err != nil {
		t.Fatalf("create invited family: %v", err)
	}

	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM families WHERE id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{user.ID, otherOwner.ID})
	})

	// user is already active in activeFamilyID; inviting them into a second
	// family gives them two family_members rows.
	if err := s.CreateInvite(ctx, invitedFamilyID, user.Email); err != nil {
		t.Fatalf("invite into second family: %v", err)
	}

	membership, err := s.GetFamilyMembership(ctx, user.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.Status != MembershipStatusActive {
		t.Fatalf("expected the active membership to be preferred, got status %q", membership.Status)
	}
	if membership.FamilyID == nil || *membership.FamilyID != activeFamilyID {
		t.Fatalf("expected family id %v (the active one), got %v", activeFamilyID, membership.FamilyID)
	}
}

func TestGetFamilyMembershipForFamily_ReturnsSpecificMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	user, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	activeFamilyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "active family")
	if err != nil {
		t.Fatalf("create active family: %v", err)
	}

	otherOwner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert other owner: %v", err)
	}
	invitedFamilyID, err := s.CreateFamilyWithOwner(ctx, otherOwner.ID, "invited family")
	if err != nil {
		t.Fatalf("create invited family: %v", err)
	}

	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM families WHERE id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{user.ID, otherOwner.ID})
	})

	if err := s.CreateInvite(ctx, invitedFamilyID, user.Email); err != nil {
		t.Fatalf("invite into second family: %v", err)
	}

	membership, err := s.GetFamilyMembershipForFamily(ctx, user.ID, invitedFamilyID)
	if err != nil {
		t.Fatalf("get membership for invited family: %v", err)
	}
	if !membership.Found {
		t.Fatal("expected invited membership to be found")
	}
	if membership.FamilyID == nil || *membership.FamilyID != invitedFamilyID {
		t.Fatalf("expected family id %v, got %v", invitedFamilyID, membership.FamilyID)
	}
	if membership.Status != MembershipStatusInvited {
		t.Fatalf("expected status %q, got %q", MembershipStatusInvited, membership.Status)
	}
}

func TestHasPendingInviteOutsideFamily(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	user, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	activeFamilyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "active family")
	if err != nil {
		t.Fatalf("create active family: %v", err)
	}

	otherOwner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert other owner: %v", err)
	}
	invitedFamilyID, err := s.CreateFamilyWithOwner(ctx, otherOwner.ID, "invited family")
	if err != nil {
		t.Fatalf("create invited family: %v", err)
	}

	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM families WHERE id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{user.ID, otherOwner.ID})
	})

	found, err := s.HasPendingInviteOutsideFamily(ctx, user.ID, activeFamilyID)
	if err != nil {
		t.Fatalf("check pending invite before invite: %v", err)
	}
	if found {
		t.Fatal("expected no pending invite before invite")
	}

	if err := s.CreateInvite(ctx, invitedFamilyID, user.Email); err != nil {
		t.Fatalf("invite into second family: %v", err)
	}

	found, err = s.HasPendingInviteOutsideFamily(ctx, user.ID, activeFamilyID)
	if err != nil {
		t.Fatalf("check pending invite after invite: %v", err)
	}
	if !found {
		t.Fatal("expected pending invite outside active family")
	}
}

func TestListTimelineMembers(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}

	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	members, err := s.ListTimelineMembers(ctx, familyID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d: %+v", len(members), members)
	}
	if members[0].UserID != owner.ID || members[0].Email != owner.Email || members[0].Role != MembershipRoleOwner || members[0].Status != MembershipStatusActive {
		t.Fatalf("expected owner first, got %+v", members[0])
	}
	if members[1].Email != inviteeEmail || members[1].Role != MembershipRoleMember || members[1].Status != MembershipStatusInvited {
		t.Fatalf("expected invited member second, got %+v", members[1])
	}
}

func TestUpdateTimelineMemberRelationship(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	if err := s.UpdateTimelineMemberRelationship(ctx, familyID, owner.ID, "  Mum  "); err != nil {
		t.Fatalf("update relationship: %v", err)
	}

	members, err := s.ListTimelineMembers(ctx, familyID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || members[0].Relationship != "Mum" {
		t.Fatalf("expected relationship to be trimmed and stored, got %+v", members)
	}

	if err := s.UpdateTimelineMemberRelationship(ctx, familyID, owner.ID, " "); err != nil {
		t.Fatalf("clear relationship: %v", err)
	}
	members, err = s.ListTimelineMembers(ctx, familyID)
	if err != nil {
		t.Fatalf("list members after clear: %v", err)
	}
	if len(members) != 1 || members[0].Relationship != "" {
		t.Fatalf("expected relationship to be cleared, got %+v", members)
	}
}

func TestRemoveTimelineMember(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}

	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("create invite: %v", err)
	}
	invitee, err := s.UpsertUserByEmail(ctx, inviteeEmail)
	if err != nil {
		t.Fatalf("resolve invitee: %v", err)
	}

	if err := s.RemoveTimelineMember(ctx, familyID, invitee.ID); err != nil {
		t.Fatalf("remove member: %v", err)
	}

	members, err := s.ListTimelineMembers(ctx, familyID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || members[0].UserID != owner.ID {
		t.Fatalf("expected only owner to remain, got %+v", members)
	}
	if err := s.RemoveTimelineMember(ctx, familyID, invitee.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on repeat remove, got %v", err)
	}
}

func TestRemoveTimelineMemberDeletesActiveMember(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	if err := s.RemoveTimelineMember(ctx, familyID, owner.ID); err != nil {
		t.Fatalf("remove active member: %v", err)
	}

	membership, err := s.GetFamilyMembershipForFamily(ctx, owner.ID, familyID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.Found {
		t.Fatalf("expected active membership to be removed, got %+v", membership)
	}
}

func TestCreateBaby(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM events WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	baby, err := s.CreateBaby(ctx, familyID, "YauYau", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}
	if baby.FamilyID != familyID {
		t.Fatalf("expected family id %v, got %v", familyID, baby.FamilyID)
	}
	if baby.Name != "YauYau" {
		t.Fatalf("expected name %q, got %q", "YauYau", baby.Name)
	}
	if baby.Timezone != "Australia/Adelaide" {
		t.Fatalf("expected timezone %q, got %q", "Australia/Adelaide", baby.Timezone)
	}
}

func TestGetBaby(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM events WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	created, err := s.CreateBaby(ctx, familyID, "YauYau", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}

	got, err := s.GetBaby(ctx, created.ID)
	if err != nil {
		t.Fatalf("get baby: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected baby id %v, got %v", created.ID, got.ID)
	}
	if got.FamilyID != familyID {
		t.Fatalf("expected family id %v, got %v", familyID, got.FamilyID)
	}
}

// TestGetCurrentBaby_ReturnsFirstCreated guards the "current baby" ordering
// for a family with more than one baby (e.g. twins added one after another) —
// it must consistently return the first one created, not an arbitrary row.
func TestGetCurrentBaby_ReturnsFirstCreated(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	first, err := s.CreateBaby(ctx, familyID, "First", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create first baby: %v", err)
	}
	if _, err := s.CreateBaby(ctx, familyID, "Second", "Australia/Adelaide"); err != nil {
		t.Fatalf("create second baby: %v", err)
	}

	current, err := s.GetCurrentBaby(ctx, familyID)
	if err != nil {
		t.Fatalf("get current baby: %v", err)
	}
	if current.ID != first.ID {
		t.Fatalf("expected the first-created baby %v, got %v", first.ID, current.ID)
	}
}

func TestGetCurrentBaby_NotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	if _, err := s.GetCurrentBaby(ctx, familyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a family with no babies, got %v", err)
	}
}

func TestUpdateBaby(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	baby, err := s.CreateBaby(ctx, familyID, "Old", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}

	updated, err := s.UpdateBaby(ctx, familyID, baby.ID, Baby{
		Name:          "New",
		Timezone:      "UTC",
		BirthDate:     "2026-07-10",
		BirthWeightKg: "3.45",
		BirthLengthCm: "51.2",
		Sex:           "female",
	})
	if err != nil {
		t.Fatalf("update baby: %v", err)
	}
	if updated.Name != "New" || updated.Timezone != "UTC" {
		t.Fatalf("expected updated baby, got %+v", updated)
	}
	if updated.BirthDate != "2026-07-10" {
		t.Fatalf("expected birth date %q, got %q", "2026-07-10", updated.BirthDate)
	}
	if updated.BirthWeightKg != "3.45" {
		t.Fatalf("expected birth weight %q, got %q", "3.45", updated.BirthWeightKg)
	}
	if updated.BirthLengthCm != "51.2" {
		t.Fatalf("expected birth length %q, got %q", "51.2", updated.BirthLengthCm)
	}
	if updated.Sex != "female" {
		t.Fatalf("expected sex %q, got %q", "female", updated.Sex)
	}

	got, err := s.GetBaby(ctx, baby.ID)
	if err != nil {
		t.Fatalf("get baby: %v", err)
	}
	if got.BirthDate != updated.BirthDate || got.BirthWeightKg != updated.BirthWeightKg || got.BirthLengthCm != updated.BirthLengthCm || got.Sex != updated.Sex {
		t.Fatalf("expected profile details to persist, got %+v", got)
	}
}

func TestArchiveBabyHidesCurrentBaby(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	baby, err := s.CreateBaby(ctx, familyID, "Archived", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}
	if err := s.ArchiveBaby(ctx, familyID, baby.ID); err != nil {
		t.Fatalf("archive baby: %v", err)
	}
	if _, err := s.GetCurrentBaby(ctx, familyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected archived baby to be hidden from current baby lookup, got %v", err)
	}
	if _, err := s.GetBaby(ctx, baby.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected archived baby to be hidden from direct lookup, got %v", err)
	}
}

func TestUpdateEvent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM events WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	baby, err := s.CreateBaby(ctx, familyID, "YauYau", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}
	createdAt := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	ev, err := s.CreateEvent(ctx, familyID, baby.ID, "nappy", map[string]any{"kind": "wet"}, createdAt)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	updatedAt := createdAt.Add(30 * time.Minute)
	updated, err := s.UpdateEvent(ctx, familyID, baby.ID, ev.ID, "nappy", map[string]any{"kind": "poo", "colour": "mustard"}, updatedAt)
	if err != nil {
		t.Fatalf("update event: %v", err)
	}
	if updated.EventType != "nappy" || updated.OccurredAt != updatedAt {
		t.Fatalf("unexpected updated event: %+v", updated)
	}
	if updated.Attributes["kind"] != "poo" || updated.Attributes["colour"] != "mustard" {
		t.Fatalf("expected updated attributes, got %+v", updated.Attributes)
	}

	if _, err := s.UpdateEvent(ctx, familyID, baby.ID, ev.ID, "feed", map[string]any{"type": "breast"}, updatedAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected event_type mismatch to return ErrNotFound, got %v", err)
	}
}

func TestBabyLatestGrowthProjectionStaysConsistentWithGrowthEvents(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM baby_latest_growth WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM events WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	baby, err := s.CreateBaby(ctx, familyID, "YauYau", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}
	firstMeasuredAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	first, err := s.CreateEvent(ctx, familyID, baby.ID, "growth_measurement", map[string]any{
		"weight_grams":          7200,
		"length_cm":             66.5,
		"head_circumference_cm": 42.1,
	}, firstMeasuredAt)
	if err != nil {
		t.Fatalf("create first growth event: %v", err)
	}
	secondMeasuredAt := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	second, err := s.CreateEvent(ctx, familyID, baby.ID, "growth_measurement", map[string]any{
		"weight_grams": 7400,
	}, secondMeasuredAt)
	if err != nil {
		t.Fatalf("create second growth event: %v", err)
	}
	if _, err := s.CreateEvent(ctx, familyID, baby.ID, "nappy", map[string]any{"kind": "wet"}, secondMeasuredAt.Add(time.Hour)); err != nil {
		t.Fatalf("create unrelated event: %v", err)
	}

	gotEvent, err := s.GetEvent(ctx, familyID, baby.ID, first.ID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if gotEvent.EventType != "growth_measurement" {
		t.Fatalf("GetEvent EventType = %q, want growth_measurement", gotEvent.EventType)
	}

	growth, err := s.GetBabyLatestGrowth(ctx, familyID, baby.ID)
	if err != nil {
		t.Fatalf("get latest growth after create: %v", err)
	}
	if growth.WeightGrams == nil || *growth.WeightGrams != 7400 || growth.WeightMeasuredAt == nil || !growth.WeightMeasuredAt.Equal(secondMeasuredAt) {
		t.Fatalf("weight projection = %#v at %v, want 7400 at %v", growth.WeightGrams, growth.WeightMeasuredAt, secondMeasuredAt)
	}
	if growth.LengthCM == nil || *growth.LengthCM != 66.5 || growth.LengthMeasuredAt == nil || !growth.LengthMeasuredAt.Equal(firstMeasuredAt) {
		t.Fatalf("length projection = %#v at %v, want 66.5 at %v", growth.LengthCM, growth.LengthMeasuredAt, firstMeasuredAt)
	}
	if growth.HeadCircumferenceCM == nil || *growth.HeadCircumferenceCM != 42.1 || growth.HeadCircumferenceMeasuredAt == nil || !growth.HeadCircumferenceMeasuredAt.Equal(firstMeasuredAt) {
		t.Fatalf("head projection = %#v at %v, want 42.1 at %v", growth.HeadCircumferenceCM, growth.HeadCircumferenceMeasuredAt, firstMeasuredAt)
	}

	updatedMeasuredAt := secondMeasuredAt.Add(24 * time.Hour)
	if _, err := s.UpdateEvent(ctx, familyID, baby.ID, second.ID, "growth_measurement", map[string]any{
		"weight_grams": 7500,
	}, updatedMeasuredAt); err != nil {
		t.Fatalf("update second growth event: %v", err)
	}
	updated, err := s.GetBabyLatestGrowth(ctx, familyID, baby.ID)
	if err != nil {
		t.Fatalf("get latest growth after update: %v", err)
	}
	if updated.WeightGrams == nil || *updated.WeightGrams != 7500 || updated.WeightMeasuredAt == nil || !updated.WeightMeasuredAt.Equal(updatedMeasuredAt) {
		t.Fatalf("updated projection = %#v, want weight 7500 at %v", updated, updatedMeasuredAt)
	}

	if err := s.DeleteEvent(ctx, familyID, baby.ID, first.ID); err != nil {
		t.Fatalf("delete first growth event: %v", err)
	}
	growth, err = s.GetBabyLatestGrowth(ctx, familyID, baby.ID)
	if err != nil {
		t.Fatalf("get latest growth after delete: %v", err)
	}
	if growth.LengthCM != nil || growth.HeadCircumferenceCM != nil {
		t.Fatalf("length/head should clear after source delete, got %#v", growth)
	}

	if err := s.DeleteEvent(ctx, familyID, baby.ID, gotEvent.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected second delete of same event to return ErrNotFound, got %v", err)
	}
	if err := s.DeleteEvent(ctx, familyID, baby.ID, second.ID); err != nil {
		t.Fatalf("delete remaining growth event: %v", err)
	}
	if _, err := s.GetBabyLatestGrowth(ctx, familyID, baby.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected projection to be removed with final growth event, got %v", err)
	}

	invalidMeasuredAt := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	if _, err := s.CreateEvent(ctx, familyID, baby.ID, "growth_measurement", map[string]any{
		"weight_grams": "not-a-number",
	}, invalidMeasuredAt); err == nil {
		t.Fatal("expected invalid projection value to fail")
	}
	invalidEvents, err := s.ListAllEvents(ctx, familyID, baby.ID, invalidMeasuredAt.Add(-time.Minute), invalidMeasuredAt.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("list invalid growth event window: %v", err)
	}
	if len(invalidEvents) != 0 {
		t.Fatalf("invalid growth event was committed despite projection failure: %#v", invalidEvents)
	}
}
