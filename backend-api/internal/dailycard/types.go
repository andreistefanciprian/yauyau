// Package dailycard defines the dedicated model contract for today's UI card.
// It is intentionally separate from aireport, which serves range reports and
// scheduled email content.
package dailycard

import "encoding/json"

const (
	InputSchemaVersion  = "daily_card_input.v1"
	OutputSchemaVersion = "daily_card_output.v1"
	PromptVersion       = "daily_card_prompt.v2"
)

// GenerationResult is the structured model response plus cache metadata.
type GenerationResult struct {
	Model       string
	ContentJSON json.RawMessage
}

// Output contains only prose. The backend and UI continue to own the title
// and deterministic feed and sleep metrics.
type Output struct {
	SchemaVersion string `json:"schema_version"`
	Opening       string `json:"opening"`
	Story         string `json:"story"`
	Observation   string `json:"observation"`
	Encouragement string `json:"encouragement"`
}
