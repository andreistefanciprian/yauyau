package main

import (
	"bytes"
	"encoding/xml"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestStaticAssetURLsChangeWithContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.js")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatalf("write first asset: %v", err)
	}

	firstURLs, err := staticAssetURLs(dir)
	if err != nil {
		t.Fatalf("fingerprint first asset: %v", err)
	}
	firstURL, err := staticAssetURL(firstURLs, "app.js")
	if err != nil {
		t.Fatalf("get first asset URL: %v", err)
	}
	if !strings.HasPrefix(firstURL, "/static/app.js?v=") {
		t.Fatalf("first asset URL = %q", firstURL)
	}

	if err := os.WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatalf("write second asset: %v", err)
	}
	secondURLs, err := staticAssetURLs(dir)
	if err != nil {
		t.Fatalf("fingerprint second asset: %v", err)
	}
	secondURL, err := staticAssetURL(secondURLs, "app.js")
	if err != nil {
		t.Fatalf("get second asset URL: %v", err)
	}
	if secondURL == firstURL {
		t.Fatalf("asset URL did not change after content changed: %q", secondURL)
	}
}

func TestStaticAssetURLRejectsUnknownAsset(t *testing.T) {
	if _, err := staticAssetURL(map[string]string{}, "missing.js"); err == nil {
		t.Fatal("staticAssetURL accepted an unknown asset")
	}
}

func TestDiscoveryFiles(t *testing.T) {
	handler := http.FileServer(http.Dir("../../static"))

	tests := []struct {
		path        string
		contentType string
		contains    []string
	}{
		{
			path:        "/robots.txt",
			contentType: "text/plain",
			contains: []string{
				"User-agent: *",
				"Allow: /",
				"Disallow: /app",
				"Sitemap: https://getyauli.com/sitemap.xml",
			},
		},
		{
			path:        "/sitemap.xml",
			contentType: "xml",
			contains: []string{
				`<loc>https://getyauli.com/</loc>`,
			},
		},
		{
			path:        "/llms.txt",
			contentType: "text/plain",
			contains: []string{
				"# Yauli",
				"> Yauli is a baby tracking and parenting companion",
				"[Yauli homepage](https://getyauli.com/)",
				"not yet generally available",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); !strings.Contains(got, test.contentType) {
				t.Fatalf("Content-Type = %q, want it to contain %q", got, test.contentType)
			}
			for _, want := range test.contains {
				if !strings.Contains(response.Body.String(), want) {
					t.Fatalf("%s does not contain %q: %s", test.path, want, response.Body.String())
				}
			}
		})
	}
}

