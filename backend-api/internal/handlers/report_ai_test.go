package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aireport"
	"github.com/andreistefanciprian/yauli/backend-api/internal/authctx"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestAIReportInputHashIgnoresGeneratedAt(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	generatedAt := time.Date(2026, 7, 13, 9, 30, 0, 0, loc)
	reportData := reportDataResponse{
		Baby: reportBabyResponse{
			ID:       uuid.New(),
			Name:     "YauYau",
			Timezone: "Australia/Adelaide",
		},
		Range: reportRangeResponse{
			StartDate:    "2026-07-13",
			EndDate:      "2026-07-13",
			DaysIncluded: 1,
			RangeStart:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
			RangeEnd:     time.Date(2026, 7, 14, 0, 0, 0, 0, loc),
			GeneratedAt:  generatedAt,
		},
		Baseline: reportBaselineResponse{
			Range: reportRangeResponse{
				StartDate:    "2026-07-06",
				EndDate:      "2026-07-12",
				DaysIncluded: 7,
				RangeStart:   time.Date(2026, 7, 6, 0, 0, 0, 0, loc),
				RangeEnd:     time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
				GeneratedAt:  generatedAt,
			},
		},
		Days: []reportDayResponse{{
			LocalDate:  "2026-07-13",
			RangeStart: time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
			RangeEnd:   time.Date(2026, 7, 14, 0, 0, 0, 0, loc),
			Report: dailyReportResponse{
				Title:       "Monday summary",
				GeneratedAt: generatedAt,
				RangeStart:  time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
				RangeEnd:    time.Date(2026, 7, 14, 0, 0, 0, 0, loc),
			},
		}},
	}

	canonical, err := canonicalAIReportData(reportData)
	if err != nil {
		t.Fatalf("canonicalAIReportData returned error: %v", err)
	}
	firstHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, canonical)
	if err != nil {
		t.Fatalf("aiReportInputHash returned error: %v", err)
	}

	reportData.Range.GeneratedAt = generatedAt.Add(time.Hour)
	reportData.Baseline.Range.GeneratedAt = generatedAt.Add(2 * time.Hour)
	reportData.Days[0].Report.GeneratedAt = generatedAt.Add(3 * time.Hour)

	canonical, err = canonicalAIReportData(reportData)
	if err != nil {
		t.Fatalf("canonicalAIReportData returned error: %v", err)
	}
	secondHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, canonical)
	if err != nil {
		t.Fatalf("aiReportInputHash returned error: %v", err)
	}

	if secondHash != firstHash {
		t.Fatalf("hash changed after generated_at changes: %s vs %s", secondHash, firstHash)
	}
}

func TestAIReportInputHashIncludesSemanticInputs(t *testing.T) {
	reportData, err := canonicalAIReportData(reportDataResponse{})
	if err != nil {
		t.Fatalf("canonicalAIReportData returned error: %v", err)
	}

	dailyHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, reportData)
	if err != nil {
		t.Fatalf("daily hash: %v", err)
	}
	weeklyHash, err := aiReportInputHash(aiReportTypeWeekly, defaultAIReportLocale, reportData)
	if err != nil {
		t.Fatalf("weekly hash: %v", err)
	}
	localeHash, err := aiReportInputHash(aiReportTypeDaily, "ro", reportData)
	if err != nil {
		t.Fatalf("locale hash: %v", err)
	}

	if dailyHash == weeklyHash {
		t.Fatal("hash should include report_type")
	}
	if dailyHash == localeHash {
		t.Fatal("hash should include locale")
	}
}

