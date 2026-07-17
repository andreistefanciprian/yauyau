package main

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
	"github.com/andreistefanciprian/yauli/frontend/internal/handlers"
)

func TestIconTemplatesRenderSVG(t *testing.T) {
	templates := parseFrontendTemplates(t)

	tests := []struct {
		name         string
		templateName string
		values       []string
	}{
		{
			name:         "event type",
			templateName: "event-type-icon",
			values:       []string{"nappy", "feed", "pump", "bath", "sleep", "observation", "temperature", "growth_measurement"},
		},
		{
			name:         "nappy kind",
			templateName: "nappy-kind-icon",
			values:       []string{"wet", "poo", "both"},
		},
		{
			name:         "poo size",
			templateName: "nappy-poo-size-icon",
			values:       []string{"smear", "small", "medium", "large", "blowout"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, value := range test.values {
				t.Run(value, func(t *testing.T) {
					var rendered bytes.Buffer
					if err := templates.ExecuteTemplate(&rendered, test.templateName, value); err != nil {
						t.Fatalf("render icon: %v", err)
					}
					if !strings.Contains(rendered.String(), "<svg") {
						t.Fatalf("icon = %q, want inline SVG", rendered.String())
					}
				})
			}
		})
	}
}

func TestDailyReportRendersStructuredCopyAndDeterministicMetrics(t *testing.T) {
	templates := parseFrontendTemplates(t)
	report := backendclient.DailyReport{
		Title:        "Today so far",
		SelectedDate: "2026-07-17",
		LoadAI:       true,
		Card: &backendclient.DailyReportCard{
			Intro: "Here's how YauYau's day is taking shape.",
			PrimaryMetrics: []backendclient.DailyReportPrimaryMetric{
				{Count: "4 feeds", Total: "320 ml", Qualifier: "recorded"},
				{Count: "4 sleep periods", Total: "9 hr 39 min", Qualifier: "total"},
			},
			Story:         "There were plenty of nappy changes and a growth measurement.",
			Observation:   "The day is still unfolding.",
			Encouragement: "You've got this, Dad. 💛",
		},
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "daily-report", report); err != nil {
		t.Fatalf("render daily report: %v", err)
	}
	html := rendered.String()
	for _, want := range []string{
		`hx-get="/daily-report/ai?date=2026-07-17"`,
		`<strong>4 feeds</strong>`,
		`<strong>320 ml</strong>`,
		`<strong>4 sleep periods</strong>`,
		`<strong>9 hr 39 min</strong>`,
		`a growth measurement`,
		`You&#39;ve got this, Dad. 💛`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("daily report HTML does not contain %q: %s", want, html)
		}
	}
	if strings.Contains(html, "<svg") {
		t.Fatalf("daily report contains an icon: %s", html)
	}
	if got := strings.Count(html, "<strong>"); got != 4 {
		t.Fatalf("daily report contains %d strong elements, want primary metrics only: %s", got, html)
	}
}

func TestDailyReportEscapesGeneratedCopyAndStopsReloadingAfterAI(t *testing.T) {
	templates := parseFrontendTemplates(t)
	report := backendclient.DailyReport{
		Title: "Today so far",
		Card: &backendclient.DailyReportCard{
			Intro:         `<script>alert("no")</script>`,
			Observation:   "The day is captured here.",
			Encouragement: "You've got this.",
		},
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "daily-report", report); err != nil {
		t.Fatalf("render daily report: %v", err)
	}
	html := rendered.String()
	if strings.Contains(html, "<script>") {
		t.Fatalf("daily report rendered model HTML: %s", html)
	}
	if !strings.Contains(html, `&lt;script&gt;alert`) {
		t.Fatalf("daily report did not escape model HTML: %s", html)
	}
	if strings.Contains(html, "hx-get=") {
		t.Fatalf("AI daily report would reload itself: %s", html)
	}
}

