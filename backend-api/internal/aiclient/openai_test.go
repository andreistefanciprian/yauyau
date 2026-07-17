package aiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aireport"
)

func TestGenerateAIReportUsesResponsesStructuredOutput(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %s, want /responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q, want bearer token", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"model": "test-model",
			"output_text": "{\"schema_version\":\"ai_report_output.v2\",\"title\":\"Generated\",\"summary\":\"Generated summary.\",\"highlights\":[],\"patterns\":[],\"comparison\":[],\"caveats\":[],\"questions_for_parent\":[],\"daily_card\":{\"intro\":\"Generated intro.\",\"story\":\"\",\"observation\":\"Generated observation.\",\"encouragement\":\"Generated encouragement.\"}}"
		}`))
	}))
	t.Cleanup(server.Close)

	client := New("test-key", "test-model")
	client.baseURL = server.URL
	client.httpClient = server.Client()

	result, err := client.GenerateAIReport(t.Context(), aireport.GenerationInput{
		InputSchemaVersion:  aireport.InputSchemaVersion,
		OutputSchemaVersion: aireport.OutputSchemaVersion,
		PromptVersion:       aireport.PromptVersion,
		ReportType:          "daily",
		Locale:              "en",
		ViewerRelationship:  "Dad",
		ReportData:          map[string]any{"range": map[string]any{"start_date": "2026-07-13"}},
	})
	if err != nil {
		t.Fatalf("GenerateAIReport returned error: %v", err)
	}
	if result.Model != "test-model" {
		t.Fatalf("Model = %q, want test-model", result.Model)
	}
	if !strings.Contains(string(result.ContentJSON), `"title":"Generated"`) {
		t.Fatalf("ContentJSON = %s, want generated report JSON", result.ContentJSON)
	}

	if captured["model"] != "test-model" {
		t.Fatalf("request model = %#v, want test-model", captured["model"])
	}
	text, ok := captured["text"].(map[string]any)
	if !ok {
		t.Fatalf("request text = %#v, want object", captured["text"])
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		t.Fatalf("request text.format = %#v, want object", text["format"])
	}
	if format["type"] != "json_schema" || format["strict"] != true {
		t.Fatalf("request text.format = %#v, want strict json_schema", format)
	}
	schema := format["schema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	highlights := properties["highlights"].(map[string]any)
	if highlights["maxItems"] != float64(4) {
		t.Fatalf("highlights maxItems = %#v, want 4", highlights["maxItems"])
	}
	if captured["store"] != false {
		t.Fatalf("request store = %#v, want false", captured["store"])
	}
	input := captured["input"].([]any)
	developerMessage := input[0].(map[string]any)
	if developerMessage["role"] != "developer" {
		t.Fatalf("developer message role = %#v, want developer", developerMessage["role"])
	}
	developerContent := developerMessage["content"].(string)
	if !strings.Contains(developerContent, "Prompt version: ai_report_prompt.v3.") {
		t.Fatalf("developer prompt = %q, want prompt version", developerContent)
	}
	if !strings.Contains(developerContent, "Do not diagnose") {
		t.Fatalf("developer prompt = %q, want embedded product rules", developerContent)
	}
	if !strings.Contains(developerContent, "Do not use hyphens, en dashes, or em dashes") {
		t.Fatalf("developer prompt = %q, want punctuation rule", developerContent)
	}
	if !strings.Contains(developerContent, "at most one emoji") {
		t.Fatalf("developer prompt = %q, want emoji rule", developerContent)
	}
	userMessage := input[1].(map[string]any)
	var modelInput map[string]any
	if err := json.Unmarshal([]byte(userMessage["content"].(string)), &modelInput); err != nil {
		t.Fatalf("decode user message content: %v", err)
	}
	if _, ok := modelInput["delivery"]; ok {
		t.Fatalf("model input should not include delivery: %#v", modelInput)
	}
	viewer := modelInput["viewer"].(map[string]any)
	if viewer["relationship"] != "Dad" {
		t.Fatalf("viewer relationship = %#v, want Dad", viewer["relationship"])
	}
}

func TestGenerateAIReportFallsBackToOutputContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"model": "test-model",
			"output": [{
				"type": "message",
				"content": [{
					"type": "output_text",
					"text": "{\"schema_version\":\"ai_report_output.v2\",\"title\":\"Nested\",\"summary\":\"Nested summary.\",\"highlights\":[],\"patterns\":[],\"comparison\":[],\"caveats\":[],\"questions_for_parent\":[],\"daily_card\":{\"intro\":\"Nested intro.\",\"story\":\"\",\"observation\":\"Nested observation.\",\"encouragement\":\"Nested encouragement.\"}}"
				}]
			}]
		}`))
	}))
	t.Cleanup(server.Close)

	client := New("test-key", "test-model")
	client.baseURL = server.URL
	client.httpClient = server.Client()

	result, err := client.GenerateAIReport(t.Context(), aireport.GenerationInput{
		InputSchemaVersion:  aireport.InputSchemaVersion,
		OutputSchemaVersion: aireport.OutputSchemaVersion,
		PromptVersion:       aireport.PromptVersion,
		ReportType:          "daily",
		Locale:              "en",
		ReportData:          map[string]any{},
	})
	if err != nil {
		t.Fatalf("GenerateAIReport returned error: %v", err)
	}
	if !strings.Contains(string(result.ContentJSON), `"title":"Nested"`) {
		t.Fatalf("ContentJSON = %s, want nested report JSON", result.ContentJSON)
	}
}