func TestValidAIReportRequestRequiresMatchingRangeLength(t *testing.T) {
	tests := []struct {
		name         string
		reportType   string
		daysIncluded int
		want         bool
	}{
		{name: "daily one day", reportType: aiReportTypeDaily, daysIncluded: 1, want: true},
		{name: "daily seven days", reportType: aiReportTypeDaily, daysIncluded: 7, want: false},
		{name: "weekly seven days", reportType: aiReportTypeWeekly, daysIncluded: 7, want: true},
		{name: "weekly one day", reportType: aiReportTypeWeekly, daysIncluded: 1, want: false},
		{name: "unknown", reportType: "month", daysIncluded: 31, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validAIReportRequest(tt.reportType, tt.daysIncluded); got != tt.want {
				t.Fatalf("validAIReportRequest = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateAIReportReturnsCachedContent(t *testing.T) {
	baby := store.Baby{
		ID:       uuid.New(),
		FamilyID: uuid.New(),
		Name:     "YauYau",
		Timezone: "Australia/Adelaide",
	}
	content := json.RawMessage(`{"schema_version":"ai_report_output.v1","title":"Cached report","summary":"Already generated.","highlights":[],"patterns":[],"comparison":[],"caveats":[],"questions_for_parent":[]}`)
	fake := &aiReportFakeStore{
		baby:          baby,
		cachedContent: content,
	}
	h := &Handlers{Store: fake}

	rec := httptest.NewRecorder()
	req := authenticatedAIReportRequest(t, baby.FamilyID, `{"report_type":"daily","start_date":"2026-07-13","end_date":"2026-07-13","delivery":"scheduled_email"}`)

	h.CreateAIReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != string(content) {
		t.Fatalf("body = %s, want cached content %s", rec.Body.String(), content)
	}
	if fake.cacheInputHash == "" {
		t.Fatal("cache lookup did not receive input hash")
	}
}

func TestCreateAIReportReturnsNotImplementedOnCacheMiss(t *testing.T) {
	baby := store.Baby{
		ID:       uuid.New(),
		FamilyID: uuid.New(),
		Name:     "YauYau",
		Timezone: "Australia/Adelaide",
	}
	h := &Handlers{Store: &aiReportFakeStore{
		baby:     baby,
		cacheErr: store.ErrNotFound,
	}}

	rec := httptest.NewRecorder()
	req := authenticatedAIReportRequest(t, baby.FamilyID, `{"report_type":"daily","start_date":"2026-07-13","end_date":"2026-07-13"}`)

	h.CreateAIReport(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AI report generation is not configured") {
		t.Fatalf("body = %s, want not implemented error", rec.Body.String())
	}
}

func TestCreateAIReportGeneratesAndCachesOnCacheMiss(t *testing.T) {
	baby := store.Baby{
		ID:       uuid.New(),
		FamilyID: uuid.New(),
		Name:     "YauYau",
		Timezone: "Australia/Adelaide",
	}
	output := json.RawMessage(`{"schema_version":"ai_report_output.v1","title":"Generated report","summary":"Generated from report data.","highlights":["One useful fact."],"patterns":[],"comparison":[],"caveats":[],"questions_for_parent":[]}`)
	fakeStore := &aiReportFakeStore{
		baby:     baby,
		cacheErr: store.ErrNotFound,
	}
	h := &Handlers{
		Store: fakeStore,
		AI:    &fakeAIReportGenerator{output: output, model: "test-model"},
	}

	rec := httptest.NewRecorder()
	req := authenticatedAIReportRequest(t, baby.FamilyID, `{"report_type":"daily","start_date":"2026-07-13","end_date":"2026-07-13"}`)

	h.CreateAIReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fakeStore.created.InputHash == "" {
		t.Fatal("generated report was not cached")
	}
	if fakeStore.created.Model != "test-model" {
		t.Fatalf("cached model = %q, want test-model", fakeStore.created.Model)
	}
	if !strings.Contains(rec.Body.String(), "Generated report") {
		t.Fatalf("body = %s, want generated report", rec.Body.String())
	}
}

func TestCreateAIReportRejectsInvalidGeneratedOutput(t *testing.T) {
	baby := store.Baby{
		ID:       uuid.New(),
		FamilyID: uuid.New(),
		Name:     "YauYau",
		Timezone: "Australia/Adelaide",
	}
	fakeStore := &aiReportFakeStore{
		baby:     baby,
		cacheErr: store.ErrNotFound,
	}
	h := &Handlers{
		Store: fakeStore,
		AI:    &fakeAIReportGenerator{output: json.RawMessage(`{"schema_version":"ai_report_output.v1","title":"","summary":"Missing title.","highlights":[],"patterns":[],"comparison":[],"caveats":[],"questions_for_parent":[]}`)},
	}

	rec := httptest.NewRecorder()
	req := authenticatedAIReportRequest(t, baby.FamilyID, `{"report_type":"daily","start_date":"2026-07-13","end_date":"2026-07-13"}`)

	h.CreateAIReport(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if fakeStore.created.InputHash != "" {
		t.Fatalf("invalid generated report should not be cached: %#v", fakeStore.created)
	}
}

func authenticatedAIReportRequest(t *testing.T, familyID uuid.UUID, body string) *http.Request {
	t.Helper()

	claims := jwt.RegisteredClaims{
		Subject:   uuid.NewString(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, struct {
		FamilyID string `json:"family_id"`
		jwt.RegisteredClaims
	}{
		FamilyID:         familyID.String(),
		RegisteredClaims: claims,
	})
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/babies/current/reports/ai", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+signed)

	var captured *http.Request
	authctx.Middleware("test-secret")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r
	})).ServeHTTP(httptest.NewRecorder(), req)
	if captured == nil {
		t.Fatal("auth middleware did not capture request")
	}
	return captured
}

type aiReportFakeStore struct {
	baby           store.Baby
	cachedContent  json.RawMessage
	cacheErr       error
	cacheInputHash string
	created        store.AIReportCache
}

func (s *aiReportFakeStore) GetBaby(context.Context, uuid.UUID) (store.Baby, error) {
	return store.Baby{}, store.ErrNotFound
}

func (s *aiReportFakeStore) GetCurrentBaby(context.Context, uuid.UUID) (store.Baby, error) {
	return s.baby, nil
}

func (s *aiReportFakeStore) CreateBaby(context.Context, uuid.UUID, string, string) (store.Baby, error) {
	return store.Baby{}, errors.New("not implemented")
}

func (s *aiReportFakeStore) UpdateBaby(context.Context, uuid.UUID, uuid.UUID, store.Baby) (store.Baby, error) {
	return store.Baby{}, errors.New("not implemented")
}

func (s *aiReportFakeStore) ArchiveBaby(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func (s *aiReportFakeStore) UpdateDailyReportEmailPreference(context.Context, uuid.UUID, uuid.UUID, bool) (store.FamilyMembership, error) {
	return store.FamilyMembership{}, errors.New("not implemented")
}

func (s *aiReportFakeStore) CreateEvent(context.Context, uuid.UUID, uuid.UUID, string, map[string]any, time.Time) (store.Event, error) {
	return store.Event{}, errors.New("not implemented")
}

func (s *aiReportFakeStore) GetEvent(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (store.Event, error) {
	return store.Event{}, errors.New("not implemented")
}

func (s *aiReportFakeStore) UpdateEvent(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, map[string]any, time.Time) (store.Event, error) {
	return store.Event{}, errors.New("not implemented")
}

func (s *aiReportFakeStore) ListAllEvents(context.Context, uuid.UUID, uuid.UUID, time.Time, time.Time, int) ([]store.Event, error) {
	return nil, nil
}

func (s *aiReportFakeStore) DeleteEvent(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func (s *aiReportFakeStore) GetBabyLatestGrowth(context.Context, uuid.UUID, uuid.UUID) (store.BabyLatestGrowth, error) {
	return store.BabyLatestGrowth{}, store.ErrNotFound
}

func (s *aiReportFakeStore) RefreshBabyLatestGrowth(context.Context, uuid.UUID, uuid.UUID) (store.BabyLatestGrowth, error) {
	return store.BabyLatestGrowth{}, errors.New("not implemented")
}

func (s *aiReportFakeStore) GetAIReportCache(_ context.Context, familyID, babyID uuid.UUID, reportType string, rangeStart, rangeEnd time.Time, inputHash string) (store.AIReportCache, error) {
	s.cacheInputHash = inputHash
	if s.cacheErr != nil {
		return store.AIReportCache{}, s.cacheErr
	}
	return store.AIReportCache{
		ID:          uuid.New(),
		FamilyID:    familyID,
		BabyID:      babyID,
		ReportType:  reportType,
		RangeStart:  rangeStart,
		RangeEnd:    rangeEnd,
		InputHash:   inputHash,
		ContentJSON: s.cachedContent,
	}, nil
}

func (s *aiReportFakeStore) CreateAIReportCache(_ context.Context, report store.AIReportCache) (store.AIReportCache, error) {
	s.created = report
	if s.created.ID == uuid.Nil {
		s.created.ID = uuid.New()
	}
	return s.created, nil
}

type fakeAIReportGenerator struct {
	output json.RawMessage
	model  string
	err    error
}

func (g *fakeAIReportGenerator) GenerateAIReport(context.Context, aireport.GenerationInput) (aireport.GenerationResult, error) {
	if g.err != nil {
		return aireport.GenerationResult{}, g.err
	}
	return aireport.GenerationResult{
		Model:       g.model,
		ContentJSON: g.output,
	}, nil
}
