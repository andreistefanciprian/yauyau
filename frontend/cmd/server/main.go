package main

import (
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/andreistefanciprian/yauli/frontend/internal/authclient"
	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
	"github.com/andreistefanciprian/yauli/frontend/internal/handlers"
)

func main() {
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

	templates, err := template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	h := handlers.New(backendclient.New(backendURL), authclient.New(authServiceURL, authServiceSecret), templates, secureCookies)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Logger)

		r.Get("/login", h.ShowLogin)
		r.Post("/login", h.RequestMagicLink)
		r.Post("/logout", h.Logout)

		r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

		r.Get("/", h.ShowIntro)

		r.Group(func(r chi.Router) {
			r.Use(h.RequireSession)

			r.Get("/app", h.Index)
			r.Get("/settings/baby", h.ShowBabySettings)
			r.Post("/settings/baby", h.UpdateBabySettings)
			r.Post("/settings/baby/delete", h.ArchiveCurrentBaby)
			r.Get("/settings/timeline", h.ShowTimelineSettings)
			r.Post("/settings/timeline/invite", h.CreateTimelineInvite)
			r.Post("/settings/timeline/members/{userID}/relationship", h.UpdateTimelineMemberRelationship)
			r.Post("/settings/timeline/members/{userID}/remove", h.RemoveTimelineMember)
			r.Post("/nappies", h.CreateNappy)
			r.Post("/feeds", h.CreateFeed)
			r.Post("/baths", h.CreateBath)
			r.Post("/sleeps", h.CreateSleep)
			r.Post("/observations", h.CreateObservation)
			r.Patch("/events/{id}", h.UpdateEvent)
			r.Delete("/events/{id}", h.DeleteEvent)
		})

		r.Group(func(r chi.Router) {
			r.Use(h.RequireOnboardingSession)

			r.Get("/onboarding", h.ShowOnboarding)
			r.Post("/onboarding", h.CreateFirstBaby)
		})
	})

	// /auth/verify (both GET, which carries the raw magic-link token in its
	// query string, and POST, which a malformed or malicious client could
	// still send with the token in the query string even though
	// ConfirmVerify reads it from the body via PostFormValue) is
	// deliberately kept out of the r.Use(middleware.Logger) group above —
	// that logger writes the full request URI including any query string,
	// which would otherwise put the token straight into access logs
	// regardless of what the handler itself reads (see
	// docs/auth-magic-link.md's "Token exposure in the URL" hardening
	// note).
	r.With(logRedactedVerifyRequest).Get("/auth/verify", h.ShowVerify)
	r.With(logRedactedVerifyRequest).Post("/auth/verify", h.ConfirmVerify)

	log.Printf("frontend listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// logRedactedVerifyRequest stands in for middleware.Logger on /auth/verify
// only, logging the method and path without any query string.
func logRedactedVerifyRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s (query redacted)", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
