// Package dailycard defines the dedicated model contract for today's UI card.
// It is intentionally separate from aireport, which serves range reports and
// scheduled email content.
package dailycard

import "encoding/json"

const (
	InputSchemaVersion  = "daily_card_input.v1"
	OutputSchemaVersion = "daily_card_output.v2"
	PromptVersion       = "daily_card_prompt.v4"
)

// GenerationResult is the structured model response plus cache metadata.
type GenerationResult struct {
	Model       string
	ContentJSON json.RawMessage
}

// Output contains the model-owned copy. The backend and UI continue to own
// the deterministic feed and sleep metrics.
type Output struct {
	SchemaVersion string `json:"schema_version"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	Closing       string `json:"closing"`
}