func TestSitemapContainsOnlyCanonicalHomepage(t *testing.T) {
	content, err := os.ReadFile("../../static/sitemap.xml")
	if err != nil {
		t.Fatalf("read sitemap: %v", err)
	}

	var sitemap struct {
		URLs []struct {
			Location string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(content, &sitemap); err != nil {
		t.Fatalf("parse sitemap XML: %v", err)
	}
	if len(sitemap.URLs) != 1 {
		t.Fatalf("sitemap has %d URLs, want 1", len(sitemap.URLs))
	}
	if got := sitemap.URLs[0].Location; got != "https://getyauli.com/" {
		t.Fatalf("sitemap URL = %q, want canonical homepage", got)
	}
}

func TestTemplatesSetSearchIndexingPolicy(t *testing.T) {
	intro, err := os.ReadFile("../../templates/intro.html")
	if err != nil {
		t.Fatalf("read intro template: %v", err)
	}
	for _, want := range []string{
		`<title>Yauli | Baby Tracker &amp; Parenting Companion</title>`,
		`<meta name="description"`,
		`<meta name="robots" content="index, follow">`,
		`<link rel="canonical" href="https://getyauli.com/">`,
		`<meta property="og:title"`,
	} {
		if !strings.Contains(string(intro), want) {
			t.Fatalf("intro template does not contain %q", want)
		}
	}

	for _, name := range []string{"auth-verify.html", "index.html", "login.html", "onboarding.html", "settings.html"} {
		content, err := os.ReadFile(filepath.Join("../../templates", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(content), `<meta name="robots" content="noindex, nofollow">`) {
			t.Fatalf("%s does not opt out of search indexing", name)
		}
	}
}

func TestDailyReportRendersFourKPIs(t *testing.T) {
	templates := parseFrontendTemplates(t)
	report := backendclient.DailyReport{
		Title: "Yau Yau today",
		Card: &backendclient.DailyReportCard{
			Metrics: []backendclient.DailyReportMetric{
				{Key: "feed", Count: 3, Label: "Feeds", Detail: "530 ml · 1 hr 27 min"},
				{Key: "sleep", Count: 3, Label: "Sleep", Detail: "5 hr 57 min"},
				{Key: "pump", Count: 1, Label: "Pump", Detail: "150 ml · 1 hr"},
				{Key: "nappy", Count: 4, Label: "Nappies"},
			},
		},
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "daily-report", report); err != nil {
		t.Fatalf("render daily report: %v", err)
	}
	html := rendered.String()
	for _, want := range []string{
		`Yau Yau today`,
		`daily-report-metric-feed`,
		`daily-report-metric-count">3</strong>`,
		`daily-report-metric-label">Feeds</span>`,
		`daily-report-metric-detail">530 ml</span>`,
		`daily-report-metric-detail">1 hr 27 min</span>`,
		`daily-report-metric-sleep`,
		`daily-report-metric-detail">5 hr 57 min</span>`,
		`daily-report-metric-pump`,
		`daily-report-metric-detail">150 ml</span>`,
		`daily-report-metric-detail">1 hr</span>`,
		`daily-report-metric-nappy`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("daily report HTML does not contain %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{"hx-get=", "<p>", "<svg", "changed"} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("daily report contains %q: %s", unwanted, html)
		}
	}
	if got := strings.Count(html, `class="daily-report-metric `); got != 4 {
		t.Fatalf("daily report contains %d metrics, want 4: %s", got, html)
	}
	if got := strings.Count(html, `class="daily-report-metric-detail"`); got != 5 {
		t.Fatalf("daily report contains %d metric detail rows, want 5: %s", got, html)
	}
}

func TestIndexOmitsDailyReportToggle(t *testing.T) {
	templates := parseFrontendTemplates(t)
	data := map[string]any{
		"Baby":        backendclient.Baby{Name: "YauYau"},
		"Account":     map[string]string{"Label": "Parent", "Email": "parent@example.com"},
		"Timeline":    handlers.TimelineViewData{SelectedDate: "2026-07-18"},
		"DailyReport": (*backendclient.DailyReport)(nil),
		"NowDate":     "2026-07-18",
		"NowTime":     "09:30",
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "index", data); err != nil {
		t.Fatalf("render index: %v", err)
	}
	html := rendered.String()
	if !strings.Contains(html, `id="type-filter"`) {
		t.Fatalf("event type filter missing: %s", html)
	}
	for _, want := range []string{`data-filter-type="observation" title="Observations"`, `type-filter-chip-label">Observations</span>`} {
		if !strings.Contains(html, want) {
			t.Fatalf("observation filter does not contain %q: %s", want, html)
		}
	}
	for _, asset := range []string{"style.css", "htmx.min.js", "number-stepper.js", "app.js"} {
		if !strings.Contains(html, `/static/`+asset+`?v=test`) {
			t.Fatalf("index does not contain fingerprinted %s URL: %s", asset, html)
		}
	}
	for _, unwanted := range []string{"show-daily-report", "/timeline/preferences/daily-report", "timeline-display-filter"} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("index contains removed daily report toggle marker %q: %s", unwanted, html)
		}
	}
}

func TestTimelineSectionDoesNotPoll(t *testing.T) {
	templates := parseFrontendTemplates(t)

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "timeline-section", handlers.TimelineViewData{}); err != nil {
		t.Fatalf("render timeline section: %v", err)
	}
	html := rendered.String()
	if strings.Contains(html, "hx-trigger") || strings.Contains(html, "/timeline/events") {
		t.Fatalf("timeline section still contains polling attributes: %s", html)
	}
}

