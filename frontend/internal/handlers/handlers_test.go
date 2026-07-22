package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
)

func TestFeedAmountFromFormIgnoresBreastAmount(t *testing.T) {
	amount, err := feedAmountFromForm("breast", "80")
	if err != nil {
		t.Fatalf("feedAmountFromForm returned error: %v", err)
	}
	if amount != nil {
		t.Fatalf("feedAmountFromForm breast amount = %v, want nil", *amount)
	}
}

func TestFeedAmountFromFormRequiresBottleAmount(t *testing.T) {
	for _, feedType := range []string{"formula", "expressed"} {
		t.Run(feedType, func(t *testing.T) {
			if _, err := feedAmountFromForm(feedType, ""); err == nil {
				t.Fatalf("feedAmountFromForm accepted empty %s amount", feedType)
			}
			if _, err := feedAmountFromForm(feedType, "0"); err == nil {
				t.Fatalf("feedAmountFromForm accepted zero %s amount", feedType)
			}
		})
	}
}

func TestBathUpdatePayloadUsesEditedSettings(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	tests := []struct {
		name           string
		bathType       string
		time           string
		timeBasis      string
		wantOccurredAt string
	}{
		{name: "type and clock time", bathType: "bottom_part", time: "08:15", timeBasis: "start", wantOccurredAt: "2026-07-20T08:15:00+09:30"},
		{name: "end time", bathType: "whole_body", time: "08:15", timeBasis: "end", wantOccurredAt: "2026-07-20T08:05:00+09:30"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := url.Values{
				"event_type":       {"bath"},
				"type":             {test.bathType},
				"date":             {"2026-07-20"},
				"time":             {test.time},
				"bath_time_basis":  {test.timeBasis},
				"duration_minutes": {"10"},
			}
			req := httptest.NewRequest(http.MethodPatch, "/events/bath-id", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm returned error: %v", err)
			}

			payload, err := (&Handlers{}).eventUpdatePayloadFromForm(loc, req)
			if err != nil {
				t.Fatalf("eventUpdatePayloadFromForm returned error: %v", err)
			}
			if payload["occurred_at"] != test.wantOccurredAt {
				t.Errorf("occurred_at = %q, want %q", payload["occurred_at"], test.wantOccurredAt)
			}
			attributes, ok := payload["attributes"].(map[string]any)
			if !ok {
				t.Fatalf("attributes = %#v, want map[string]any", payload["attributes"])
			}
			if attributes["type"] != test.bathType {
				t.Errorf("type = %q, want %q", attributes["type"], test.bathType)
			}
		})
	}
}

func TestPumpUpdatePayloadPreservesDuration(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	form := url.Values{
		"event_type":       {"pump"},
		"amount_ml":        {"80"},
		"duration_minutes": {"15"},
		"ongoing":          {"false"},
		"date":             {"2026-07-20"},
		"time":             {"08:15"},
	}
	req := httptest.NewRequest(http.MethodPatch, "/events/pump-id", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm returned error: %v", err)
	}

	payload, err := (&Handlers{}).eventUpdatePayloadFromForm(loc, req)
	if err != nil {
		t.Fatalf("eventUpdatePayloadFromForm returned error: %v", err)
	}
	attributes := payload["attributes"].(map[string]any)
	duration, ok := attributes["duration_minutes"].(*int)
	if !ok || *duration != 15 {
		t.Fatalf("duration_minutes = %#v, want 15", attributes["duration_minutes"])
	}
	if _, exists := attributes["ongoing"]; exists {
		t.Fatalf("completed pump gained ongoing marker: %#v", attributes)
	}
}

func TestPumpUpdatePayloadPreservesExplicitOngoingState(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	form := url.Values{
		"event_type": {"pump"},
		"amount_ml":  {"80"},
		"ongoing":    {"true"},
		"date":       {"2026-07-20"},
		"time":       {"08:15"},
	}
	req := httptest.NewRequest(http.MethodPatch, "/events/pump-id", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm returned error: %v", err)
	}

	payload, err := (&Handlers{}).eventUpdatePayloadFromForm(loc, req)
	if err != nil {
		t.Fatalf("eventUpdatePayloadFromForm returned error: %v", err)
	}
	attributes := payload["attributes"].(map[string]any)
	if ongoing, _ := attributes["ongoing"].(bool); !ongoing {
		t.Fatalf("ongoing = %#v, want true", attributes["ongoing"])
	}
	if _, exists := attributes["duration_minutes"]; exists {
		t.Fatalf("ongoing pump gained duration: %#v", attributes)
	}
}

func TestFeedTimelineEventMarksMissingDurationOngoing(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "feed",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"type":      "expressed",
			"amount_ml": float64(80),
			"labels":    []any{"burped_after"},
		},
	}

	timelineEvent := feedTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))
	if timelineEvent.StatusLabel != "Ongoing" {
		t.Fatalf("StatusLabel = %q, want Ongoing", timelineEvent.StatusLabel)
	}
	if !timelineEvent.CanFinishFeed {
		t.Fatal("CanFinishFeed = false, want true")
	}
	if timelineEvent.DurationMinutes != "" {
		t.Fatalf("DurationMinutes = %q, want empty", timelineEvent.DurationMinutes)
	}
	if timelineEvent.AmountMl != "80" {
		t.Fatalf("AmountMl = %q, want 80", timelineEvent.AmountMl)
	}
}

