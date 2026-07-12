package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestNormalizeTemperatureAttributes(t *testing.T) {
	attrs, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypeTemperature, map[string]any{
		"temperature_c": float64(37.2),
		"method":        string(TemperatureMethodArmpit),
		"notes":         "after bath",
	})
	if !ok {
		t.Fatal("normalizeEventAttributes rejected valid temperature")
	}
	if got := attrs["temperature_c"]; got != 37.2 {
		t.Fatalf("attrs[temperature_c] = %#v, want 37.2", got)
	}
	if got := attrs["method"]; got != string(TemperatureMethodArmpit) {
		t.Fatalf("attrs[method] = %#v, want armpit", got)
	}
}

func TestNormalizeTemperatureAttributesRejectsOutOfRangeValue(t *testing.T) {
	if _, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypeTemperature, map[string]any{
		"temperature_c": float64(29.9),
	}); ok {
		t.Fatal("normalizeEventAttributes accepted out-of-range temperature")
	}
}
