package reportemail

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aireport"
)

func TestMailgunSendReportEmail(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotForm url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"<20260716.abc@mg.example.com>","message":"Queued. Thank you."}`))
	}))
	t.Cleanup(server.Close)

	m := NewMailgun("secret-key", "mg.example.com", "Yauli <reports@example.com>", server.URL)
	messageID, err := m.SendReportEmail(context.Background(), testReport())
	if err != nil {
		t.Fatalf("send report email: %v", err)
	}

	if messageID != "<20260716.abc@mg.example.com>" {
		t.Fatalf("messageID = %q", messageID)
	}
	if gotPath != "/v3/mg.example.com/messages" {
		t.Fatalf("path = %q, want %q", gotPath, "/v3/mg.example.com/messages")
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("api:secret-key"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotForm.Get("from") != "Yauli <reports@example.com>" {
		t.Fatalf("from = %q", gotForm.Get("from"))
	}
	if gotForm.Get("to") != "parent@example.com" {
		t.Fatalf("to = %q", gotForm.Get("to"))
	}
	if gotForm.Get("subject") != "YauYau's daily report" {
		t.Fatalf("subject = %q", gotForm.Get("subject"))
	}
	if !strings.Contains(gotForm.Get("text"), "Feeds were steady") {
		t.Fatalf("text body did not contain report summary: %q", gotForm.Get("text"))
	}
	if !strings.Contains(gotForm.Get("text"), reportEncouragement) {
		t.Fatalf("text body did not contain encouragement: %q", gotForm.Get("text"))
	}
	if !strings.Contains(gotForm.Get("html"), "Feeds were steady") {
		t.Fatalf("html body did not contain report summary: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "You&#39;ve got this.") {
		t.Fatalf("html body did not contain encouragement: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "parent@example.com") {
		t.Fatalf("html body did not contain recipient email: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "Wednesday, July 15") {
		t.Fatalf("html body did not contain the date heading: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "summarised") {
		t.Fatalf("html body did not contain the subtitle: %q", gotForm.Get("html"))
	}
}

func TestMailgunSendReportEmailIncludesCard(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test-id"}`))
	}))
	t.Cleanup(server.Close)

	report := testReport()
	report.Card = []CardMetric{
		{Label: "Feeds", Count: 9, Detail: "660 ml · 1 hr 27 min"},
		{Label: "Sleep", Count: 9, Detail: "15 hr 44 min"},
		{Label: "Pump", Count: 2, Detail: "150 ml · 1 hr"},
		{Label: "Nappies", Count: 11},
	}
	m := NewMailgun("secret-key", "mg.example.com", "Yauli <reports@example.com>", server.URL)
	if _, err := m.SendReportEmail(context.Background(), report); err != nil {
		t.Fatalf("send report email: %v", err)
	}

	html := gotForm.Get("html")
	if !strings.Contains(html, ">9</p>") || !strings.Contains(html, ">Feeds<") {
		t.Fatalf("html body did not contain feeds KPI: %q", html)
	}
	if !strings.Contains(html, "660 ml</p>") || !strings.Contains(html, "1 hr 27 min</p>") {
		t.Fatalf("html body did not contain feeds detail: %q", html)
	}
	if strings.Contains(html, "660 ml · 1 hr 27 min") {
		t.Fatalf("html body did not split feeds volume and duration onto separate rows: %q", html)
	}
	if !strings.Contains(html, ">Nappies<") {
		t.Fatalf("html body did not contain nappies KPI label: %q", html)
	}
	if got := strings.Count(html, `<td width="25%" valign="top"`); got != len(report.Card) {
		t.Fatalf("top-aligned KPI columns = %d, want %d: %q", got, len(report.Card), html)
	}
}

func TestMailgunSendReportEmailIncludesUnsubscribeHeadersWhenURLSet(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test-id"}`))
	}))
	t.Cleanup(server.Close)

	report := testReport()
	report.UnsubscribeURL = "https://getyauli.com/unsubscribe?family=f&user=u&sig=s"
	m := NewMailgun("secret-key", "mg.example.com", "Yauli <reports@example.com>", server.URL)
	if _, err := m.SendReportEmail(context.Background(), report); err != nil {
		t.Fatalf("send report email: %v", err)
	}

	if got := gotForm.Get("h:List-Unsubscribe"); got != "<https://getyauli.com/unsubscribe?family=f&user=u&sig=s>" {
		t.Fatalf("h:List-Unsubscribe = %q", got)
	}
	if got := gotForm.Get("h:List-Unsubscribe-Post"); got != "List-Unsubscribe=One-Click" {
		t.Fatalf("h:List-Unsubscribe-Post = %q", got)
	}
}