func TestHistoricalDailyReportOmitsEmptyObservationAndEncouragement(t *testing.T) {
	templates := parseFrontendTemplates(t)
	report := backendclient.DailyReport{
		Title: "Tuesday summary",
		Card: &backendclient.DailyReportCard{
			Intro: "Here's how Yau Yau's day took shape.",
			Story: "A new growth check recorded 3.5 kg, a lovely milestone to remember.",
		},
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "daily-report", report); err != nil {
		t.Fatalf("render historical daily report: %v", err)
	}
	html := rendered.String()
	if strings.Contains(html, "<p></p>") {
		t.Fatalf("historical daily report renders empty prose: %s", html)
	}
	if got := strings.Count(html, "<p>"); got != 2 {
		t.Fatalf("historical daily report contains %d paragraphs, want intro and story: %s", got, html)
	}
}

func TestNappyTimelineDetailIcons(t *testing.T) {
	templates := parseFrontendTemplates(t)

	tests := []struct {
		name     string
		kind     string
		pooSize  string
		wantSVGs int
	}{
		{name: "wet", kind: "wet", wantSVGs: 1},
		{name: "poo with size", kind: "poo", pooSize: "medium", wantSVGs: 2},
		{name: "poo without size", kind: "poo", wantSVGs: 1},
		{name: "both", kind: "both", pooSize: "large", wantSVGs: 4},
		{name: "both without size", kind: "both", wantSVGs: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var rendered bytes.Buffer
			data := map[string]string{"Kind": test.kind, "PooSize": test.pooSize}
			if err := templates.ExecuteTemplate(&rendered, "nappy-timeline-detail-icons", data); err != nil {
				t.Fatalf("render nappy timeline detail icons: %v", err)
			}
			if got := strings.Count(rendered.String(), "<svg"); got != test.wantSVGs {
				t.Fatalf("rendered %d SVGs, want %d: %s", got, test.wantSVGs, rendered.String())
			}
		})
	}
}

func TestNappyTimelineRendersSpecificLabelAndKindIconOnlyInDetailRow(t *testing.T) {
	templates := parseFrontendTemplates(t)
	data := handlers.TimelineViewData{
		SelectedDate: "2026-07-15",
		Events: []handlers.TimelineEvent{
			{
				ID:        "event-1",
				EventType: "nappy",
				CSSClass:  "nappy",
				TypeLabel: "Wee",
				KindValue: "wet",
				Time:      "10:15 AM",
			},
		},
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "timeline", data); err != nil {
		t.Fatalf("render timeline: %v", err)
	}
	html := rendered.String()
	header := elementMarkup(t, html, `<div class="event-card-header">`)
	if strings.Contains(header, "nappy-detail-icons") {
		t.Fatalf("event header contains nappy kind icons: %s", header)
	}
	if !strings.Contains(header, `<span class="event-type">Wee</span>`) {
		t.Fatalf("event header does not contain the specific nappy label: %s", header)
	}
	if strings.Contains(header, `class="event-kind"`) {
		t.Fatalf("event header contains redundant nappy kind text: %s", header)
	}

	detail := elementMarkup(t, html, `<div class="event-detail">`)
	if !strings.Contains(detail, `class="nappy-detail-icons"`) {
		t.Fatalf("event detail does not contain nappy kind icons: %s", detail)
	}
	if got := strings.Count(detail, "<svg"); got != 1 {
		t.Fatalf("event detail contains %d SVGs, want one wet icon: %s", got, detail)
	}
}

func parseFrontendTemplates(t *testing.T) *template.Template {
	t.Helper()
	templates, err := template.New("").Funcs(template.FuncMap{"dict": dict}).ParseGlob("../../templates/*.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	return templates
}

func elementMarkup(t *testing.T, html, openingTag string) string {
	t.Helper()
	start := strings.Index(html, openingTag)
	if start == -1 {
		t.Fatalf("rendered HTML does not contain %q", openingTag)
	}
	relativeEnd := strings.Index(html[start:], "</div>")
	if relativeEnd == -1 {
		t.Fatalf("rendered HTML element %q is not closed", openingTag)
	}
	return html[start : start+relativeEnd+len("</div>")]
}
