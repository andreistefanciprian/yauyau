package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/dailycard"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestLoadOrCreateDailyCardPassesFullCurrentDayJSON(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	now := time.Date(2026, 7, 17, 17, 0, 0, 0, loc)
	baby := store.Baby{
		ID:       uuid.New(),
		FamilyID: uuid.New(),
		Name:     "YauYau",
		Timezone: "Australia/Adelaide",
	}
	fakeStore := &aiReportFakeStore{baby: baby, cacheErr: store.ErrNotFound}
	generator := &fakeDailyCardGenerator{
		model: "test-model",
		output: json.RawMessage(`{
			"schema_version":"daily_card_output.v2",
			"title":"YauYau's day so far",
			"body":"Only a few updates have been recorded so far.",
			"closing":"You've got this, Dad."
		}`),
	}
	h := &Handlers{Store: fakeStore, DailyCardAI: generator}

	result, err := h.loadOrCreateDailyCard(t.Context(), baby, "Dad", now)
	if err != nil {
		t.Fatalf("loadOrCreateDailyCard returned error: %v", err)
	}
	if result.Cache.ReportType != dailyCardCacheReportType {
		t.Fatalf("cache report type = %q, want %q", result.Cache.ReportType, dailyCardCacheReportType)
	}
	if fakeStore.created.InputSchemaVersion != dailycard.InputSchemaVersion || fakeStore.created.OutputSchemaVersion != dailycard.OutputSchemaVersion {
		t.Fatalf("cache schema versions = %q/%q", fakeStore.created.InputSchemaVersion, fakeStore.created.OutputSchemaVersion)
	}
	if fakeStore.created.PromptVersion != dailycard.PromptVersion {
		t.Fatalf("cache prompt version = %q, want %q", fakeStore.created.PromptVersion, dailycard.PromptVersion)
	}

	var input dailyCardInput
	if err := json.Unmarshal(generator.input, &input); err != nil {
		t.Fatalf("decode generator input: %v", err)
	}
	if input.SchemaVersion != dailycard.InputSchemaVersion || input.OutputSchemaVersion != dailycard.OutputSchemaVersion {
		t.Fatalf("input schema versions = %q/%q", input.SchemaVersion, input.OutputSchemaVersion)
	}
	if input.Viewer.Relationship != "Dad" {
		t.Fatalf("viewer relationship = %q, want Dad", input.Viewer.Relationship)
	}
	if !input.ReportData.Range.GeneratedAt.Equal(now) || !input.ReportData.Range.RangeEnd.Equal(now) {
		t.Fatalf("current timestamps = generated %s, range end %s, want %s", input.ReportData.Range.GeneratedAt, input.ReportData.Range.RangeEnd, now)
	}
	if len(input.ReportData.Days) != 1 || !input.ReportData.Days[0].RangeEnd.Equal(now) {
		t.Fatalf("current day = %#v, want one day ending at now", input.ReportData.Days)
	}
	if input.ReportData.Baseline.Range.DaysIncluded != 7 {
		t.Fatalf("baseline days = %d, want 7", input.ReportData.Baseline.Range.DaysIncluded)
	}
}

func TestCreateAIDailyCardUsesViewerRelationship(t *testing.T) {
	baby := store.Baby{
		ID:       uuid.New(),
		FamilyID: uuid.New(),
		Name:     "YauYau",
		Timezone: "Australia/Adelaide",
	}
	fakeStore := &aiReportFakeStore{baby: baby, cacheErr: store.ErrNotFound}
	familyStore := &dailyCardFamilyStore{membership: store.FamilyMembership{
		Found:        true,
		FamilyID:     &baby.FamilyID,
		Status:       store.MembershipStatusActive,
		Relationship: "Father",
	}}
	generator := &fakeDailyCardGenerator{
		model: "test-model",
		output: json.RawMessage(`{
			"schema_version":"daily_card_output.v2",
			"title":"YauYau's day so far",
			"body":"Only a few updates have been recorded so far.",
			"closing":"You've got this, Dad."
		}`),
	}
	h := &Handlers{Store: fakeStore, FamilyStore: familyStore, DailyCardAI: generator}

	rec := httptest.NewRecorder()
	req := authenticatedAIReportRequest(t, baby.FamilyID, `{}`)
	req.URL.Path = "/api/v1/babies/current/reports/daily-card/ai"
	h.CreateAIDailyCard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"schema_version":"daily_card_output.v2"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	var input dailyCardInput
	if err := json.Unmarshal(generator.input, &input); err != nil {
		t.Fatalf("decode generator input: %v", err)
	}
	if input.Viewer.Relationship != "Dad" {
		t.Fatalf("viewer relationship = %q, want Dad", input.Viewer.Relationship)
	}
}

