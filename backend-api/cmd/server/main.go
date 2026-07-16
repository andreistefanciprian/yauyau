package main

import (
	"context"
	"crypto/subtle"
	"fmt"
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

const sendDailyReportEmailsCommand = "send-daily-report-emails"

func main() {
	if len(os.Args) > 1 {
		if err := runCommand(os.Args[1:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := runHTTPServer(); err != nil {
		log.Fatal(err)
	}
}

func runCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing command")
	}
	switch args[0] {
	case sendDailyReportEmailsCommand:
		return runSendDailyReportEmailsCommand()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// runSendDailyReportEmailsCommand is intentionally one-shot: Railway or
// another scheduler can invoke it repeatedly while delivery rows prevent
// duplicate sends.
func runSendDailyReportEmailsCommand() error {
	ctx := context.Background()

	appStore, closeStore, err := connectStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	h := handlers.New(appStore, nil)
	configureAI(h)
	h.ReportEmailSender = configureReportEmailSender()

	result, err := h.SendDueDailyReportEmails(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("send due daily report emails: %w", err)
	}
	log.Printf(
		"daily report email run complete: due=%d skipped=%d sent=%d failed=%d",
		result.DueJobs,
		result.Skipped,
		result.Sent,
		result.Failed,
	)
	return nil
}

func runHTTPServer() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	internalSecret := os.Getenv("INTERNAL_AUTH_SECRET")
	if internalSecret == "" {
		return fmt.Errorf("INTERNAL_AUTH_SECRET is required")
	}

	jwtSecret := os.Getenv("JWT_SIGNING_SECRET")
	if jwtSecret == "" {
		return fmt.Errorf("JWT_SIGNING_SECRET is required")
	}

	authServiceURL := os.Getenv("AUTH_SERVICE_URL")
	if authServiceURL == "" {
		return fmt.Errorf("AUTH_SERVICE_URL is required")
	}

	frontendAuthSecret := os.Getenv("FRONTEND_AUTH_SECRET")
	if frontendAuthSecret == "" {
		return fmt.Errorf("FRONTEND_AUTH_SECRET is required")
	}

	ctx := context.Background()
	appStore, closeStore, err := connectStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	h := handlers.New(appStore, authclient.New(authServiceURL, frontendAuthSecret))
	configureAI(h)
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
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func connectStore(ctx context.Context) (*store.PostgresStore, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("DATABASE_URL is required")
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := store.Connect(connectCtx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := store.Migrate(ctx, pool, "migrations"); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("run migrations: %w", err)
	}

	return store.NewPostgresStore(pool), pool.Close, nil
}

func configureAI(h *handlers.Handlers) {
	if openAIAPIKey := os.Getenv("OPENAI_API_KEY"); openAIAPIKey != "" {
		h.AI = aiclient.New(openAIAPIKey, os.Getenv("OPENAI_MODEL"))
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