func TestPumpTimelineEventRequiresExplicitOngoingMarker(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)

	legacy := pumpTimelineEvent(backendclient.Event{
		EventType:  "pump",
		OccurredAt: occurredAt,
		Attributes: map[string]any{"amount_ml": float64(80)},
	}, loc, occurredAt.Add(15*time.Minute))
	if legacy.StatusLabel != "" || legacy.CanFinishPump || legacy.Ongoing {
		t.Fatalf("legacy pump marked ongoing: %#v", legacy)
	}

	ongoing := pumpTimelineEvent(backendclient.Event{
		EventType:  "pump",
		OccurredAt: occurredAt,
		Attributes: map[string]any{"amount_ml": float64(80), "ongoing": true},
	}, loc, occurredAt.Add(15*time.Minute))
	if ongoing.StatusLabel != "Ongoing" || !ongoing.CanFinishPump || !ongoing.Ongoing {
		t.Fatalf("explicit ongoing pump not marked ongoing: %#v", ongoing)
	}
}

func TestNappyTimelineEventUsesPlainPooSizeLabel(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "nappy",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"kind":     "both",
			"poo_size": "large",
		},
	}

	timelineEvent := nappyTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))
	if timelineEvent.Detail != "Large" {
		t.Fatalf("Detail = %q, want Large", timelineEvent.Detail)
	}
	if timelineEvent.PooSizeValue != "large" {
		t.Fatalf("PooSizeValue = %q, want large", timelineEvent.PooSizeValue)
	}
}

func TestNappyTimelineEventUsesKindAsLabel(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{kind: "wet", want: "Wee"},
		{kind: "both", want: "Wee Poo"},
		{kind: "poo", want: "Poo"},
	}

	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			ev := backendclient.Event{
				EventType:  "nappy",
				OccurredAt: occurredAt,
				Attributes: map[string]any{"kind": test.kind},
			}

			timelineEvent := nappyTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))
			if timelineEvent.TypeLabel != test.want {
				t.Fatalf("TypeLabel = %q, want %q", timelineEvent.TypeLabel, test.want)
			}
			if timelineEvent.Kind != "" {
				t.Fatalf("Kind = %q, want empty", timelineEvent.Kind)
			}
			if timelineEvent.KindValue != test.kind {
				t.Fatalf("KindValue = %q, want %q", timelineEvent.KindValue, test.kind)
			}
		})
	}
}

func TestSleepTimelineEventUsesSleepTypeAsLabel(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 16, 30, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "sleep",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"type":             "nap",
			"duration_minutes": float64(10),
		},
	}

	timelineEvent := sleepTimelineEvent(ev, loc, occurredAt.Add(10*time.Minute))

	if timelineEvent.TypeLabel != "Nap" {
		t.Fatalf("TypeLabel = %q, want Nap", timelineEvent.TypeLabel)
	}
	if timelineEvent.Kind != "" {
		t.Fatalf("Kind = %q, want empty", timelineEvent.Kind)
	}
	if timelineEvent.TypeValue != "nap" {
		t.Fatalf("TypeValue = %q, want nap", timelineEvent.TypeValue)
	}
}

func TestGrowthMeasurementTimelineEventPrefillsEditValues(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "growth_measurement",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"weight_grams":          float64(3135),
			"length_cm":             float64(52.4),
			"head_circumference_cm": float64(35.7),
			"notes":                 "checkup",
		},
	}

	timelineEvent := growthMeasurementTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))

	if timelineEvent.WeightKg != "3.135" {
		t.Fatalf("WeightKg = %q, want 3.135", timelineEvent.WeightKg)
	}
	if timelineEvent.LengthCM != "52.4" {
		t.Fatalf("LengthCM = %q, want 52.4", timelineEvent.LengthCM)
	}
	if timelineEvent.HeadCircumferenceCM != "35.7" {
		t.Fatalf("HeadCircumferenceCM = %q, want 35.7", timelineEvent.HeadCircumferenceCM)
	}
	if timelineEvent.Notes != "checkup" {
		t.Fatalf("Notes = %q, want checkup", timelineEvent.Notes)
	}
	if timelineEvent.Detail != "3.135 kg · Length 52.4 cm · Head 35.7 cm · checkup" {
		t.Fatalf("Detail = %q", timelineEvent.Detail)
	}
}

func TestGrowthMeasurementTimelineEventAcceptsStoredNumberTypes(t *testing.T) {
	loc := time.FixedZone("ACST", 9*60*60+30*60)
	occurredAt := time.Date(2026, 7, 14, 9, 15, 0, 0, loc)
	ev := backendclient.Event{
		EventType:  "growth_measurement",
		OccurredAt: occurredAt,
		Attributes: map[string]any{
			"weight_grams":          int64(3135),
			"length_cm":             json.Number("52.4"),
			"head_circumference_cm": json.Number("35.7"),
		},
	}

	timelineEvent := growthMeasurementTimelineEvent(ev, loc, occurredAt.Add(15*time.Minute))

	if timelineEvent.WeightKg != "3.135" || timelineEvent.LengthCM != "52.4" || timelineEvent.HeadCircumferenceCM != "35.7" {
		t.Fatalf("growth edit values = weight %q length %q head %q, want 3.135/52.4/35.7", timelineEvent.WeightKg, timelineEvent.LengthCM, timelineEvent.HeadCircumferenceCM)
	}
}

func TestShouldAutoRefreshTimelineOnlyForToday(t *testing.T) {
	now := time.Date(2026, 7, 14, 22, 15, 0, 0, time.UTC)

	if !shouldAutoRefreshTimeline("2026-07-14", now) {
		t.Fatal("today timeline should auto-refresh")
	}
	if shouldAutoRefreshTimeline("2026-07-13", now) {
		t.Fatal("past timeline should not auto-refresh")
	}
}
