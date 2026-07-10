package main

import (
	"context"
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

	h := handlers.New(store.NewPostgresStore(pool), backendclient.New(backendURL, internalSecret), frontendURL)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.Healthz)
	r.Route("/internal/auth", func(r chi.Router) {
		r.Post("/request", h.RequestMagicLink)
		r.Post("/verify", h.VerifyMagicLink)
	})

	log.Printf("auth-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