func TestMailgunSendReportEmailOmitsUnsubscribeHeadersWhenURLUnset(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test-id"}`))
	}))
	t.Cleanup(server.Close)

	m := NewMailgun("secret-key", "mg.example.com", "Yauli <reports@example.com>", server.URL)
	if _, err := m.SendReportEmail(context.Background(), testReport()); err != nil {
		t.Fatalf("send report email: %v", err)
	}

	if gotForm.Has("h:List-Unsubscribe") || gotForm.Has("h:List-Unsubscribe-Post") {
		t.Fatalf("unsubscribe headers present without a Report.UnsubscribeURL: %v", gotForm)
	}
}

func TestMailgunSendReportEmailIncludesTrendInHTMLAndText(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test-id"}`))
	}))
	t.Cleanup(server.Close)

	report := testReport()
	report.Trend = []TrendDay{
		{
			Label:               "Mon",
			SleepHours:          12.5,
			FeedCount:           7,
			FeedDurationMinutes: 75,
			FeedBottleMl:        420,
			NappyCount:          8,
			PumpMl:              90,
			PumpDurationMinutes: 25,
		},
		{
			Label:               "Tue",
			SleepHours:          10,
			FeedCount:           6,
			FeedDurationMinutes: 60,
			FeedBottleMl:        360,
			NappyCount:          7,
			PumpMl:              0,
			PumpDurationMinutes: 0,
		},
	}

	m := NewMailgun("secret-key", "mg.example.com", "Yauli <reports@example.com>", server.URL)
	if _, err := m.SendReportEmail(context.Background(), report); err != nil {
		t.Fatalf("send report email: %v", err)
	}

	text := gotForm.Get("text")
	if !strings.Contains(text, "Last 7 days:") {
		t.Fatalf("text body did not contain trend heading: %q", text)
	}
	wantDay := "Mon: Sleep 12.5h · Feeds 7 (1h 15m, 420 mL bottle) · Pump 90 mL (25 min) · Nappies 8"
	if !strings.Contains(text, wantDay) {
		t.Fatalf("text body did not contain trend day %q: %q", wantDay, text)
	}
	if !strings.Contains(text, "Tue: Sleep 10.0h · Feeds 6 (1h, 360 mL bottle) · Pump 0 mL (0 min) · Nappies 7") {
		t.Fatalf("text body did not contain zero-value trend data: %q", text)
	}

	html := gotForm.Get("html")
	if !strings.Contains(html, "Last 7 days") || !strings.Contains(html, "Bottle mL") {
		t.Fatalf("html body did not contain trend charts: %q", html)
	}
}

func TestMailgunSendReportEmailEscapesHTML(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test-id"}`))
	}))
	t.Cleanup(server.Close)

	report := testReport()
	report.Output.Summary = `<script>alert("x")</script>`
	report.Output.Highlights = []string{`<b>not bold</b>`}
	m := NewMailgun("secret-key", "mg.example.com", "Yauli <reports@example.com>", server.URL)
	if _, err := m.SendReportEmail(context.Background(), report); err != nil {
		t.Fatalf("send report email: %v", err)
	}

	html := gotForm.Get("html")
	if strings.Contains(html, "<script>") || strings.Contains(html, "<b>not bold</b>") {
		t.Fatalf("html body contains unescaped user/model content: %q", html)
	}
	if !strings.Contains(html, "&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;") {
		t.Fatalf("html body did not contain escaped summary: %q", html)
	}
}

func TestMailgunSendReportEmailReturnsErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad domain", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	m := NewMailgun("secret-key", "mg.example.com", "Yauli <reports@example.com>", server.URL)
	if _, err := m.SendReportEmail(context.Background(), testReport()); err == nil {
		t.Fatal("expected error")
	}
}

func TestDisabledSenderReturnsError(t *testing.T) {
	if _, err := (Disabled{}).SendReportEmail(context.Background(), testReport()); err == nil {
		t.Fatal("expected error")
	}
}

func testReport() Report {
	return Report{
		RecipientEmail: "parent@example.com",
		BabyName:       "YauYau",
		ReportType:     "daily",
		StartDate:      "2026-07-15",
		EndDate:        "2026-07-15",
		Output: aireport.Output{
			SchemaVersion: aireport.OutputSchemaVersion,
			Title:         "YauYau's day",
			Summary:       "Feeds were steady and sleep was calm.",
			Highlights: []string{
				"Five feeds were logged.",
				"One longer sleep stood out.",
			},
			Patterns:           []string{"Nappies often followed feeds."},
			Comparison:         []string{"Sleep was close to baseline."},
			Caveats:            []string{},
			QuestionsForParent: []string{"Would you like the weekly view?"},
		},
	}
}
