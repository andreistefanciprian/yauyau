package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/authctx"
	"github.com/andreistefanciprian/yauli/backend-api/internal/dailycard"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const (
	dailyCardCacheReportType = "daily_card"
	dailyCardLocale          = "en-AU"
	dailyCardCacheTTL        = 2 * time.Hour
)

var (
	errDailyCardGenerationUnconfigured = errors.New("daily card generation is not configured")
	errDailyCardGenerationFailed       = errors.New("daily card generation failed")
	errDailyCardOutputInvalid          = errors.New("daily card output invalid")
)

type dailyCardViewer struct {
	Relationship string `json:"relationship"`
}

// dailyCardInput is the complete JSON passed to GenerateDailyCard. ReportData
// is the unmodified current-day buildReportDataForBaby response, including
// generated, range, analytics, and event timestamps.
type dailyCardInput struct {
	SchemaVersion       string             `json:"schema_version"`
	OutputSchemaVersion string             `json:"output_schema_version"`
	Locale              string             `json:"locale"`
	Viewer              dailyCardViewer    `json:"viewer"`
	ReportData          reportDataResponse `json:"report_data"`
}

type dailyCardHashEnvelope struct {
	InputSchemaVersion  string `json:"input_schema_version"`
	OutputSchemaVersion string `json:"output_schema_version"`
	PromptVersion       string `json:"prompt_version"`
	Locale              string `json:"locale"`
	ViewerRelationship  string `json:"viewer_relationship,omitempty"`
	ReportData          any    `json:"report_data"`
}

type dailyCardResult struct {
	Cache     store.AIReportCache
	Window    reportDataWindow
	InputHash string
}

// CreateAIDailyCard returns the AI copy for today's card only. Historical
// timeline days continue to use the deterministic daily report.
func (h *Handlers) CreateAIDailyCard(w http.ResponseWriter, r *http.Request) {
	baby, ok := h.currentBabyForRequest(w, r)
	if !ok {
		return
	}

	relationship, err := h.currentViewerRelationship(r.Context(), baby.FamilyID)
	if err != nil {
		log.Printf("load daily card viewer relationship: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load daily card")
		return
	}

	result, err := h.loadOrCreateDailyCard(r.Context(), baby, relationship, time.Now())
	if errors.Is(err, errDailyCardGenerationUnconfigured) {
		writeError(w, http.StatusNotImplemented, "daily card generation is not configured")
		return
	}
	if errors.Is(err, errDailyCardGenerationFailed) || errors.Is(err, errDailyCardOutputInvalid) {
		log.Printf("generate daily card: %v", err)
		writeError(w, http.StatusBadGateway, "failed to generate daily card")
		return
	}
	if err != nil {
		log.Printf("load or create daily card: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load daily card")
		return
	}

	writeRawJSON(w, http.StatusOK, result.Cache.ContentJSON)
}

func (h *Handlers) loadOrCreateDailyCard(ctx context.Context, baby store.Baby, relationship string, now time.Time) (dailyCardResult, error) {
	relationship = parentFacingRelationship(relationship)
	reportData, window, err := h.buildReportDataForBaby(ctx, baby, "", "", now)
	if err != nil {
		return dailyCardResult{}, err
	}

	inputJSON, err := json.Marshal(dailyCardInput{
		SchemaVersion:       dailycard.InputSchemaVersion,
		OutputSchemaVersion: dailycard.OutputSchemaVersion,
		Locale:              dailyCardLocale,
		Viewer:              dailyCardViewer{Relationship: strings.TrimSpace(relationship)},
		ReportData:          reportData,
	})
	if err != nil {
		return dailyCardResult{Window: window}, fmt.Errorf("marshal daily card input: %w", err)
	}

	hashReportData, err := canonicalAIReportHashData(reportData, window)
	if err != nil {
		return dailyCardResult{Window: window}, fmt.Errorf("canonicalize daily card cache identity: %w", err)
	}
	inputHash, err := dailyCardInputHash(relationship, hashReportData)
	if err != nil {
		return dailyCardResult{Window: window}, fmt.Errorf("hash daily card input: %w", err)
	}

	result := dailyCardResult{Window: window, InputHash: inputHash}
	cacheRangeEnd := aiReportCacheRangeEnd(window)
	cached, err := h.Store.GetAIReportCache(ctx, baby.FamilyID, baby.ID, dailyCardCacheReportType, window.RangeStart, cacheRangeEnd, inputHash)
	if err == nil && dailyCardCacheFresh(cached, now) {
		contentJSON, validationErr := validateDailyCardOutput(cached.ContentJSON)
		if validationErr != nil {
			return result, fmt.Errorf("%w: cached output: %v", errDailyCardOutputInvalid, validationErr)
		}
		cached.ContentJSON = contentJSON
		result.Cache = cached
		return result, nil
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return result, fmt.Errorf("get daily card cache: %w", err)
	}
	if h.DailyCardAI == nil {
		return result, errDailyCardGenerationUnconfigured
	}

	generated, err := h.DailyCardAI.GenerateDailyCard(ctx, inputJSON)
	if err != nil {
		return result, fmt.Errorf("%w: %v", errDailyCardGenerationFailed, err)
	}
	contentJSON, err := validateDailyCardOutput(generated.ContentJSON)
	if err != nil {
		return result, fmt.Errorf("%w: %v", errDailyCardOutputInvalid, err)
	}

	cached, err = h.Store.CreateAIReportCache(ctx, store.AIReportCache{
		FamilyID:            baby.FamilyID,
		BabyID:              baby.ID,
		ReportType:          dailyCardCacheReportType,
		RangeStart:          window.RangeStart,
		RangeEnd:            cacheRangeEnd,
		InputHash:           inputHash,
		PromptVersion:       dailycard.PromptVersion,
		InputSchemaVersion:  dailycard.InputSchemaVersion,
		OutputSchemaVersion: dailycard.OutputSchemaVersion,
		Model:               generated.Model,
		ContentJSON:         contentJSON,
	})
	if err != nil {
		return result, fmt.Errorf("cache daily card: %w", err)
	}

	result.Cache = cached
	return result, nil
}

