package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"yauyau/backend-api/internal/handlers"
	"yauyau/backend-api/internal/store"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
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

	h := handlers.New(store.NewPostgresStore(pool))

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.Healthz)
	r.Route("/api/v1/babies/current", func(r chi.Router) {
		r.Get("/", h.GetCurrentBaby)
		r.Route("/nappies", func(r chi.Router) {
			r.Post("/", h.CreateNappy)
			r.Get("/", h.ListNappies)
		})
		r.Route("/feeds", func(r chi.Router) {
			r.Post("/", h.CreateFeed)
			r.Get("/", h.ListFeeds)
		})
		r.Route("/baths", func(r chi.Router) {
			r.Post("/", h.CreateBath)
			r.Get("/", h.ListBaths)
		})
	})

	log.Printf("backend-api listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
