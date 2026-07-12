package aiclient

import "testing"

func TestResponsesResponseOutputText(t *testing.T) {
	topLevel := responsesResponse{OutputText: " {\"ai_summary\":\"top\"} "}
	if got := topLevel.outputText(); got != "{\"ai_summary\":\"top\"}" {
		t.Fatalf("top-level outputText = %q", got)
	}

	nested := responsesResponse{
		Output: []responsesItem{
			{
				Content: []responsesContent{
					{Type: "output_text", Text: " {\"ai_summary\":\"nested\"} "},
				},
			},
		},
	}
	if got := nested.outputText(); got != "{\"ai_summary\":\"nested\"}" {
		t.Fatalf("nested outputText = %q", got)
	}
}

func TestNormalizeDailyReportOutput(t *testing.T) {
	output := DailyReportOutput{
		AISummary:          " A gentle day so far. ",
		PatternNotes:       []string{" First ", "", "Second", "Third", "Fourth"},
		SuggestedQuestions: []string{" What changed today? ", ""},
	}

	normalizeDailyReportOutput(&output)

	if output.AISummary != "A gentle day so far." {
		t.Fatalf("AISummary = %q", output.AISummary)
	}
	if len(output.PatternNotes) != 3 {
		t.Fatalf("len(PatternNotes) = %d, want 3: %#v", len(output.PatternNotes), output.PatternNotes)
	}
	if output.PatternNotes[0] != "First" || output.PatternNotes[2] != "Third" {
		t.Fatalf("PatternNotes = %#v", output.PatternNotes)
	}
	if len(output.SuggestedQuestions) != 1 || output.SuggestedQuestions[0] != "What changed today?" {
		t.Fatalf("SuggestedQuestions = %#v", output.SuggestedQuestions)
	}
}
