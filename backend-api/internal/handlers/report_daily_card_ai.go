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
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/authctx"
	"github.com/andreistefanciprian/yauli/backend-api/internal/dailycard"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const (
	dailyCardCacheReportType = "daily_card"
	dailyCardLocale          = "en-AU"
	dailyCardCacheTTL        = 2 * time.Hour
	dailyCardCountPattern    = `(?:\d+|one|two|three|four|five|six|seven|eight|nine|ten)`
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

// CreateAIDailyCard returns the AI prose for today's card only. Historical
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
		contentJSON, validationErr := validateDailyCardOutput(cached.ContentJSON, reportData, relationship)
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
	contentJSON, err := validateDailyCardOutput(generated.ContentJSON, reportData, relationship)
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

func validateDailyCardOutput(raw json.RawMessage, reportData reportDataResponse, relationship string) (json.RawMessage, error) {
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
	if strings.TrimSpace(output.Opening) == "" {
		return nil, errors.New("opening is required")
	}
	if strings.TrimSpace(output.Observation) == "" {
		return nil, errors.New("observation is required")
	}
	if strings.TrimSpace(output.Encouragement) == "" {
		return nil, errors.New("encouragement is required")
	}
	if hasSecondaryDailyReportEvents(reportData.Totals) && strings.TrimSpace(output.Story) == "" {
		return nil, errors.New("story is required when secondary events are present")
	}

	all := strings.Join([]string{output.Opening, output.Story, output.Observation, output.Encouragement}, " ")
	if len(strings.Fields(all)) > 80 {
		return nil, errors.New("prose exceeds 80 words")
	}

	babyName := strings.TrimSpace(reportData.Baby.Name)
	if babyName != "" && countDailyCardMention(all, babyName) != 1 {
		return nil, errors.New("baby name must appear exactly once")
	}
	if babyName == "" && !strings.Contains(strings.ToLower(all), "your little one") {
		return nil, errors.New("missing baby name requires neutral wording")
	}

	relationship = strings.TrimSpace(relationship)
	if relationship != "" && countDailyCardMention(all, relationship) != 1 {
		return nil, errors.New("viewer relationship must appear exactly once")
	}
	if relationship != "" && countDailyCardMention(output.Encouragement, relationship) != 1 {
		return nil, errors.New("viewer relationship must appear in encouragement")
	}
	relationshipCheck := removeDailyCardMention(all, babyName)
	if relationship == "" && containsDailyCardWord(relationshipCheck, "dad", "father", "grandma", "grandpa", "mom", "mother", "mum") {
		return nil, errors.New("viewer relationship must not be assumed")
	}

	if strings.ContainsAny(all, "`<>#[]") || strings.Contains(all, "**") {
		return nil, errors.New("Markdown or HTML is not allowed")
	}
	generatedPunctuation := removeDailyCardMention(removeDailyCardMention(all, babyName), relationship)
	if strings.ContainsAny(generatedPunctuation, "-–—") {
		return nil, errors.New("hyphen or dash punctuation is not allowed")
	}
	if countDailyCardEmoji(output.Opening)+countDailyCardEmoji(output.Story) > 0 {
		return nil, errors.New("emoji is allowed only in observation or encouragement")
	}
	if countDailyCardEmoji(output.Observation)+countDailyCardEmoji(output.Encouragement) > 1 {
		return nil, errors.New("at most one emoji is allowed")
	}

	if err := validateDailyCardSafety(all); err != nil {
		return nil, err
	}
	if err := validateDailyCardPrimaryMetrics(all); err != nil {
		return nil, err
	}
	if err := validateDailyCardStory(output.Story, reportData.Totals); err != nil {
		return nil, err
	}
	if !containsAnyFold(all, "so far", "taking shape", "still unfolding", "coming together") {
		return nil, errors.New("partial day language is required")
	}

	normalized, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("encode normalized output: %w", err)
	}
	return normalized, nil
}

