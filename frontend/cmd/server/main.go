package main

import (
	"crypto/sha256"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/andreistefanciprian/yauli/frontend/internal/authclient"
	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
	"github.com/andreistefanciprian/yauli/frontend/internal/handlers"
)

// dict lets a template pass several named fields to {{template}} in one call
// (e.g. {{template "number-stepper" (dict "Name" "duration_minutes" ...)}}),
// since Go templates only pass a single value as the invoked template's ".".
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", pairs[i])
		}
		m[key] = pairs[i+1]
	}
	return m, nil
}

// splitEventTime breaks a TimelineEvent's pre-formatted display time —
// "11:15 AM" or, for historical days, "Jan 2, 11:15 AM" — into separate
// date/clock parts so the timeline's narrow time column can stack the date
// above the clock instead of widening to fit the longer historical format.
func splitEventTime(s string) map[string]string {
	if date, clock, ok := strings.Cut(s, ", "); ok {
		return map[string]string{"Date": date, "Clock": clock}
	}
	return map[string]string{"Date": "", "Clock": s}
}

// initial returns the uppercased first rune of s, for the navbar's
// initial-letter account avatar. Empty input yields an empty string rather
// than panicking, since a signed-out or still-loading account view can pass
// through a zero-value label.
func initial(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return ""
}

// staticAssetURLs fingerprints each static file once at startup. The content
// hash in the query string gives browsers and CDNs a new URL whenever an asset
// changes, preventing fresh HTML from running stale JavaScript after deploys.
func staticAssetURLs(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read static assets: %w", err)
	}

	urls := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read static asset %q: %w", name, err)
		}
		fingerprint := sha256.Sum256(content)
		urls[name] = fmt.Sprintf("/static/%s?v=%x", name, fingerprint[:8])
	}

	return urls, nil
}

func staticAssetURL(urls map[string]string, name string) (string, error) {
	assetURL, ok := urls[name]
	if !ok {
		return "", fmt.Errorf("static asset %q not found", name)
	}
	return assetURL, nil
}

func main() {
	if err := configureLogging("frontend"); err != nil {
		log.Fatal(err)
	}

	backendURL := os.Getenv("BACKEND_API_URL")
	if backendURL == "" {
		log.Fatal("BACKEND_API_URL is required")
	}

	authServiceURL := os.Getenv("AUTH_SERVICE_URL")
	if authServiceURL == "" {
		log.Fatal("AUTH_SERVICE_URL is required")
	}

	authServiceSecret := os.Getenv("FRONTEND_AUTH_SECRET")
	if authServiceSecret == "" {
		log.Fatal("FRONTEND_AUTH_SECRET is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	secureCookies := os.Getenv("ENV") == "production"

	staticURLs, err := staticAssetURLs("static")
	if err != nil {
		log.Fatal(err)
	}

	templates, err := template.New("").Funcs(template.FuncMap{
		"assetURL":  func(name string) (string, error) { return staticAssetURL(staticURLs, name) },
		"dict":      dict,
		"splitTime": splitEventTime,
		"initial":   initial,
	}).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	h := handlers.New(backendclient.New(backendURL), authclient.New(authServiceURL, authServiceSecret), templates, secureCookies)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)

	r.Group(func(r chi.Router) {
		r.Get("/login", h.ShowLogin)
		r.Post("/login", h.RequestMagicLink)
		r.Post("/logout", h.Logout)

		staticFiles := http.StripPrefix("/static/", http.FileServer(http.Dir("static")))
		r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			staticFiles.ServeHTTP(w, r)
		}))

		r.Get("/", h.ShowIntro)

		r.Group(func(r chi.Router) {
			r.Use(h.RequireSession)

			r.Get("/app", h.Index)
			r.Get("/timeline/events", h.TimelineEvents)
			r.Get("/settings", h.ShowSettings)
			r.Post("/settings/account", h.UpdateAccountSettings)
			r.Post("/settings/baby", h.UpdateBabySettings)
			r.Post("/settings/baby/delete", h.ArchiveCurrentBaby)
			r.Post("/settings/timeline/invite", h.CreateTimelineInvite)
			r.Post("/settings/timeline/members", h.UpdateTimelineMembers)
			r.Post("/settings/timeline/members/{userID}/relationship", h.UpdateTimelineMemberRelationship)
			r.Post("/settings/timeline/members/{userID}/report-emails", h.UpdateTimelineMemberReportEmails)
			r.Post("/settings/timeline/members/{userID}/remove", h.RemoveTimelineMember)
			r.Post("/nappies", h.CreateNappy)
			r.Post("/feeds", h.CreateFeed)
			r.Post("/pumps", h.CreatePump)
			r.Post("/baths", h.CreateBath)
			r.Post("/sleeps", h.CreateSleep)
			r.Post("/observations", h.CreateObservation)
			r.Post("/temperatures", h.CreateTemperature)
			r.Post("/growth-measurements", h.CreateGrowthMeasurement)
			r.Post("/events/{id}/finish-feed", h.FinishFeedNow)
			r.Post("/events/{id}/finish-sleep", h.FinishSleepNow)
			r.Post("/events/{id}/finish-pump", h.FinishPumpNow)
			r.Patch("/events/{id}", h.UpdateEvent)
			r.Delete("/events/{id}", h.DeleteEvent)
		})

		r.Group(func(r chi.Router) {
			r.Use(h.RequireOnboardingSession)

			r.Get("/onboarding", h.ShowOnboarding)
			r.Post("/onboarding", h.CreateFirstBaby)
		})
	})

	// The request logger records paths but never query strings, so the raw
	// magic-link token on this route cannot enter access logs.
	r.Get("/auth/verify", h.ShowVerify)
	r.Post("/auth/verify", h.ConfirmVerify)

	slog.Info("server listening", "port", port, "secure_cookies", secureCookies)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
