package aireport

import "encoding/json"

const (
	InputSchemaVersion  = "ai_report_input.v1"
	OutputSchemaVersion = "ai_report_output.v2"
	PromptVersion       = "ai_report_prompt.v3"
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
	ViewerRelationship  string
	ReportData          any
}

// GenerationResult is the raw structured JSON returned by the model plus the
// model identifier used for cache metadata.
type GenerationResult struct {
	Model       string
	ContentJSON json.RawMessage
}

// Output is the current AI report response contract. The handler validates this
// before anything is cached so downstream renderers can trust the shape.
type Output struct {
	SchemaVersion      string    `json:"schema_version"`
	Title              string    `json:"title"`
	Summary            string    `json:"summary"`
	Highlights         []string  `json:"highlights"`
	Patterns           []string  `json:"patterns"`
	Comparison         []string  `json:"comparison"`
	Caveats            []string  `json:"caveats"`
	QuestionsForParent []string  `json:"questions_for_parent"`
	DailyCard          DailyCard `json:"daily_card"`
}

// DailyCard is the small, structured prose layer used around deterministic
// daily feed and sleep metrics. Weekly reports return empty strings here.
// Keeping these fields separate lets HTML renderers escape model text and
// apply emphasis to backend-owned facts without accepting model Markdown.
type DailyCard struct {
	Intro         string `json:"intro"`
	Story         string `json:"story"`
	Observation   string `json:"observation"`
	Encouragement string `json:"encouragement"`
}