func TestAppJSUsesTimelineEventStreamWithoutPolling(t *testing.T) {
	content, err := os.ReadFile("../../static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(content)
	for _, want := range []string{
		`new EventSource("/timeline/events/stream")`,
		`timelineEvents.addEventListener("timeline_changed"`,
		`timelineEvents.addEventListener("navigate"`,
		`document.visibilityState === "hidden"`,
		`event.detail.successful === true`,
		`refreshRetryDelay`,
		`target: "#timeline-workspace"`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js does not contain %q", want)
		}
	}
	for _, unwanted := range []string{"setInterval", "FALLBACK_INTERVAL", "every 30s"} {
		if strings.Contains(js, unwanted) {
			t.Fatalf("app.js still contains polling marker %q", unwanted)
		}
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

func TestTimelineEventCardOpensEditorWithoutActionIcons(t *testing.T) {
	templates := parseFrontendTemplates(t)
	data := handlers.TimelineViewData{
		SelectedDate: "2026-07-15",
		Events: []handlers.TimelineEvent{
			{
				ID:        "event-1",
				EventType: "feed",
				CSSClass:  "feed",
				TypeLabel: "Bottle",
				Time:      "10:15 AM",
			},
		},
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "timeline", data); err != nil {
		t.Fatalf("render timeline: %v", err)
	}
	html := rendered.String()
	for _, marker := range []string{
		`class="event-card-open" role="button" tabindex="0" aria-label="Edit Bottle event at 10:15 AM"`,
		`data-event-id="event-1"`,
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("timeline event card missing %q: %s", marker, html)
		}
	}
	for _, unwanted := range []string{`class="event-edit"`, `class="event-delete"`, `hx-confirm`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("timeline event card contains removed action %q: %s", unwanted, html)
		}
	}
}

func TestIndexEditDialogHasImmediateDeleteAndDisabledSave(t *testing.T) {
	templates := parseFrontendTemplates(t)
	data := map[string]any{
		"Baby":        backendclient.Baby{Name: "YauYau"},
		"Account":     map[string]string{"Label": "Parent", "Email": "parent@example.com"},
		"Timeline":    handlers.TimelineViewData{SelectedDate: "2026-07-18"},
		"DailyReport": (*backendclient.DailyReport)(nil),
		"NowDate":     "2026-07-18",
		"NowTime":     "09:30",
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "index", data); err != nil {
		t.Fatalf("render index: %v", err)
	}
	html := rendered.String()
	for _, marker := range []string{
		`class="edit-event-actions"`,
		`id="edit-event-delete"`,
		`hx-delete="/events/__event_id__"`,
		`id="edit-event-save" disabled`,
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("edit dialog missing %q: %s", marker, html)
		}
	}
	if saveIndex, deleteIndex := strings.Index(html, `id="edit-event-save"`), strings.Index(html, `id="edit-event-delete"`); saveIndex == -1 || deleteIndex == -1 || saveIndex > deleteIndex {
		t.Fatalf("edit dialog actions are not ordered Save then Delete: %s", html)
	}
	for _, unwanted := range []string{`hx-confirm`, `id="confirm-dialog"`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("edit dialog contains removed confirmation UI %q: %s", unwanted, html)
		}
	}
}

func TestIndexGroupsEventDateAndTimeFields(t *testing.T) {
	templates := parseFrontendTemplates(t)
	data := map[string]any{
		"Baby":        backendclient.Baby{Name: "YauYau"},
		"Account":     map[string]string{"Label": "Parent", "Email": "parent@example.com"},
		"Timeline":    handlers.TimelineViewData{SelectedDate: "2026-07-18"},
		"DailyReport": (*backendclient.DailyReport)(nil),
		"NowDate":     "2026-07-18",
		"NowTime":     "09:30",
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "index", data); err != nil {
		t.Fatalf("render index: %v", err)
	}
	html := rendered.String()

	for _, eventType := range []string{"nappy", "bath", "observation", "temperature", "growth_measurement"} {
		form := createEventFormMarkup(t, html, eventType)
		if got := strings.Count(form, `class="event-occurred-at-fields"`); got != 1 {
			t.Errorf("%s create form has %d Time/Date groups, want 1", eventType, got)
		}
		if !strings.Contains(form, `type="time" name="time"`) || !strings.Contains(form, `type="date" name="date"`) {
			t.Errorf("%s create form does not contain Time and Date inputs in its shared group", eventType)
		}
	}

	for _, eventType := range []string{"feed", "pump"} {
		form := createEventFormMarkup(t, html, eventType)
		if !strings.Contains(form, `Started`) || strings.Count(form, `class="sleep-time-pair"`) != 2 {
			t.Errorf("%s create form does not group Started and Finished Date/Time fields", eventType)
		}
	}

	if got := strings.Count(html, `class="edit-occurred-at-fields"`); got != 1 {
		t.Errorf("edit dialog has %d shared Time/Date groups, want 1", got)
	}
}

