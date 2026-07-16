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

	"github.com/andreistefanciprian/yauli/backend-api/internal/aiclient"
	"github.com/andreistefanciprian/yauli/backend-api/internal/authclient"
	"github.com/andreistefanciprian/yauli/backend-api/internal/authctx"
	"github.com/andreistefanciprian/yauli/backend-api/internal/handlers"
	"github.com/andreistefanciprian/yauli/backend-api/internal/reportemail"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
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

	internalSecret := os.Getenv("INTERNAL_AUTH_SECRET")
	if internalSecret == "" {
		log.Fatal("INTERNAL_AUTH_SECRET is required")
	}

	jwtSecret := os.Getenv("JWT_SIGNING_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SIGNING_SECRET is required")
	}

	authServiceURL := os.Getenv("AUTH_SERVICE_URL")
	if authServiceURL == "" {
		log.Fatal("AUTH_SERVICE_URL is required")
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

	h := handlers.New(store.NewPostgresStore(pool), authclient.New(authServiceURL, frontendAuthSecret))
	if openAIAPIKey := os.Getenv("OPENAI_API_KEY"); openAIAPIKey != "" {
		h.AI = aiclient.New(openAIAPIKey, os.Getenv("OPENAI_MODEL"))
	}
	h.ReportEmailSender = configureReportEmailSender()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.Healthz)
	r.Route("/api/v1/users", func(r chi.Router) {
		r.Use(authctx.Middleware(jwtSecret))
		r.Get("/me", h.GetCurrentUser)
		r.Patch("/me", h.UpdateCurrentUser)
		r.Patch("/me/report-preferences", h.UpdateReportPreferences)
	})
	r.Route("/api/v1/babies", func(r chi.Router) {
		r.Use(authctx.Middleware(jwtSecret))
		r.Post("/", h.CreateBaby)
		r.Post("/{id}/invite", h.InviteHelper)
		r.Route("/current", func(r chi.Router) {
			r.Get("/", h.GetCurrentBaby)
			r.Patch("/", h.UpdateCurrentBaby)
			r.Delete("/", h.ArchiveCurrentBaby)
			r.Route("/members", func(r chi.Router) {
				r.Get("/", h.ListTimelineMembers)
				r.Patch("/{userID}", h.UpdateTimelineMember)
				r.Delete("/{userID}", h.RemoveTimelineMember)
			})
			r.Route("/events", func(r chi.Router) {
				r.Get("/", h.ListAllEvents)
				r.Patch("/{id}", h.UpdateEvent)
				r.Delete("/{id}", h.DeleteEvent)
			})
			r.Route("/reports", func(r chi.Router) {
				// Canonical report-data source for frontend, email, MCP tools,
				// and later AI report generation.
				r.Get("/data", h.GetReportData)
				r.Get("/daily", h.GetDailyReport)
				r.Post("/ai", h.CreateAIReport)
			})
			r.Route("/nappies", func(r chi.Router) {
				r.Post("/", h.CreateNappy)
			})
			r.Route("/feeds", func(r chi.Router) {
				r.Post("/", h.CreateFeed)
			})
			r.Route("/pumps", func(r chi.Router) {
				r.Post("/", h.CreatePump)
			})
			r.Route("/baths", func(r chi.Router) {
				r.Post("/", h.CreateBath)
			})
			r.Route("/sleeps", func(r chi.Router) {
				r.Post("/", h.CreateSleep)
			})
			r.Route("/observations", func(r chi.Router) {
				r.Post("/", h.CreateObservation)
			})
			r.Route("/temperatures", func(r chi.Router) {
				r.Post("/", h.CreateTemperature)
			})
			r.Route("/growth-measurements", func(r chi.Router) {
				r.Post("/", h.CreateGrowthMeasurement)
			})
		})
	})

	r.Route("/internal", func(r chi.Router) {
		r.Use(requireInternalSecret(internalSecret))
		r.Post("/users", h.UpsertUser)
		r.Get("/family-membership", h.GetFamilyMembership)
		r.Post("/family-membership/invite", h.CreateInvite)
	})

	log.Printf("backend-api listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func configureReportEmailSender() reportemail.Sender {
	if os.Getenv("ENV") != "production" {
		log.Print("backend-api report email sender: stdout")
		return reportemail.Stdout{}
	}

	apiKey := os.Getenv("MAILGUN_API_KEY")
	domain := os.Getenv("MAILGUN_DOMAIN")
	from := os.Getenv("MAILGUN_FROM")
	if apiKey == "" || domain == "" || from == "" {
		log.Print("backend-api report email sender: disabled (missing Mailgun configuration)")
		return reportemail.Disabled{}
	}

	log.Print("backend-api report email sender: mailgun")
	return reportemail.NewMailgun(apiKey, domain, from, os.Getenv("MAILGUN_BASE_URL"))
}

// requireInternalSecret gates the internal (auth-service-facing) API behind
// a single static shared secret, set as the same env var value on both
// services. ConstantTimeCompare avoids leaking the secret's value one byte
// at a time through response-timing differences.
func requireInternalSecret(secret string) func(http.Handler) http.Handler {
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
