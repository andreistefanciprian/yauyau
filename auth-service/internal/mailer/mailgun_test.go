package mailer

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMailgunSendMagicLink(t *testing.T) {
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
		w.Write([]byte(`{"id":"test","message":"Queued. Thank you."}`))
	}))
	t.Cleanup(server.Close)

	m := NewMailgun("secret-key", "mg.example.com", "Yauli <login@example.com>", server.URL)
	if err := m.SendMagicLink(context.Background(), "user@example.com", "https://app.example.com/auth/verify?token=abc"); err != nil {
		t.Fatalf("send magic link: %v", err)
	}

	if gotPath != "/v3/mg.example.com/messages" {
		t.Fatalf("path = %q, want %q", gotPath, "/v3/mg.example.com/messages")
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("api:secret-key"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotForm.Get("from") != "Yauli <login@example.com>" {
		t.Fatalf("from = %q", gotForm.Get("from"))
	}
	if gotForm.Get("to") != "user@example.com" {
		t.Fatalf("to = %q", gotForm.Get("to"))
	}
	if gotForm.Get("subject") == "" {
		t.Fatal("expected subject")
	}
	if !strings.Contains(gotForm.Get("text"), "https://app.example.com/auth/verify?token=abc") {
		t.Fatalf("text body did not contain magic link: %q", gotForm.Get("text"))
	}
	if !strings.Contains(gotForm.Get("html"), "https://app.example.com/auth/verify?token=abc") {
		t.Fatalf("html body did not contain magic link: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "Your parenting companion, from day one.") {
		t.Fatalf("html body did not contain tagline: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "#74C7C3") {
		t.Fatalf("html body did not contain Yauli teal button color: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "user@example.com") {
		t.Fatalf("html body did not contain recipient email: %q", gotForm.Get("html"))
	}
}

func TestMailgunSendInviteMagicLink(t *testing.T) {
	var gotForm url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test","message":"Queued. Thank you."}`))
	}))
	t.Cleanup(server.Close)

	m := NewMailgun("secret-key", "mg.example.com", "Yauli <login@example.com>", server.URL)
	if err := m.SendInviteMagicLink(context.Background(), "helper@example.com", "Ada", "https://app.example.com/auth/verify?token=invite"); err != nil {
		t.Fatalf("send invite magic link: %v", err)
	}

	if gotForm.Get("to") != "helper@example.com" {
		t.Fatalf("to = %q", gotForm.Get("to"))
	}
	if gotForm.Get("subject") != "You're invited to join Yauli" {
		t.Fatalf("subject = %q", gotForm.Get("subject"))
	}
	if !strings.Contains(gotForm.Get("text"), "help care for Ada") {
		t.Fatalf("text body did not contain invite copy: %q", gotForm.Get("text"))
	}
	if !strings.Contains(gotForm.Get("html"), "help care for Ada") {
		t.Fatalf("html body did not contain baby name: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "Join on Yauli") {
		t.Fatalf("html body did not contain invite CTA: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("text"), "delete that timeline") {
		t.Fatalf("text body did not contain starter baby guidance: %q", gotForm.Get("text"))
	}
	if !strings.Contains(gotForm.Get("html"), "Delete that timeline from Baby settings first") {
		t.Fatalf("html body did not contain starter baby guidance: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "https://app.example.com/auth/verify?token=invite") {
		t.Fatalf("html body did not contain magic link: %q", gotForm.Get("html"))
	}
	if !strings.Contains(gotForm.Get("html"), "helper@example.com") {
		t.Fatalf("html body did not contain recipient email: %q", gotForm.Get("html"))
	}
}

func TestMailgunSendMagicLinkReturnsErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad domain", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	m := NewMailgun("secret-key", "mg.example.com", "Yauli <login@example.com>", server.URL)
	if err := m.SendMagicLink(context.Background(), "user@example.com", "https://app.example.com/auth/verify?token=abc"); err == nil {
		t.Fatal("expected error")
	}
}
