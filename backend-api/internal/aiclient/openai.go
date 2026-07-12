package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const responsesURL = "https://api.openai.com/v1/responses"

var ErrDisabled = errors.New("AI client disabled")

type DailyReportInput struct {
	ReportLabel string             `json:"report_label"`
	LocalDate   string             `json:"local_date"`
	Timezone    string             `json:"timezone"`
	CurrentTime time.Time          `json:"current_time"`
	RangeStart  time.Time          `json:"range_start"`
	RangeEnd    time.Time          `json:"range_end"`
	Summary     string             `json:"summary"`
	Highlights  []string           `json:"highlights"`
	Totals      DailyReportTotals  `json:"totals"`
	Events      []DailyReportEvent `json:"events"`
}

type DailyReportTotals struct {
	Feeds        int `json:"feeds"`
	MilkMl       int `json:"milk_ml"`
	BreastFeeds  int `json:"breast_feeds"`
	WetNappies   int `json:"wet_nappies"`
	PooNappies   int `json:"poo_nappies"`
	Sleeps       int `json:"sleeps"`
	SleepMinutes int `json:"sleep_minutes"`
	Pumps        int `json:"pumps"`
	PumpMl       int `json:"pump_ml"`
	Baths        int `json:"baths"`
	Observations int `json:"observations"`
}

type DailyReportEvent struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Attributes map[string]any `json:"attributes"`
}

type DailyReportOutput struct {
	AISummary          string   `json:"ai_summary"`
	PatternNotes       []string `json:"pattern_notes"`
	SuggestedQuestions []string `json:"suggested_questions"`
}

type Client struct {
	apiKey string
	model  string
	http   *http.Client
}

func New(apiKey, model string) *Client {
	if model == "" {
		model = "gpt-4.1-mini"
	}
	return &Client{
		apiKey: strings.TrimSpace(apiKey),
		model:  model,
		http:   &http.Client{Timeout: 4 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.apiKey != ""
}

func (c *Client) GenerateDailyReport(ctx context.Context, input DailyReportInput) (DailyReportOutput, string, error) {
	if !c.Enabled() {
		return DailyReportOutput{}, "", ErrDisabled
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return DailyReportOutput{}, "", fmt.Errorf("encoding daily report input: %w", err)
	}

	reqBody := map[string]any{
		"model": c.model,
		"input": []map[string]any{
			{
				"role": "system",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": dailyReportSystemPrompt,
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": string(inputJSON),
					},
				},
			},
		},
		"max_output_tokens": 500,
		"store":             false,
		"text": map[string]any{
			"format": map[string]any{
				"type":        "json_schema",
				"name":        "daily_report_ai",
				"description": "Parent-friendly non-medical timeline insight fields.",
				"strict":      true,
				"schema":      dailyReportSchema,
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return DailyReportOutput{}, "", fmt.Errorf("encoding OpenAI request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, responsesURL, bytes.NewReader(body))
	if err != nil {
		return DailyReportOutput{}, "", fmt.Errorf("building OpenAI request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return DailyReportOutput{}, "", fmt.Errorf("calling OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return DailyReportOutput{}, "", fmt.Errorf("OpenAI returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	var parsed responsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return DailyReportOutput{}, "", fmt.Errorf("decoding OpenAI response: %w", err)
	}

	outputText := parsed.outputText()
	if outputText == "" {
		return DailyReportOutput{}, "", errors.New("OpenAI response did not include output text")
	}

	var output DailyReportOutput
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		return DailyReportOutput{}, "", fmt.Errorf("decoding daily report output: %w", err)
	}
	normalizeDailyReportOutput(&output)
	if output.AISummary == "" {
		return DailyReportOutput{}, "", errors.New("OpenAI response omitted ai_summary")
	}

	return output, c.model, nil
}

func normalizeDailyReportOutput(output *DailyReportOutput) {
	output.AISummary = strings.TrimSpace(output.AISummary)
	output.PatternNotes = trimStrings(output.PatternNotes, 3)
	output.SuggestedQuestions = trimStrings(output.SuggestedQuestions, 3)
}

func trimStrings(values []string, limit int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
		if len(out) == limit {
			return out
		}
	}
	return out
}

type responsesResponse struct {
	OutputText string          `json:"output_text"`
	Output     []responsesItem `json:"output"`
}

type responsesItem struct {
	Content []responsesContent `json:"content"`
}

type responsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (r responsesResponse) outputText() string {
	if strings.TrimSpace(r.OutputText) != "" {
		return strings.TrimSpace(r.OutputText)
	}
	for _, item := range r.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return strings.TrimSpace(content.Text)
			}
		}
	}
	return ""
}

const dailyReportSystemPrompt = `You write short baby timeline insights for tired parents.

Rules:
- Use only the provided JSON input.
- Be warm, calm, and factual.
- Do not diagnose, provide medical advice, or imply something is unsafe.
- Prefer "logged" or "recorded" phrasing because missing data may simply not have been entered.
- Mention patterns only when they are plainly supported by the events.
- Keep the summary to one sentence.
- Return only JSON matching the schema.`

var dailyReportSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"ai_summary", "pattern_notes", "suggested_questions"},
	"properties": map[string]any{
		"ai_summary": map[string]any{
			"type":        "string",
			"description": "One short, parent-friendly sentence summarizing today's rhythm so far without medical advice.",
		},
		"pattern_notes": map[string]any{
			"type":        "array",
			"description": "Zero to three concise observations that are directly supported by the logged events.",
			"maxItems":    3,
			"items": map[string]any{
				"type": "string",
			},
		},
		"suggested_questions": map[string]any{
			"type":        "array",
			"description": "Zero to three useful follow-up questions a parent may want to ask.",
			"maxItems":    3,
			"items": map[string]any{
				"type": "string",
			},
		},
	},
}
