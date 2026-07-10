package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/andreistefanciprian/yauli/auth-service/internal/backendclient"
	"github.com/andreistefanciprian/yauli/auth-service/internal/handlers"
	"github.com/andreistefanciprian/yauli/auth-service/internal/mailer"
	"github.com/andreistefanciprian/yauli/auth-service/internal/store"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	backendURL := os.Getenv("BACKEND_API_URL")
	if backendURL == "" {
		log.Fatal("BACKEND_API_URL is required")
	}

	internalSecret := os.Getenv("INTERNAL_AUTH_SECRET")
	if internalSecret == "" {
		log.Fatal("INTERNAL_AUTH_SECRET is required")
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		log.Fatal("FRONTEND_URL is required")
	}

	jwtSecret := os.Getenv("JWT_SIGNING_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SIGNING_SECRET is required")
	}

	frontendAuthSecret := os.Getenv("FRONTEND_AUTH_SECRET")
	if frontendAuthSecret == "" {
		log.Fatal("FRONTEND_AUTH_SECRET is required")
	}

	mail := configureMailer()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := store.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool, "migrations"); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	h := handlers.New(store.NewPostgresStore(pool), backendclient.New(backendURL, internalSecret), mail, frontendURL, jwtSecret)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.Healthz)
	r.Route("/internal/auth", func(r chi.Router) {
		r.Use(requireAuthSecret(frontendAuthSecret))
		r.Post("/request", h.RequestMagicLink)
		r.Post("/invite", h.RequestInviteMagicLink)
		r.Post("/verify", h.VerifyMagicLink)
		r.Post("/token", h.MintToken)
		r.Post("/logout", h.Logout)
		r.Post("/sessions/revoke-family-member", h.RevokeFamilyMemberSessions)
		r.Post("/session/{id}/attach-family", h.AttachFamily)
	})

	log.Printf("auth-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func configureMailer() mailer.Mailer {
	if os.Getenv("ENV") != "production" {
		log.Print("auth-service mailer: stdout")
		return mailer.Stdout{}
	}

	apiKey := os.Getenv("MAILGUN_API_KEY")
	if apiKey == "" {
		log.Fatal("MAILGUN_API_KEY is required in production")
	}
	domain := os.Getenv("MAILGUN_DOMAIN")
	if domain == "" {
		log.Fatal("MAILGUN_DOMAIN is required in production")
	}
	from := os.Getenv("MAILGUN_FROM")
	if from == "" {
		log.Fatal("MAILGUN_FROM is required in production")
	}

	log.Print("auth-service mailer: mailgun")
	return mailer.NewMailgun(apiKey, domain, from, os.Getenv("MAILGUN_BASE_URL"))
}

// requireAuthSecret gates auth-service's internal API behind a single
// static shared secret, set as the same env var value on trusted callers
// (frontend and backend-api). Without this, anyone with network reach to
// auth-service could mint access tokens or revoke sessions with no
// credential check at all. ConstantTimeCompare avoids leaking the secret's
// value one byte at a time through response-timing differences.
func requireAuthSecret(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			given := r.Header.Get("X-Internal-Secret")
			if subtle.ConstantTimeCompare([]byte(given), []byte(secret)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"forbidden"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