func dailyCardInputHash(relationship string, reportData any) (string, error) {
	payload, err := json.Marshal(dailyCardHashEnvelope{
		InputSchemaVersion:  dailycard.InputSchemaVersion,
		OutputSchemaVersion: dailycard.OutputSchemaVersion,
		PromptVersion:       dailycard.PromptVersion,
		Locale:              dailyCardLocale,
		ViewerRelationship:  strings.TrimSpace(relationship),
		ReportData:          reportData,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func dailyCardCacheFresh(cached store.AIReportCache, now time.Time) bool {
	return !cached.CreatedAt.IsZero() && !cached.CreatedAt.Before(now.Add(-dailyCardCacheTTL))
}

func validateDailyCardOutput(raw json.RawMessage) (json.RawMessage, error) {
	var output dailycard.Output
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return nil, fmt.Errorf("decode output: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("output must contain one JSON object")
	}
	if output.SchemaVersion != dailycard.OutputSchemaVersion {
		return nil, fmt.Errorf("schema_version = %q, want %q", output.SchemaVersion, dailycard.OutputSchemaVersion)
	}
	if strings.TrimSpace(output.Title) == "" {
		return nil, errors.New("title is required")
	}
	if len(strings.Fields(output.Title)) > 5 {
		return nil, errors.New("title exceeds 5 words")
	}
	if strings.TrimSpace(output.Body) == "" {
		return nil, errors.New("body is required")
	}
	if strings.TrimSpace(output.Closing) == "" {
		return nil, errors.New("closing is required")
	}

	all := strings.Join([]string{output.Title, output.Body, output.Closing}, " ")
	if len(strings.Fields(all)) > 80 {
		return nil, errors.New("prose exceeds 80 words")
	}

	if strings.ContainsAny(all, "`<>#[]") || strings.Contains(all, "**") {
		return nil, errors.New("Markdown or HTML is not allowed")
	}
	normalized, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("encode normalized output: %w", err)
	}
	return normalized, nil
}
func (h *Handlers) currentViewerRelationship(ctx context.Context, familyID uuid.UUID) (string, error) {
	claims, ok := authctx.FromContext(ctx)
	if !ok || h.FamilyStore == nil {
		return "", nil
	}
	membership, err := h.FamilyStore.GetFamilyMembershipForFamily(ctx, claims.UserID, familyID)
	if err != nil {
		return "", err
	}
	if !membership.Found || membership.Status != store.MembershipStatusActive {
		return "", nil
	}
	return parentFacingRelationship(membership.Relationship), nil
}

func parentFacingRelationship(relationship string) string {
	relationship = strings.TrimSpace(relationship)
	switch strings.ToLower(relationship) {
	case "father":
		return "Dad"
	case "mother":
		return "Mum"
	case "grandmother":
		return "Grandma"
	case "grandfather":
		return "Grandpa"
	default:
		return relationship
	}
}
