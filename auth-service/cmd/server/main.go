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

	h := handlers.New(store.NewPostgresStore(pool), backendclient.New(backendURL, internalSecret), frontendURL, jwtSecret)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.Healthz)
	r.Route("/internal/auth", func(r chi.Router) {
		r.Use(requireFrontendSecret(frontendAuthSecret))
		r.Post("/request", h.RequestMagicLink)
		r.Post("/verify", h.VerifyMagicLink)
		r.Post("/token", h.MintToken)
		r.Post("/logout", h.Logout)
		r.Post("/session/{id}/attach-family", h.AttachFamily)
	})

	log.Printf("auth-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// requireFrontendSecret gates the frontend-facing API behind a single
// static shared secret, set as the same env var value on both services —
// mirrors backend-api's identical requireInternalSecret. Without this,
// anyone with network reach to auth-service could mint access tokens or
// revoke sessions for any known session_id with no credential check at all.
// ConstantTimeCompare avoids leaking the secret's value one byte at a time
// through response-timing differences.
func requireFrontendSecret(secret string) func(http.Handler) http.Handler {
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
