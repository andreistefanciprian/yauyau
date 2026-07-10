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
