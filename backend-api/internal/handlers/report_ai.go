package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const (
	aiReportInputSchemaVersion  = "ai_report_input.v1"
	aiReportOutputSchemaVersion = "ai_report_output.v1"
	aiReportPromptVersion       = "ai_report_prompt.v1"
	defaultAIReportLocale       = "en"

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

type aiReportHashEnvelope struct {
	InputSchemaVersion  string `json:"input_schema_version"`
	OutputSchemaVersion string `json:"output_schema_version"`
	PromptVersion       string `json:"prompt_version"`
	ReportType          string `json:"report_type"`
	Locale              string `json:"locale"`
	ReportData          any    `json:"report_data"`
}

// CreateAIReport returns cached AI report JSON for the selected range, or a
// clear 501 while generation is intentionally not implemented yet. This
// endpoint is expected to be leveraged by future MCP tools and scheduled
// email jobs once AI generation exists.
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

	inputHash, err := aiReportInputHash(req.ReportType, req.Locale, reportData)
	if err != nil {
		log.Printf("hash AI report input: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to prepare AI report")
		return
	}

	cached, err := h.Store.GetAIReportCache(r.Context(), baby.FamilyID, baby.ID, req.ReportType, window.RangeStart, window.RangeEnd, inputHash)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotImplemented, aiReportCacheMissResponse{
			Error:      "AI report generation is not implemented yet",
			ReportType: req.ReportType,
			StartDate:  window.StartDate,
			EndDate:    window.EndDate,
			InputHash:  inputHash,
		})
		return
	}
	if err != nil {
		log.Printf("get AI report cache: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load AI report")
		return
	}

	writeRawJSON(w, http.StatusOK, cached.ContentJSON)
}

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

func normalizeAIReportLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return defaultAIReportLocale
	}
	return locale
}

func aiReportInputHash(reportType, locale string, reportData reportDataResponse) (string, error) {
	canonicalReportData, err := canonicalAIReportData(reportData)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(aiReportHashEnvelope{
		InputSchemaVersion:  aiReportInputSchemaVersion,
		OutputSchemaVersion: aiReportOutputSchemaVersion,
		PromptVersion:       aiReportPromptVersion,
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
