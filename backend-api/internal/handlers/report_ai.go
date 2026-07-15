package handlers

import (
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

	reportData, window, err := h.buildReportDataForBaby(r.Context(), baby, req.StartDate, req.EndDate, time.Now())
	if err != nil {
		if errors.Is(err, errInvalidReportDataRange) {
			writeError(w, http.StatusBadRequest, "invalid report date range")
			return
		}
		log.Printf("build AI report data: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load report data")
		return
	}
	if !validAIReportRequest(req.ReportType, window.DaysIncluded) {
		writeError(w, http.StatusBadRequest, "invalid report type or date range")
		return
	}

	canonicalReportData, err := canonicalAIReportData(reportData)
	if err != nil {
		log.Printf("canonicalize AI report data: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to prepare AI report")
		return
	}

	inputHash, err := aiReportInputHash(req.ReportType, req.Locale, canonicalReportData)
	if err != nil {
		log.Printf("hash AI report input: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to prepare AI report")
		return
	}

	cached, err := h.Store.GetAIReportCache(r.Context(), baby.FamilyID, baby.ID, req.ReportType, window.RangeStart, window.RangeEnd, inputHash)
	if errors.Is(err, store.ErrNotFound) {
		h.generateAndCacheAIReport(w, r, baby, req, window, canonicalReportData, inputHash)
		return
	}
	if err != nil {
		log.Printf("get AI report cache: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load AI report")
		return
	}

	writeRawJSON(w, http.StatusOK, cached.ContentJSON)
}

// generateAndCacheAIReport is the cache-miss path. It keeps model I/O,
// output validation, and cache persistence together so invalid model output
// can never be stored.
func (h *Handlers) generateAndCacheAIReport(w http.ResponseWriter, r *http.Request, baby store.Baby, req aiReportRequest, window reportDataWindow, reportData any, inputHash string) {
	if h.AI == nil {
		writeJSON(w, http.StatusNotImplemented, aiReportCacheMissResponse{
			Error:      "AI report generation is not configured",
			ReportType: req.ReportType,
			StartDate:  window.StartDate,
			EndDate:    window.EndDate,
			InputHash:  inputHash,
		})
		return
	}

	result, err := h.AI.GenerateAIReport(r.Context(), aireport.GenerationInput{
		InputSchemaVersion:  aireport.InputSchemaVersion,
		OutputSchemaVersion: aireport.OutputSchemaVersion,
		PromptVersion:       aireport.PromptVersion,
		ReportType:          req.ReportType,
		Locale:              req.Locale,
		ReportData:          reportData,
	})
	if err != nil {
		log.Printf("generate AI report: %v", err)
		writeError(w, http.StatusBadGateway, "failed to generate AI report")
		return
	}

	contentJSON, err := validateAIReportOutput(result.ContentJSON)
	if err != nil {
		log.Printf("validate AI report output: %v", err)
		writeError(w, http.StatusBadGateway, "AI report output was invalid")
		return
	}

	cached, err := h.Store.CreateAIReportCache(r.Context(), store.AIReportCache{
		FamilyID:            baby.FamilyID,
		BabyID:              baby.ID,
		ReportType:          req.ReportType,
		RangeStart:          window.RangeStart,
		RangeEnd:            window.RangeEnd,
		InputHash:           inputHash,
		PromptVersion:       aireport.PromptVersion,
		InputSchemaVersion:  aireport.InputSchemaVersion,
		OutputSchemaVersion: aireport.OutputSchemaVersion,
		Model:               result.Model,
		ContentJSON:         contentJSON,
	})
	if err != nil {
		log.Printf("cache AI report: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to cache AI report")
		return
	}

	writeRawJSON(w, http.StatusOK, cached.ContentJSON)
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
	if len(output.Highlights) > 5 {
		return nil, errors.New("highlights exceeds max 5")
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