func validateDailyCardSafety(all string) error {
	lower := strings.ToLower(all)
	for _, phrase := range []string{
		"abnormal", "concerning", "dehydrat", "diagnos", "getting enough", "healthy",
		"growing fast", "growing quickly", "insufficient", "normal", "nothing to worry", "on track", "reassuring", "safe",
		"sufficient", "thriving", "too high", "too low", "treatment", "unhealthy", "unsafe", "unwell",
	} {
		if strings.Contains(lower, phrase) {
			return fmt.Errorf("medical or evaluative phrase %q is not allowed", phrase)
		}
	}
	return nil
}

func validateDailyCardPrimaryMetrics(all string) error {
	feedCount := regexp.MustCompile(`(?i)\b` + dailyCardCountPattern + `\s+(?:feeds?|feeding sessions?)\b`)
	sleepCount := regexp.MustCompile(`(?i)\b` + dailyCardCountPattern + `\s+(?:sleeps?|sleep periods?)\b`)
	feedVolume := regexp.MustCompile(`(?i)(?:\bfeeds?\b[^.!?]{0,40}\b\d+\s*ml\b|\b\d+\s*ml\b[^.!?]{0,40}\bfeeds?\b)`)
	sleepDuration := regexp.MustCompile(`(?i)(?:\bsleeps?\b[^.!?]{0,40}\b\d+\s*(?:hours?|hrs?|hr|minutes?|mins?|min)\b|\b\d+\s*(?:hours?|hrs?|hr|minutes?|mins?|min)\b[^.!?]{0,40}\bsleeps?\b)`)

	if feedCount.MatchString(all) || feedVolume.MatchString(all) {
		return errors.New("feed KPI facts must not be repeated")
	}
	if sleepCount.MatchString(all) || sleepDuration.MatchString(all) {
		return errors.New("sleep KPI facts must not be repeated")
	}
	return nil
}

func validateDailyCardStory(story string, totals reportTotalsResponse) error {
	lower := strings.ToLower(story)
	for _, nappyDetail := range []string{"mixed", "poo", "wee", "wet only", "wet-only"} {
		if totals.Nappies.Count > 0 && strings.Contains(lower, nappyDetail) {
			return fmt.Errorf("nappy detail %q is not allowed", nappyDetail)
		}
	}
	nappyCount := regexp.MustCompile(`(?i)\b(?:` + dailyCardCountPattern + `|a)\s+(?:nappy changes?|nappies)\b`)
	if nappyCount.MatchString(story) {
		return errors.New("nappy count is not allowed")
	}

	categoryChecks := []struct {
		count int
		words []string
	}{
		{totals.Nappies.Count, []string{"nappy", "nappies"}},
		{totals.Pumps.Count, []string{"pump", "pumping"}},
		{totals.Baths.Count, []string{"bath", "baths"}},
		{totals.Temperatures.Count, []string{"temperature"}},
		{totals.Growth.Count, []string{"growth", "weight", "length", "head circumference"}},
	}
	for _, check := range categoryChecks {
		if check.count == 0 && containsAnyFold(lower, check.words...) {
			return fmt.Errorf("story mentions an event category that was not recorded")
		}
	}
	if totals.Growth.Count > 0 && !strings.Contains(lower, "growth") {
		return errors.New("growth measurement must be mentioned")
	}
	if err := validateDailyCardGrowthValues(story, totals.Growth); err != nil {
		return err
	}

	pumpingCount := regexp.MustCompile(`(?i)\b(` + dailyCardCountPattern + `)\s+(?:pumping\s+sessions?|pumps?)\b`)
	for _, match := range pumpingCount.FindAllStringSubmatch(story, -1) {
		count, ok := dailyCardCountValue(match[1])
		if !ok {
			return errors.New("pumping session count is invalid")
		}
		if count != totals.Pumps.Count {
			return errors.New("pumping session count does not match report data")
		}
	}
	for _, match := range regexp.MustCompile(`(?i)(\d+)\s*ml`).FindAllStringSubmatch(story, -1) {
		amount, _ := strconv.Atoi(match[1])
		if totals.Pumps.Count == 0 || amount != totals.Pumps.TotalMl {
			return errors.New("recorded ml in story does not match pumping total")
		}
	}
	return nil
}

