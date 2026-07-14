package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestNormalizeGrowthMeasurementAttributes(t *testing.T) {
	attrs, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypeGrowthMeasurement, map[string]any{
		"weight_grams":          float64(4200),
		"length_cm":             float64(55.5),
		"head_circumference_cm": float64(38.2),
		"notes":                 "8 week check",
	})
	if !ok {
		t.Fatal("normalizeEventAttributes rejected valid growth measurement")
	}
	if got := attrs["weight_grams"]; got != 4200 {
		t.Fatalf("attrs[weight_grams] = %#v, want 4200", got)
	}
	if got := attrs["length_cm"]; got != 55.5 {
		t.Fatalf("attrs[length_cm] = %#v, want 55.5", got)
	}
	if got := attrs["head_circumference_cm"]; got != 38.2 {
		t.Fatalf("attrs[head_circumference_cm] = %#v, want 38.2", got)
	}
	if got := attrs["notes"]; got != "8 week check" {
		t.Fatalf("attrs[notes] = %#v, want 8 week check", got)
	}
}

func TestNormalizeGrowthMeasurementAttributesAllowsSingleMeasurement(t *testing.T) {
	attrs, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypeGrowthMeasurement, map[string]any{
		"weight_grams": float64(4200),
	})
	if !ok {
		t.Fatal("normalizeEventAttributes rejected single growth measurement")
	}
	if got := attrs["weight_grams"]; got != 4200 {
		t.Fatalf("attrs[weight_grams] = %#v, want 4200", got)
	}
}

func TestNormalizeGrowthMeasurementAttributesRejectsEmptyMeasurement(t *testing.T) {
	if _, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypeGrowthMeasurement, map[string]any{
		"notes": "no measurements",
	}); ok {
		t.Fatal("normalizeEventAttributes accepted growth measurement without measurements")
	}
}

func TestNormalizeGrowthMeasurementAttributesRejectsOutOfRangeValue(t *testing.T) {
	if _, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypeGrowthMeasurement, map[string]any{
		"weight_grams": float64(0),
	}); ok {
		t.Fatal("normalizeEventAttributes accepted out-of-range weight")
	}
}