// TestEventOccurredAtFieldsShowDateBeforeTime guards the date-before-time
// convention for the simple event types (nappy, bath, observation,
// temperature, growth_measurement, and the edit dialog's non-grouped case).
// The markup itself still puts the Time field first (unchanged, so nothing
// that depends on that markup order breaks) — only the *visual* order is
// flipped via CSS `order`, the same technique .grouped-edit-time-fields
// already relied on for Started/Finished. Because that reorder lives
// entirely in CSS, no template-rendering test can see it; this test instead
// reads style.css directly so removing the `order` rules fails a test
// instead of silently reintroducing the Time/Date inconsistency.
func TestEventOccurredAtFieldsShowDateBeforeTime(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "static", "style.css"))
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	css := string(data)

	firstOfType := cssRuleBody(t, css, ".event-occurred-at-fields .field-label:first-of-type")
	if !strings.Contains(firstOfType, "order: 2") {
		t.Errorf("the Time field (first in markup) should be ordered after Date (order: 2), got rule body %q", firstOfType)
	}

	lastOfType := cssRuleBody(t, css, ".event-occurred-at-fields .field-label:last-of-type")
	if !strings.Contains(lastOfType, "order: 1") {
		t.Errorf("the Date field (last in markup) should be ordered first (order: 1), got rule body %q", lastOfType)
	}

	// .edit-occurred-at-fields shares the same selector list as
	// .event-occurred-at-fields for both rules above (comma-separated), so
	// finding it at all confirms the edit dialog's non-grouped case is
	// covered by the same reorder.
	if !strings.Contains(css, ".edit-occurred-at-fields .field-label:first-of-type") {
		t.Error("edit dialog's shared Time/Date fields are not included in the date-before-time reorder")
	}
}

func TestIntroLandingHandlesNarrowAndDarkScreens(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "static", "style.css"))
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	css := string(data)

	tests := []struct {
		selector string
		want     []string
	}{
		{selector: ".intro-hero-copy", want: []string{"min-width: min(300px, 100%)"}},
		{selector: ".intro-hero-visual", want: []string{"min-width: min(280px, 100%)"}},
		{selector: ".intro-phone", want: []string{"box-sizing: border-box", "width: min(280px, 100%)"}},
		{selector: ".intro-features-copy", want: []string{"min-width: min(300px, 100%)"}},
		{selector: ".intro-features-copy > p", want: []string{"color: var(--color-text-secondary)"}},
		{selector: ".intro-growth-card", want: []string{"box-sizing: border-box", "min-width: min(300px, 100%)"}},
	}

	for _, test := range tests {
		t.Run(test.selector, func(t *testing.T) {
			body := cssRuleBody(t, css, test.selector)
			for _, want := range test.want {
				if !strings.Contains(body, want) {
					t.Fatalf("%s rule does not contain %q:\n%s", test.selector, want, body)
				}
			}
		})
	}
}

// cssRuleBody returns the `{ ... }` body of the first CSS rule whose
// selector list contains selector, ignoring how the rest of the selector
// list is formatted (e.g. a comma-separated sibling selector on its own
// line). Fails the test if selector or its rule body cannot be found.
func cssRuleBody(t *testing.T, css, selector string) string {
	t.Helper()
	start := strings.Index(css, selector)
	if start == -1 {
		t.Fatalf("style.css does not contain selector %q", selector)
	}
	openBrace := strings.Index(css[start:], "{")
	if openBrace == -1 {
		t.Fatalf("selector %q has no rule body", selector)
	}
	bodyStart := start + openBrace + 1
	closeBrace := strings.Index(css[bodyStart:], "}")
	if closeBrace == -1 {
		t.Fatalf("rule body for selector %q is not closed", selector)
	}
	return css[bodyStart : bodyStart+closeBrace]
}

func parseFrontendTemplates(t *testing.T) *template.Template {
	t.Helper()
	templates, err := template.New("").Funcs(template.FuncMap{
		"assetURL":          func(name string) string { return "/static/" + name + "?v=test" },
		"dict":              dict,
		"splitTime":         splitEventTime,
		"splitMetricDetail": splitMetricDetail,
		"initial":           initial,
	}).ParseGlob("../../templates/*.html")
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

func createEventFormMarkup(t *testing.T, html, eventType string) string {
	t.Helper()
	openingTag := `<form class="event-form" data-type="` + eventType + `"`
	start := strings.Index(html, openingTag)
	if start == -1 {
		t.Fatalf("rendered HTML does not contain %s create form", eventType)
	}
	relativeEnd := strings.Index(html[start:], "</form>")
	if relativeEnd == -1 {
		t.Fatalf("rendered %s create form is not closed", eventType)
	}
	return html[start : start+relativeEnd+len("</form>")]
}