func validateDailyCardGrowthValues(story string, growth reportGrowthTotals) error {
	weightMatches := regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(kg|kilograms?|g|grams?)\b`).FindAllStringSubmatch(story, -1)
	if growth.LatestWeightGrams == nil && len(weightMatches) > 0 {
		return errors.New("story includes a growth weight that was not recorded")
	}
	if growth.LatestWeightGrams != nil {
		if len(weightMatches) == 0 {
			return errors.New("recorded growth weight must be mentioned")
		}
		for _, match := range weightMatches {
			value, _ := strconv.ParseFloat(match[1], 64)
			unit := strings.ToLower(match[2])
			if unit == "kg" || strings.HasPrefix(unit, "kilogram") {
				value *= 1000
			}
			if math.Abs(value-float64(*growth.LatestWeightGrams)) > 0.001 {
				return errors.New("growth weight does not match report data")
			}
		}
	}

	type expectedCentimetres struct {
		name  string
		value float64
	}
	expected := make([]expectedCentimetres, 0, 2)
	if growth.LatestLengthCM != nil {
		expected = append(expected, expectedCentimetres{name: "length", value: *growth.LatestLengthCM})
	}
	if growth.LatestHeadCircumferenceCM != nil {
		expected = append(expected, expectedCentimetres{name: "head circumference", value: *growth.LatestHeadCircumferenceCM})
	}

	centimetreMatches := regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(?:cm|centimetres?|centimeters?)\b`).FindAllStringSubmatch(story, -1)
	for _, match := range centimetreMatches {
		value, _ := strconv.ParseFloat(match[1], 64)
		matchesRecordedValue := false
		for _, measurement := range expected {
			if math.Abs(value-measurement.value) <= 0.001 {
				matchesRecordedValue = true
				break
			}
		}
		if !matchesRecordedValue {
			return errors.New("growth measurement in centimetres does not match report data")
		}
	}
	for _, measurement := range expected {
		found := false
		for _, match := range centimetreMatches {
			value, _ := strconv.ParseFloat(match[1], 64)
			if math.Abs(value-measurement.value) <= 0.001 {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("recorded growth %s must be mentioned", measurement.name)
		}
	}

	return nil
}

func dailyCardCountValue(value string) (int, bool) {
	if count, err := strconv.Atoi(value); err == nil {
		return count, true
	}
	words := map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
		"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	}
	count, ok := words[strings.ToLower(value)]
	return count, ok
}

func hasSecondaryDailyReportEvents(totals reportTotalsResponse) bool {
	return totals.Nappies.Count+totals.Pumps.Count+totals.Baths.Count+totals.Observations.Count+totals.Temperatures.Count+totals.Growth.Count > 0
}

func countDailyCardMention(value, target string) int {
	if target == "" {
		return 0
	}
	pattern := `(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(target) + `($|[^\pL\pN])`
	return len(regexp.MustCompile(pattern).FindAllStringIndex(value, -1))
}

func removeDailyCardMention(value, target string) string {
	if target == "" {
		return value
	}
	return regexp.MustCompile(`(?i)`+regexp.QuoteMeta(target)).ReplaceAllString(value, "")
}

func containsDailyCardWord(value string, words ...string) bool {
	for _, word := range words {
		if countDailyCardMention(value, word) > 0 {
			return true
		}
	}
	return false
}

func containsAnyFold(value string, targets ...string) bool {
	value = strings.ToLower(value)
	for _, target := range targets {
		if strings.Contains(value, strings.ToLower(target)) {
			return true
		}
	}
	return false
}

func countDailyCardEmoji(value string) int {
	count := 0
	for _, r := range value {
		switch {
		case r >= 0x1F000 && r <= 0x1FAFF:
			count++
		case r >= 0x2600 && r <= 0x27BF:
			count++
		case r >= 0x1F1E6 && r <= 0x1F1FF:
			count++
		case unicode.Is(unicode.So, r):
			count++
		}
	}
	return count
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
	return strings.TrimSpace(membership.Relationship), nil
}
