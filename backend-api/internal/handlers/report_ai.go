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
	"strings"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aireport"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const (
	defaultAIReportLocale = "en"

	aiReportTypeDaily  = "daily"
	aiReportTypeWeekly = "weekly"
)

var (
	errInvalidAIReportRequest         = errors.New("invalid AI report request")
	errAIReportGenerationUnconfigured = errors.New("AI report generation is not configured")
	errAIReportGenerationFailed       = errors.New("AI report generation failed")
	errAIReportOutputInvalid          = errors.New("AI report output invalid")
)

type aiReportRequest struct {
	ReportType string `json:"report_type"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	Delivery   string `json:"delivery,omitempty"`
	Locale     string `json:"locale,omitempty"`
}

type aiReportCacheMissResponse struct {
	Error      string `json:"error"`
	ReportType string `json:"report_type"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	InputHash  string `json:"input_hash"`
}

type aiReportResult struct {
	Cache     store.AIReportCache
	Window    reportDataWindow
	InputHash string
}

// aiReportHashEnvelope is the stable cache identity payload. Delivery is not
// included here because v1 AI report content is channel-neutral.
type aiReportHashEnvelope struct {
	InputSchemaVersion  string `json:"input_schema_version"`
	OutputSchemaVersion string `json:"output_schema_version"`
	PromptVersion       string `json:"prompt_version"`
	ReportType          string `json:"report_type"`
	Locale              string `json:"locale"`
	ReportData          any    `json:"report_data"`
}

// CreateAIReport returns cached AI report JSON for the selected range, or
// generates and caches it when AI generation is configured. This endpoint is
// expected to be leveraged by future MCP tools and scheduled email jobs.
func (h *Handlers) CreateAIReport(w http.ResponseWriter, r *http.Request) {
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	var req aiReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.ReportType = strings.TrimSpace(req.ReportType)
	req.StartDate = strings.TrimSpace(req.StartDate)
	req.EndDate = strings.TrimSpace(req.EndDate)
	req.Locale = normalizeAIReportLocale(req.Locale)

	result, err := h.loadOrCreateAIReport(r.Context(), baby, req, time.Now())
	if errors.Is(err, errInvalidReportDataRange) || errors.Is(err, errInvalidAIReportRequest) {
		writeError(w, http.StatusBadRequest, "invalid report type or date range")
		return
	}
	if errors.Is(err, errAIReportGenerationUnconfigured) {
		writeJSON(w, http.StatusNotImplemented, aiReportCacheMissResponse{
			Error:      "AI report generation is not configured",
			ReportType: req.ReportType,
			StartDate:  result.Window.StartDate,
			EndDate:    result.Window.EndDate,
			InputHash:  result.InputHash,
		})
		return
	}
	if errors.Is(err, errAIReportGenerationFailed) || errors.Is(err, errAIReportOutputInvalid) {
		log.Printf("generate AI report: %v", err)
		writeError(w, http.StatusBadGateway, "failed to generate AI report")
		return
	}
	if err != nil {
		log.Printf("load or create AI report: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load AI report")
		return
	}

	writeRawJSON(w, http.StatusOK, result.Cache.ContentJSON)
}

// loadOrCreateAIReport is shared by the HTTP endpoint and scheduled email
// delivery so both paths use the same report-data, hash, generation, and
// validation rules.
func (h *Handlers) loadOrCreateAIReport(ctx context.Context, baby store.Baby, req aiReportRequest, now time.Time) (aiReportResult, error) {
	reportData, window, err := h.buildReportDataForBaby(ctx, baby, req.StartDate, req.EndDate, now)
	if err != nil {
		return aiReportResult{}, err
	}
	if !validAIReportRequest(req.ReportType, window.DaysIncluded) {
		return aiReportResult{Window: window}, errInvalidAIReportRequest
	}

	generationReportData, err := canonicalAIReportData(reportData)
	if err != nil {
		return aiReportResult{Window: window}, fmt.Errorf("canonicalizing AI report data: %w", err)
	}
	hashReportData, err := canonicalAIReportHashData(reportData, window)
	if err != nil {
		return aiReportResult{Window: window}, fmt.Errorf("canonicalizing AI report cache identity: %w", err)
	}

	inputHash, err := aiReportInputHash(req.ReportType, req.Locale, hashReportData)
	if err != nil {
		return aiReportResult{Window: window}, fmt.Errorf("hashing AI report input: %w", err)
	}
	result := aiReportResult{Window: window, InputHash: inputHash}
	cacheRangeEnd := aiReportCacheRangeEnd(window)

	cached, err := h.Store.GetAIReportCache(ctx, baby.FamilyID, baby.ID, req.ReportType, window.RangeStart, cacheRangeEnd, inputHash)
	if err == nil {
		result.Cache = cached
		return result, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return result, fmt.Errorf("getting AI report cache: %w", err)
	}

	if h.AI == nil {
		return result, errAIReportGenerationUnconfigured
	}

	generated, err := h.AI.GenerateAIReport(ctx, aireport.GenerationInput{
		InputSchemaVersion:  aireport.InputSchemaVersion,
		OutputSchemaVersion: aireport.OutputSchemaVersion,
		PromptVersion:       aireport.PromptVersion,
		ReportType:          req.ReportType,
		Locale:              req.Locale,
		ReportData:          generationReportData,
	})
	if err != nil {
		return result, fmt.Errorf("%w: %v", errAIReportGenerationFailed, err)
	}

	contentJSON, err := validateAIReportOutput(generated.ContentJSON)
	if err != nil {
		return result, fmt.Errorf("%w: %v", errAIReportOutputInvalid, err)
	}

	cached, err = h.Store.CreateAIReportCache(ctx, store.AIReportCache{
		FamilyID:            baby.FamilyID,
		BabyID:              baby.ID,
		ReportType:          req.ReportType,
		RangeStart:          window.RangeStart,
		RangeEnd:            cacheRangeEnd,
		InputHash:           inputHash,
		PromptVersion:       aireport.PromptVersion,
		InputSchemaVersion:  aireport.InputSchemaVersion,
		OutputSchemaVersion: aireport.OutputSchemaVersion,
		Model:               generated.Model,
		ContentJSON:         contentJSON,
	})
	if err != nil {
		return result, fmt.Errorf("caching AI report: %w", err)
	}

	result.Cache = cached
	return result, nil
}