func TestParentFacingRelationship(t *testing.T) {
	tests := map[string]string{
		"Father":      "Dad",
		"mother":      "Mum",
		"Grandmother": "Grandma",
		"grandfather": "Grandpa",
		"Auntie":      "Auntie",
		"":            "",
	}
	for input, want := range tests {
		if got := parentFacingRelationship(input); got != want {
			t.Errorf("parentFacingRelationship(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDailyCardCacheFreshness(t *testing.T) {
	now := time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)
	if !dailyCardCacheFresh(store.AIReportCache{CreatedAt: now.Add(-time.Hour)}, now) {
		t.Fatal("one hour old card should be fresh")
	}
	if dailyCardCacheFresh(store.AIReportCache{CreatedAt: now.Add(-3 * time.Hour)}, now) {
		t.Fatal("three hour old card should be stale")
	}
}

func TestDailyCardInputHashIgnoresMovingClockButIncludesEventTime(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	firstNow := time.Date(2026, 7, 17, 9, 0, 0, 0, loc)
	window, err := reportDataWindowFor("", "", loc, firstNow)
	if err != nil {
		t.Fatalf("reportDataWindowFor returned error: %v", err)
	}
	reportData := reportDataResponse{
		Range: reportRangeResponse{IsPartial: true, RangeEnd: firstNow, GeneratedAt: firstNow},
		Days: []reportDayResponse{{
			IsPartial: true,
			RangeEnd:  firstNow,
			Report:    dailyReportResponse{RangeEnd: firstNow, GeneratedAt: firstNow},
			Events: []reportEventResponse{{
				ID:         uuid.New(),
				Type:       eventTypeFeed,
				OccurredAt: firstNow.Add(-time.Hour),
			}},
		}},
	}

	firstCanonical, err := canonicalAIReportHashData(reportData, window)
	if err != nil {
		t.Fatalf("canonicalize first data: %v", err)
	}
	firstHash, err := dailyCardInputHash("Dad", firstCanonical)
	if err != nil {
		t.Fatalf("hash first data: %v", err)
	}

	secondNow := firstNow.Add(90 * time.Minute)
	reportData.Range.RangeEnd = secondNow
	reportData.Range.GeneratedAt = secondNow
	reportData.Days[0].RangeEnd = secondNow
	reportData.Days[0].Report.RangeEnd = secondNow
	reportData.Days[0].Report.GeneratedAt = secondNow
	secondCanonical, err := canonicalAIReportHashData(reportData, window)
	if err != nil {
		t.Fatalf("canonicalize second data: %v", err)
	}
	secondHash, err := dailyCardInputHash("Dad", secondCanonical)
	if err != nil {
		t.Fatalf("hash second data: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("moving current time changed semantic hash: %s != %s", firstHash, secondHash)
	}

	reportData.Days[0].Events[0].OccurredAt = reportData.Days[0].Events[0].OccurredAt.Add(15 * time.Minute)
	changedCanonical, err := canonicalAIReportHashData(reportData, window)
	if err != nil {
		t.Fatalf("canonicalize changed data: %v", err)
	}
	changedHash, err := dailyCardInputHash("Dad", changedCanonical)
	if err != nil {
		t.Fatalf("hash changed data: %v", err)
	}
	if changedHash == secondHash {
		t.Fatal("event timestamp should change semantic hash")
	}
}

type fakeDailyCardGenerator struct {
	input  json.RawMessage
	output json.RawMessage
	model  string
	err    error
}

func (g *fakeDailyCardGenerator) GenerateDailyCard(_ context.Context, input json.RawMessage) (dailycard.GenerationResult, error) {
	g.input = append(json.RawMessage(nil), input...)
	if g.err != nil {
		return dailycard.GenerationResult{}, g.err
	}
	return dailycard.GenerationResult{Model: g.model, ContentJSON: g.output}, nil
}

type dailyCardFamilyStore struct {
	FamilyStore
	membership store.FamilyMembership
}

func (s *dailyCardFamilyStore) GetFamilyMembershipForFamily(context.Context, uuid.UUID, uuid.UUID) (store.FamilyMembership, error) {
	return s.membership, nil
}
