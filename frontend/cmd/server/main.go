package main

import (
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"

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

	templates, err := template.New("").Funcs(template.FuncMap{"dict": dict}).ParseGlob("templates/*.html")
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
			r.Get("/daily-report/ai", h.DailyReportAI)
			r.Get("/settings", h.ShowSettings)
			r.Post("/settings/account", h.UpdateAccountSettings)
			r.Post("/settings/report-emails", h.UpdateReportEmailSettings)
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