// validAIReportRequest ties report_type to the supported date-window sizes
// so "daily" cannot silently generate a week-sized report and vice versa.
func validAIReportRequest(reportType string, daysIncluded int) bool {
	switch reportType {
	case aiReportTypeDaily:
		return daysIncluded == 1
	case aiReportTypeWeekly:
		return daysIncluded == 7
	default:
		return false
	}
}

// normalizeAIReportLocale keeps the endpoint usable before the frontend or
// scheduled jobs send locale explicitly.
func normalizeAIReportLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return defaultAIReportLocale
	}
	return locale
}

// aiReportInputHash hashes only deterministic, semantic inputs. Volatile
// generated_at values must already be removed from canonicalReportData.
func aiReportInputHash(reportType, locale string, canonicalReportData any) (string, error) {
	payload, err := json.Marshal(aiReportHashEnvelope{
		InputSchemaVersion:  aireport.InputSchemaVersion,
		OutputSchemaVersion: aireport.OutputSchemaVersion,
		PromptVersion:       aireport.PromptVersion,
		ReportType:          reportType,
		Locale:              locale,
		ReportData:          canonicalReportData,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalAIReportData converts the typed report-data response into a
// generic JSON tree so volatile generated_at fields can be removed before
// hashing or sending to the model.
func canonicalAIReportData(reportData reportDataResponse) (any, error) {
	raw, err := json.Marshal(reportData)
	if err != nil {
		return nil, err
	}

	var canonical any
	if err := json.Unmarshal(raw, &canonical); err != nil {
		return nil, err
	}
	removeGeneratedAt(canonical)
	return canonical, nil
}

// canonicalAIReportHashData keeps the cache identity stable for a partial
// current-day report when only the wall clock advances. The model still sees
// the actual cutoff from canonicalAIReportData; the hash uses the stable end
// of the selected local calendar day and changes when semantic report data
// (including events) changes.
func canonicalAIReportHashData(reportData reportDataResponse, window reportDataWindow) (any, error) {
	if window.IsPartial {
		stableRangeEnd := aiReportCacheRangeEnd(window)
		reportData.Range.RangeEnd = stableRangeEnd
		reportData.Days = append([]reportDayResponse(nil), reportData.Days...)
		for i := range reportData.Days {
			if !reportData.Days[i].IsPartial {
				continue
			}
			reportData.Days[i].RangeEnd = stableRangeEnd
			reportData.Days[i].Report.RangeEnd = stableRangeEnd
		}
	}
	return canonicalAIReportData(reportData)
}

func aiReportCacheRangeEnd(window reportDataWindow) time.Time {
	if window.IsPartial {
		return window.EndStart.AddDate(0, 0, 1)
	}
	return window.RangeEnd
}

// removeGeneratedAt recursively drops generated_at timestamps. Those values
// describe when backend assembled the report, not what happened in the baby
// timeline, so they should not affect cache identity.
func removeGeneratedAt(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "generated_at")
		for _, child := range typed {
			removeGeneratedAt(child)
		}
	case []any:
		for _, child := range typed {
			removeGeneratedAt(child)
		}
	}
}

// validateAIReportOutput enforces the backend-owned output contract before
// generated JSON is cached or returned to callers.
func validateAIReportOutput(raw json.RawMessage) (json.RawMessage, error) {
	var output aireport.Output
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("decode output: %w", err)
	}
	if output.SchemaVersion != aireport.OutputSchemaVersion {
		return nil, fmt.Errorf("schema_version = %q, want %q", output.SchemaVersion, aireport.OutputSchemaVersion)
	}
	if strings.TrimSpace(output.Title) == "" {
		return nil, errors.New("title is required")
	}
	if strings.TrimSpace(output.Summary) == "" {
		return nil, errors.New("summary is required")
	}
	if output.Highlights == nil {
		return nil, errors.New("highlights is required")
	}
	if len(output.Highlights) > 4 {
		return nil, errors.New("highlights exceeds max 4")
	}
	if output.Patterns == nil {
		return nil, errors.New("patterns is required")
	}
	if len(output.Patterns) > 3 {
		return nil, errors.New("patterns exceeds max 3")
	}
	if output.Comparison == nil {
		return nil, errors.New("comparison is required")
	}
	if len(output.Comparison) > 3 {
		return nil, errors.New("comparison exceeds max 3")
	}
	if output.Caveats == nil {
		return nil, errors.New("caveats is required")
	}
	if len(output.Caveats) > 2 {
		return nil, errors.New("caveats exceeds max 2")
	}
	if output.QuestionsForParent == nil {
		return nil, errors.New("questions_for_parent is required")
	}
	if len(output.QuestionsForParent) > 3 {
		return nil, errors.New("questions_for_parent exceeds max 3")
	}

	normalized, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("encode normalized output: %w", err)
	}
	return normalized, nil
}
