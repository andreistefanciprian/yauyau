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

	"github.com/andreistefanciprian/yauli/backend-api/internal/aireport"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "gpt-5.1-mini"
)

type Client struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// New creates an OpenAI-backed AI report generator. The API key is optional
// at process startup; callers only install this client when generation should
// be enabled.
func New(apiKey, model string) *Client {
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultModel
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

// GenerateAIReport sends the already-calculated report-data envelope to the
// Responses API and asks for strict JSON matching ai_report_output.v1.
func (c *Client) GenerateAIReport(ctx context.Context, input aireport.GenerationInput) (aireport.GenerationResult, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return aireport.GenerationResult{}, errors.New("OpenAI API key is not configured")
	}

	inputJSON, err := json.Marshal(map[string]any{
		"schema_version":        input.InputSchemaVersion,
		"report_type":           input.ReportType,
		"audience":              "parent",
		"locale":                input.Locale,
		"report_data":           input.ReportData,
		"output_schema_version": input.OutputSchemaVersion,
	})
	if err != nil {
		return aireport.GenerationResult{}, fmt.Errorf("marshal AI report input: %w", err)
	}

	requestBody, err := json.Marshal(openAIResponsesRequest{
		Model: c.model,
		Input: []openAIInputMessage{
			{Role: "developer", Content: aiReportDeveloperPrompt(input.PromptVersion)},
			{Role: "user", Content: string(inputJSON)},
		},
		Text: openAIText{
			Format: openAITextFormat{
				Type:        "json_schema",
				Name:        "ai_report_output",
				Description: "Parent-facing AI report JSON for Yauli baby report data.",
				Strict:      true,
				Schema:      aiReportOutputSchema(),
			},
		},
		Store: false,
	})
	if err != nil {
		return aireport.GenerationResult{}, fmt.Errorf("marshal OpenAI request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/responses", bytes.NewReader(requestBody))
	if err != nil {
		return aireport.GenerationResult{}, fmt.Errorf("create OpenAI request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return aireport.GenerationResult{}, fmt.Errorf("call OpenAI responses API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return aireport.GenerationResult{}, fmt.Errorf("read OpenAI response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return aireport.GenerationResult{}, fmt.Errorf("OpenAI responses API status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed openAIResponsesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return aireport.GenerationResult{}, fmt.Errorf("decode OpenAI response: %w", err)
	}
	if parsed.Error != nil {
		return aireport.GenerationResult{}, fmt.Errorf("OpenAI response error: %s", parsed.Error.Message)
	}

	outputText := strings.TrimSpace(parsed.OutputText)
	if outputText == "" {
		outputText = strings.TrimSpace(parsed.firstOutputText())
	}
	if outputText == "" {
		return aireport.GenerationResult{}, errors.New("OpenAI response did not include output text")
	}

	return aireport.GenerationResult{
		Model:       parsed.Model,
		ContentJSON: json.RawMessage(outputText),
	}, nil
}

// openAIResponsesRequest mirrors only the Responses API fields this feature
// needs, keeping the dependency small and explicit.
type openAIResponsesRequest struct {
	Model string               `json:"model"`
	Input []openAIInputMessage `json:"input"`
	Text  openAIText           `json:"text"`
	Store bool                 `json:"store"`
}

type openAIInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIText struct {
	Format openAITextFormat `json:"format"`
}

type openAITextFormat struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Strict      bool           `json:"strict"`
	Schema      map[string]any `json:"schema"`
}

type openAIResponsesResponse struct {
	Model      string                  `json:"model"`
	OutputText string                  `json:"output_text"`
	Output     []openAIResponseOutput  `json:"output"`
	Error      *openAIResponseAPIError `json:"error"`
}

type openAIResponseAPIError struct {
	Message string `json:"message"`
}

type openAIResponseOutput struct {
	Type    string                        `json:"type"`
	Content []openAIResponseOutputContent `json:"content"`
}

type openAIResponseOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// firstOutputText supports the nested Responses API shape. Some responses
// expose output_text at the top level, while others place it in output items.
func (r openAIResponsesResponse) firstOutputText() string {
	for _, output := range r.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return content.Text
			}
		}
	}
	return ""
}

// aiReportDeveloperPrompt holds the stable product rules for model behavior.
// It intentionally tells the model to interpret backend facts, not calculate
// or diagnose.
func aiReportDeveloperPrompt(promptVersion string) string {
	return strings.Join([]string{
		"You generate Yauli baby report JSON for parents.",
		"Prompt version: " + promptVersion + ".",
		"Use only facts present in the supplied backend report_data.",
		"Do not calculate totals, averages, durations, gaps, percentages, or comparisons.",
		"Do not diagnose, provide medical advice, imply danger, or make safety claims.",
		"Do not imply missing logs mean missing care.",
		"If the range is partial, say the report is based on logs so far.",
		"Use parent-entered notes only with attribution such as \"you noted\" or \"the notes mention\".",
		"Do not mention event IDs, internal schema names, prompts, tokens, or backend mechanics.",
		"Return concise, calm, parent-facing JSON matching the supplied schema.",
	}, "\n")
}

// aiReportOutputSchema is the structured-output schema sent to OpenAI. It
// mirrors aireport.Output and the max item counts documented in the contract.
func aiReportOutputSchema() map[string]any {
	stringArray := func(maxItems int) map[string]any {
		return map[string]any{
			"type":     "array",
			"maxItems": maxItems,
			"items": map[string]any{
				"type": "string",
			},
		}
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"schema_version": map[string]any{
				"type": "string",
				"enum": []string{aireport.OutputSchemaVersion},
			},
			"title": map[string]any{
				"type": "string",
			},
			"summary": map[string]any{
				"type": "string",
			},
			"highlights":           stringArray(5),
			"patterns":             stringArray(3),
			"comparison":           stringArray(3),
			"caveats":              stringArray(2),
			"questions_for_parent": stringArray(3),
		},
		"required": []string{
			"schema_version",
			"title",
			"summary",
			"highlights",
			"patterns",
			"comparison",
			"caveats",
			"questions_for_parent",
		},
	}
}
