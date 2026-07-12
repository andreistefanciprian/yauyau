package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestNormalizeNappyAttributesTreatsNilLabelsAsEmpty(t *testing.T) {
	attrs, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypeNappy, map[string]any{
		"kind":   string(NappyKindPoo),
		"labels": nil,
	})
	if !ok {
		t.Fatal("normalizeEventAttributes rejected nil labels")
	}
	if _, exists := attrs["labels"]; exists {
		t.Fatalf("attrs[labels] exists, want omitted: %#v", attrs["labels"])
	}
}

func TestNormalizeNappyAttributesDefaultsPooSizeToMedium(t *testing.T) {
	attrs, ok := normalizeEventAttributes(httptest.NewRecorder(), eventTypeNappy, map[string]any{
		"kind": string(NappyKindBoth),
	})
	if !ok {
		t.Fatal("normalizeEventAttributes rejected missing poo_size")
	}
	if got := attrs["poo_size"]; got != string(PooSizeMedium) {
		t.Fatalf("attrs[poo_size] = %q, want %q", got, PooSizeMedium)
	}
}
