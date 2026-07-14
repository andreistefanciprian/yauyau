package handlers

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestSleepTypeForStartedAtInfersFromStartHour(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	tests := []struct {
		name      string
		startedAt time.Time
		want      SleepType
	}{
		{name: "early morning is night", startedAt: time.Date(2026, 7, 14, 5, 59, 0, 0, loc), want: SleepTypeNight},
		{name: "morning is day", startedAt: time.Date(2026, 7, 14, 6, 0, 0, 0, loc), want: SleepTypeNap},
		{name: "afternoon is day", startedAt: time.Date(2026, 7, 14, 17, 59, 0, 0, loc), want: SleepTypeNap},
		{name: "evening is night", startedAt: time.Date(2026, 7, 14, 18, 0, 0, 0, loc), want: SleepTypeNight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sleepTypeForStartedAt("", tt.startedAt)
			if !ok {
				t.Fatal("sleepTypeForStartedAt rejected empty type")
			}
			if got != tt.want {
				t.Fatalf("sleepTypeForStartedAt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSleepTypeForStartedAtAllowsExplicitOverride(t *testing.T) {
	startedAt := time.Date(2026, 7, 14, 21, 0, 0, 0, time.UTC)
	got, ok := sleepTypeForStartedAt("nap", startedAt)
	if !ok {
		t.Fatal("sleepTypeForStartedAt rejected explicit nap")
	}
	if got != SleepTypeNap {
		t.Fatalf("sleepTypeForStartedAt = %q, want %q", got, SleepTypeNap)
	}
}

func TestSleepTypeForStartedAtRejectsInvalidExplicitType(t *testing.T) {
	if _, ok := sleepTypeForStartedAt("overnight", time.Now()); ok {
		t.Fatal("sleepTypeForStartedAt accepted invalid type")
	}
}

func TestNormalizeSleepAttributesInfersMissingTypeFromOccurredAt(t *testing.T) {
	occurredAt := time.Date(2026, 7, 14, 20, 0, 0, 0, time.UTC)
	attrs, ok := normalizeEventAttributesForTime(httptest.NewRecorder(), eventTypeSleep, map[string]any{
		"duration_minutes": float64(90),
	}, occurredAt)
	if !ok {
		t.Fatal("normalizeEventAttributesForTime rejected sleep without type")
	}
	if attrs["type"] != string(SleepTypeNight) {
		t.Fatalf("type = %#v, want %q", attrs["type"], SleepTypeNight)
	}
}
