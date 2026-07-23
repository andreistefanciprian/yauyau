package aireport

import "encoding/json"

const (
	InputSchemaVersion  = "ai_report_input.v1"
	OutputSchemaVersion = "ai_report_output.v1"
	PromptVersion       = "ai_report_prompt.v6"
)

// GenerationInput is the model-facing envelope. It deliberately contains
// deterministic report data and version identifiers, not raw auth/session
// context or frontend state.
type GenerationInput struct {
	InputSchemaVersion  string
	OutputSchemaVersion string
	PromptVersion       string
	ReportType          string
	Locale              string
	ReportData          any
}

// GenerationResult is the raw structured JSON returned by the model plus the
// model identifier used for cache metadata.
type GenerationResult struct {
	Model       string
	ContentJSON json.RawMessage
}

// Output is the first AI report response contract. The handler validates this
// before anything is cached so downstream renderers can trust the shape.
type Output struct {
	SchemaVersion      string   `json:"schema_version"`
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	Highlights         []string `json:"highlights"`
	Patterns           []string `json:"patterns"`
	Comparison         []string `json:"comparison"`
	Caveats            []string `json:"caveats"`
	QuestionsForParent []string `json:"questions_for_parent"`
}
