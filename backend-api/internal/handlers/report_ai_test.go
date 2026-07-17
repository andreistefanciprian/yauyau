package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	firstHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, "", canonical)
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
	secondHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, "", canonical)
	if err != nil {
		t.Fatalf("aiReportInputHash returned error: %v", err)
	}

	if secondHash != firstHash {
		t.Fatalf("hash changed after generated_at changes: %s vs %s", secondHash, firstHash)
	}
}

func TestAIReportPartialCacheIdentityIgnoresMovingCutoff(t *testing.T) {
	loc := mustLoadLocation(t, "Australia/Adelaide")
	dayStart := time.Date(2026, 7, 13, 0, 0, 0, 0, loc)
	firstCutoff := time.Date(2026, 7, 13, 9, 30, 0, 0, loc)
	window := reportDataWindow{
		EndStart:  dayStart,
		RangeEnd:  firstCutoff,
		IsPartial: true,
	}
	reportData := reportDataResponse{
		Range: reportRangeResponse{
			StartDate:    "2026-07-13",
			EndDate:      "2026-07-13",
			DaysIncluded: 1,
			IsPartial:    true,
			RangeStart:   dayStart,
			RangeEnd:     firstCutoff,
		},
		Days: []reportDayResponse{{
			LocalDate:  "2026-07-13",
			RangeStart: dayStart,
			RangeEnd:   firstCutoff,
			IsPartial:  true,
			Report: dailyReportResponse{
				RangeStart: dayStart,
				RangeEnd:   firstCutoff,
			},
		}},
	}

	firstData, err := canonicalAIReportHashData(reportData, window)
	if err != nil {
		t.Fatalf("canonicalize first partial report: %v", err)
	}
	firstHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, "", firstData)
	if err != nil {
		t.Fatalf("hash first partial report: %v", err)
	}

	secondCutoff := firstCutoff.Add(2 * time.Hour)
	window.RangeEnd = secondCutoff
	reportData.Range.RangeEnd = secondCutoff
	reportData.Days[0].RangeEnd = secondCutoff
	reportData.Days[0].Report.RangeEnd = secondCutoff
	secondData, err := canonicalAIReportHashData(reportData, window)
	if err != nil {
		t.Fatalf("canonicalize second partial report: %v", err)
	}
	secondHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, "", secondData)
	if err != nil {
		t.Fatalf("hash second partial report: %v", err)
	}
	if secondHash != firstHash {
		t.Fatalf("partial report hash changed with cutoff only: %s vs %s", secondHash, firstHash)
	}
	if got, want := aiReportCacheRangeEnd(window), dayStart.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("cache range end = %s, want stable day boundary %s", got, want)
	}

	reportData.Totals.EventCount = 1
	changedData, err := canonicalAIReportHashData(reportData, window)
	if err != nil {
		t.Fatalf("canonicalize changed partial report: %v", err)
	}
	changedHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, "", changedData)
	if err != nil {
		t.Fatalf("hash changed partial report: %v", err)
	}
	if changedHash == firstHash {
		t.Fatal("partial report hash did not change with semantic report data")
	}
}

