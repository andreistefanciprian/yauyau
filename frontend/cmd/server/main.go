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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	secureCookies := os.Getenv("ENV") == "production"

	templates, err := template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	h := handlers.New(backendclient.New(backendURL), authclient.New(authServiceURL), templates, secureCookies)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Logger)

		r.Get("/", h.Index)
		r.Post("/nappies", h.CreateNappy)
		r.Post("/feeds", h.CreateFeed)
		r.Post("/baths", h.CreateBath)
		r.Post("/sleeps", h.CreateSleep)
		r.Post("/observations", h.CreateObservation)
		r.Delete("/events/{id}", h.DeleteEvent)

		r.Get("/login", h.ShowLogin)
		r.Post("/login", h.RequestMagicLink)
		r.Post("/auth/verify", h.ConfirmVerify)
		r.Post("/logout", h.Logout)
	})

	// GET /auth/verify carries the raw magic-link token in its query string
	// and is deliberately kept out of the r.Use(middleware.Logger) group
	// above — that logger would otherwise write the token straight into
	// access logs (see docs/auth-magic-link.md's "Token exposure in the
	// URL" hardening note).
	r.With(logRedactedVerifyRequest).Get("/auth/verify", h.ShowVerify)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Printf("frontend listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// logRedactedVerifyRequest stands in for middleware.Logger on GET
// /auth/verify only, logging the route without its query string.
func logRedactedVerifyRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("GET %s (query redacted)", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
