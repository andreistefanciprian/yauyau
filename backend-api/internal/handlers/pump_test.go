package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestNormalizePumpAttributesPreservesExplicitOngoingState(t *testing.T) {
	attributes, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypePump, map[string]any{
		"amount_ml": float64(80),
		"ongoing":   true,
	})
	if !ok {
		t.Fatal("normalizeEventAttributes rejected an ongoing pump")
	}
	if ongoing, _ := attributes["ongoing"].(bool); !ongoing {
		t.Fatalf("ongoing = %#v, want true", attributes["ongoing"])
	}
}

func TestNormalizePumpAttributesKeepsLegacyPumpCompleted(t *testing.T) {
	attributes, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypePump, map[string]any{
		"amount_ml": float64(80),
	})
	if !ok {
		t.Fatal("normalizeEventAttributes rejected a legacy pump")
	}
	if _, exists := attributes["ongoing"]; exists {
		t.Fatalf("legacy pump gained ongoing marker: %#v", attributes)
	}
}

func TestNormalizePumpAttributesRejectsOngoingPumpWithDuration(t *testing.T) {
	if _, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypePump, map[string]any{
		"amount_ml":        float64(80),
		"duration_minutes": float64(15),
		"ongoing":          true,
	}); ok {
		t.Fatal("normalizeEventAttributes accepted an ongoing pump with a duration")
	}
}
