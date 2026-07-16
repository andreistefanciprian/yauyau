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
	if !strings.Contains(gotForm.Get("html"), "You&#39;re doing great. You&#39;ve got this.") {
		t.Fatalf("html body did not contain encouragement: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "parent@example.com") {
		t.Fatalf("html body did not contain recipient email: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "#74C7C3") {
		t.Fatalf("html body did not contain Yauli accent color: %q", gotForm.Get("html"))
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
	report.Output.Title = `<script>alert("x")</script>`
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
		t.Fatalf("html body did not contain escaped title: %q", html)
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
