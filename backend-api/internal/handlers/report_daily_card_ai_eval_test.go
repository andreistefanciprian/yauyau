package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDailyCardGoldenFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../../evals/daily-card/goldens/*.json")
	if err != nil {
		t.Fatalf("find daily card golden fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no daily card golden fixtures found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var fixture struct {
				Input        dailyCardInput  `json:"input"`
				GoldenOutput json.RawMessage `json:"golden_output"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			if _, err := validateDailyCardOutput(fixture.GoldenOutput); err != nil {
				t.Fatalf("validate golden output: %v", err)
			}
		})
	}
}