func TestAIReportInputHashIncludesSemanticInputs(t *testing.T) {
	reportData, err := canonicalAIReportData(reportDataResponse{})
	if err != nil {
		t.Fatalf("canonicalAIReportData returned error: %v", err)
	}

	dailyHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, "", reportData)
	if err != nil {
		t.Fatalf("daily hash: %v", err)
	}
	weeklyHash, err := aiReportInputHash(aiReportTypeWeekly, defaultAIReportLocale, "", reportData)
	if err != nil {
		t.Fatalf("weekly hash: %v", err)
	}
	localeHash, err := aiReportInputHash(aiReportTypeDaily, "ro", "", reportData)
	if err != nil {
		t.Fatalf("locale hash: %v", err)
	}

	if dailyHash == weeklyHash {
		t.Fatal("hash should include report_type")
	}
	if dailyHash == localeHash {
		t.Fatal("hash should include locale")
	}
	relationshipHash, err := aiReportInputHash(aiReportTypeDaily, defaultAIReportLocale, "Dad", reportData)
	if err != nil {
		t.Fatalf("relationship hash: %v", err)
	}
	if dailyHash == relationshipHash {
		t.Fatal("hash should include viewer relationship")
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
	content := json.RawMessage(`{"schema_version":"ai_report_output.v2","title":"Cached report","summary":"Already generated.","highlights":[],"patterns":[],"comparison":[],"caveats":[],"questions_for_parent":[],"daily_card":{"intro":"Here's how YauYau's day took shape.","story":"","observation":"The day is captured here.","encouragement":"You've got this."}}`)
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
	output := json.RawMessage(`{"schema_version":"ai_report_output.v2","title":"Generated report","summary":"Generated from report data.","highlights":["One useful fact."],"patterns":[],"comparison":[],"caveats":[],"questions_for_parent":[],"daily_card":{"intro":"Here's how YauYau's day took shape.","story":"","observation":"The day is captured here.","encouragement":"You've got this, Dad."}}`)
	fakeStore := &aiReportFakeStore{
		baby:     baby,
		cacheErr: store.ErrNotFound,
	}
	generator := &fakeAIReportGenerator{output: output, model: "test-model"}
	h := &Handlers{
		Store: fakeStore,
		FamilyStore: &aiReportRelationshipFamilyStore{membership: store.FamilyMembership{
			Found:        true,
			FamilyID:     &baby.FamilyID,
			Status:       store.MembershipStatusActive,
			Relationship: "Dad",
		}},
		AI: generator,
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
	if generator.input.ViewerRelationship != "Dad" {
		t.Fatalf("viewer relationship = %q, want Dad", generator.input.ViewerRelationship)
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
		AI:    &fakeAIReportGenerator{output: json.RawMessage(`{"schema_version":"ai_report_output.v2","title":"","summary":"Missing title.","highlights":[],"patterns":[],"comparison":[],"caveats":[],"questions_for_parent":[],"daily_card":{"intro":"Here's how YauYau's day took shape.","story":"","observation":"The day is captured here.","encouragement":"You've got this."}}`)},
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

func TestCreateAIReportHidesProviderFailure(t *testing.T) {
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
		AI:    &fakeAIReportGenerator{err: errors.New("provider secret failure")},
	}

	rec := httptest.NewRecorder()
	req := authenticatedAIReportRequest(t, baby.FamilyID, `{"report_type":"daily","start_date":"2026-07-13","end_date":"2026-07-13"}`)

	h.CreateAIReport(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "provider secret failure") {
		t.Fatalf("body exposes provider error: %s", rec.Body.String())
	}
	if fakeStore.created.InputHash != "" {
		t.Fatalf("provider failure should not be cached: %#v", fakeStore.created)
	}
}

func TestValidateAIReportOutputRejectsTooManyHighlights(t *testing.T) {
	raw := json.RawMessage(`{
		"schema_version":"ai_report_output.v2",
		"title":"Generated report",
		"summary":"One useful takeaway.",
		"highlights":["One","Two","Three","Four","Five"],
		"patterns":[],
		"comparison":[],
		"caveats":[],
		"questions_for_parent":[],
		"daily_card":{"intro":"","story":"","observation":"","encouragement":""}
	}`)

	if _, err := validateAIReportOutput(raw, aiReportTypeWeekly, reportDataResponse{}, ""); err == nil || !strings.Contains(err.Error(), "highlights exceeds max 4") {
		t.Fatalf("validateAIReportOutput err = %v, want max highlights error", err)
	}
}

func TestValidateDailyCardProductRules(t *testing.T) {
	baseData := reportDataResponse{
		Baby:  reportBabyResponse{Name: "YauYau"},
		Range: reportRangeResponse{IsPartial: true},
		Totals: reportTotalsResponse{
			Nappies: reportNappyTotals{Count: 4},
			Pumps:   reportPumpTotals{Count: 2, TotalMl: 325},
			Growth:  reportGrowthTotals{Count: 1},
		},
	}
	baseCard := aireport.DailyCard{
		Intro:         "Here's how YauYau's day is taking shape.",
		Story:         "The day also included plenty of nappy changes, two pumping sessions totalling 325 ml, and a growth measurement.",
		Observation:   "The day is still unfolding.",
		Encouragement: "You've got this, Dad. 💛",
	}

	tests := []struct {
		name         string
		data         reportDataResponse
		relationship string
		card         aireport.DailyCard
		wantError    string
	}{
		{name: "dad with one encouragement emoji", data: baseData, relationship: "Dad", card: baseCard},
		{
			name:         "mum relationship",
			data:         baseData,
			relationship: "Mum",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Encouragement = "You've got this, Mum."
				return card
			}(),
		},
		{
			name: "missing relationship",
			data: baseData,
			card: func() aireport.DailyCard {
				card := baseCard
				card.Encouragement = "You've got this."
				return card
			}(),
		},
		{
			name: "baby name is not mistaken for a relationship",
			data: func() reportDataResponse {
				data := baseData
				data.Baby.Name = "Mum"
				return data
			}(),
			card: func() aireport.DailyCard {
				card := baseCard
				card.Intro = "Here's how Mum's day is taking shape."
				card.Encouragement = "You've got this."
				return card
			}(),
		},
		{
			name: "baby name is counted as a complete mention",
			data: func() reportDataResponse {
				data := baseData
				data.Baby.Name = "Ann"
				return data
			}(),
			card: func() aireport.DailyCard {
				card := baseCard
				card.Intro = "Here's how Ann's day is taking shape."
				card.Observation = "Another part of the day is still unfolding."
				card.Encouragement = "You've got this."
				return card
			}(),
		},
		{
			name: "missing relationship cannot assume dad",
			data: baseData,
			card: func() aireport.DailyCard {
				card := baseCard
				card.Encouragement = "You've got this, Dad."
				return card
			}(),
			wantError: "must not be assumed",
		},
		{
			name: "missing baby name",
			data: func() reportDataResponse {
				data := baseData
				data.Baby.Name = ""
				return data
			}(),
			relationship: "Dad",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Intro = "Here's how your little one's day is taking shape."
				return card
			}(),
		},
		{
			name:         "growth omitted",
			data:         baseData,
			relationship: "Dad",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Story = "The day also included plenty of nappy changes and two pumping sessions totalling 325 ml."
				return card
			}(),
			wantError: "growth measurement must be mentioned",
		},
		{
			name:         "nappy subtype exposed",
			data:         baseData,
			relationship: "Dad",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Story = "The day also included mixed nappy changes, two pumping sessions totalling 325 ml, and a growth measurement."
				return card
			}(),
			wantError: "nappy detail",
		},
		{
			name:         "dash punctuation",
			data:         baseData,
			relationship: "Dad",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Observation = "The day is well-documented and still unfolding."
				return card
			}(),
			wantError: "hyphen or dash",
		},
		{
			name:         "model Markdown",
			data:         baseData,
			relationship: "Dad",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Observation = "**The day is still unfolding.**"
				return card
			}(),
			wantError: "Markdown or HTML",
		},
		{
			name:         "emoji in story",
			data:         baseData,
			relationship: "Dad",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Story += " 💛"
				card.Encouragement = "You've got this, Dad."
				return card
			}(),
			wantError: "emoji is allowed only",
		},
		{
			name:         "two emojis",
			data:         baseData,
			relationship: "Dad",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Observation += " 💛"
				return card
			}(),
			wantError: "at most one emoji",
		},
		{
			name:         "medical reassurance",
			data:         baseData,
			relationship: "Dad",
			card: func() aireport.DailyCard {
				card := baseCard
				card.Observation = "Everything looks normal."
				return card
			}(),
			wantError: "medical or evaluative",
		},
		{
			name: "historical day rejects partial wording",
			data: func() reportDataResponse {
				data := baseData
				data.Range.IsPartial = false
				return data
			}(),
			relationship: "Dad",
			card:         baseCard,
			wantError:    "partial day language is not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := mustMarshalAIOutput(t, tt.card)
			_, err := validateAIReportOutput(raw, aiReportTypeDaily, tt.data, tt.relationship)
			if tt.wantError == "" && err != nil {
				t.Fatalf("validateAIReportOutput returned error: %v", err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("validateAIReportOutput error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func mustMarshalAIOutput(t *testing.T, card aireport.DailyCard) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(aireport.Output{
		SchemaVersion:      aireport.OutputSchemaVersion,
		Title:              "Today so far",
		Summary:            "A concise report.",
		Highlights:         []string{},
		Patterns:           []string{},
		Comparison:         []string{},
		Caveats:            []string{},
		QuestionsForParent: []string{},
		DailyCard:          card,
	})
	if err != nil {
		t.Fatalf("marshal AI output: %v", err)
	}
	return raw
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
	baby                 store.Baby
	cachedContent        json.RawMessage
	cacheErr             error
	cacheInputHash       string
	created              store.AIReportCache
	dailyReportEmailJobs []store.DailyReportEmailJob
	deliveries           map[string]store.AIReportEmailDelivery
	sentDeliveries       []store.AIReportEmailDelivery
	failedDeliveries     []store.AIReportEmailDelivery
}

type aiReportRelationshipFamilyStore struct {
	FamilyStore
	membership store.FamilyMembership
}

func (s *aiReportRelationshipFamilyStore) GetFamilyMembershipForFamily(context.Context, uuid.UUID, uuid.UUID) (store.FamilyMembership, error) {
	return s.membership, nil
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

func (s *aiReportFakeStore) ListDueDailyReportEmailJobs(context.Context, time.Time) ([]store.DailyReportEmailJob, error) {
	return s.dailyReportEmailJobs, nil
}

func (s *aiReportFakeStore) CreateAIReportEmailDelivery(_ context.Context, delivery store.AIReportEmailDelivery) (store.AIReportEmailDelivery, error) {
	if s.deliveries == nil {
		s.deliveries = map[string]store.AIReportEmailDelivery{}
	}
	key := fakeDeliveryKey(delivery.FamilyID, delivery.BabyID, delivery.RecipientUserID, delivery.ReportType, delivery.RangeStart, delivery.RangeEnd, delivery.ScheduledFor)
	if existing, ok := s.deliveries[key]; ok {
		return existing, nil
	}
	if delivery.ID == uuid.Nil {
		delivery.ID = uuid.New()
	}
	if delivery.Status == "" {
		delivery.Status = store.AIReportEmailDeliveryStatusPending
	}
	s.deliveries[key] = delivery
	return delivery, nil
}

func (s *aiReportFakeStore) MarkAIReportEmailDeliverySent(_ context.Context, id, aiReportCacheID uuid.UUID, providerMessageID string, sentAt time.Time) (store.AIReportEmailDelivery, error) {
	delivery, key, ok := s.fakeDeliveryByID(id)
	if !ok {
		return store.AIReportEmailDelivery{}, store.ErrNotFound
	}
	delivery.Status = store.AIReportEmailDeliveryStatusSent
	delivery.AIReportCacheID = &aiReportCacheID
	delivery.ProviderMessageID = providerMessageID
	delivery.AttemptedAt = &sentAt
	delivery.SentAt = &sentAt
	s.deliveries[key] = delivery
	s.sentDeliveries = append(s.sentDeliveries, delivery)
	return delivery, nil
}

func (s *aiReportFakeStore) ClaimAIReportEmailDelivery(_ context.Context, id uuid.UUID, claimedAt time.Time) (store.AIReportEmailDelivery, error) {
	delivery, key, ok := s.fakeDeliveryByID(id)
	if !ok {
		return store.AIReportEmailDelivery{}, store.ErrNotFound
	}
	if delivery.Status == store.AIReportEmailDeliveryStatusSending {
		if delivery.AttemptedAt == nil || !delivery.AttemptedAt.Before(claimedAt.Add(-time.Hour)) {
			return store.AIReportEmailDelivery{}, store.ErrNotFound
		}
	} else if delivery.Status != store.AIReportEmailDeliveryStatusPending && delivery.Status != store.AIReportEmailDeliveryStatusFailed {
		return store.AIReportEmailDelivery{}, store.ErrNotFound
	}
	delivery.Status = store.AIReportEmailDeliveryStatusSending
	delivery.ErrorMessage = ""
	delivery.AttemptedAt = &claimedAt
	delivery.SentAt = nil
	s.deliveries[key] = delivery
	return delivery, nil
}

func (s *aiReportFakeStore) MarkAIReportEmailDeliveryFailed(_ context.Context, id uuid.UUID, errorMessage string, attemptedAt time.Time) (store.AIReportEmailDelivery, error) {
	delivery, key, ok := s.fakeDeliveryByID(id)
	if !ok {
		return store.AIReportEmailDelivery{}, store.ErrNotFound
	}
	delivery.Status = store.AIReportEmailDeliveryStatusFailed
	delivery.ErrorMessage = errorMessage
	delivery.AttemptedAt = &attemptedAt
	delivery.SentAt = nil
	s.deliveries[key] = delivery
	s.failedDeliveries = append(s.failedDeliveries, delivery)
	return delivery, nil
}

func (s *aiReportFakeStore) fakeDeliveryByID(id uuid.UUID) (store.AIReportEmailDelivery, string, bool) {
	for key, delivery := range s.deliveries {
		if delivery.ID == id {
			return delivery, key, true
		}
	}
	return store.AIReportEmailDelivery{}, "", false
}

func fakeDeliveryKey(familyID, babyID, recipientUserID uuid.UUID, reportType string, rangeStart, rangeEnd, scheduledFor time.Time) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s", familyID, babyID, recipientUserID, reportType, rangeStart.Format(time.RFC3339Nano), rangeEnd.Format(time.RFC3339Nano), scheduledFor.Format(time.RFC3339Nano))
}

type fakeAIReportGenerator struct {
	output json.RawMessage
	model  string
	err    error
	input  aireport.GenerationInput
}

func (g *fakeAIReportGenerator) GenerateAIReport(_ context.Context, input aireport.GenerationInput) (aireport.GenerationResult, error) {
	g.input = input
	if g.err != nil {
		return aireport.GenerationResult{}, g.err
	}
	return aireport.GenerationResult{
		Model:       g.model,
		ContentJSON: g.output,
	}, nil
}
