package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAIReportGoldenFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../../evals/ai-reports/goldens/*.json")
	if err != nil {
		t.Fatalf("find AI report golden fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no AI report golden fixtures found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var fixture struct {
				Input struct {
					ReportType string `json:"report_type"`
					Viewer     struct {
						Relationship string `json:"relationship"`
					} `json:"viewer"`
					ReportData reportDataResponse `json:"report_data"`
				} `json:"input"`
				GoldenOutput json.RawMessage `json:"golden_output"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			if _, err := validateAIReportOutput(
				fixture.GoldenOutput,
				fixture.Input.ReportType,
				fixture.Input.ReportData,
				fixture.Input.Viewer.Relationship,
			); err != nil {
				t.Fatalf("validate golden output: %v", err)
			}
		})
	}
}
