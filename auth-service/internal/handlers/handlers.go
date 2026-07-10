package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/auth-service/internal/backendclient"
)

// Store is the persistence boundary this package needs. Defined here (the
// consumer) rather than in internal/store (the producer) so it stays a
// minimal, purpose-built contract instead of growing to match whatever the
// Postgres implementation happens to expose.
type Store interface {
	CreateMagicLink(ctx context.Context, userID uuid.UUID, tokenHash string) error
	ConsumeMagicLink(ctx context.Context, tokenHash string) (uuid.UUID, error)
	CreateSession(ctx context.Context, userID uuid.UUID, familyID *uuid.UUID) (uuid.UUID, error)
	WriteAuditLog(ctx context.Context, userID uuid.UUID, sessionID *uuid.UUID, eventType string) error
	GetValidSession(ctx context.Context, sessionID uuid.UUID) (uuid.UUID, *uuid.UUID, error)
	RevokeSession(ctx context.Context, sessionID uuid.UUID) (uuid.UUID, error)
	AttachFamily(ctx context.Context, sessionID, familyID uuid.UUID) error
}

// BackendClient is the boundary onto backend-api's internal API — the only
// way this service ever learns about users or family membership, per
// docs/auth-magic-link.md's ownership split.
type BackendClient interface {
	UpsertUser(ctx context.Context, email string) (backendclient.User, error)
	GetFamilyMembership(ctx context.Context, userID uuid.UUID, activateIfInvited bool) (backendclient.FamilyMembership, error)
}

type Handlers struct {
	Store       Store
	Backend     BackendClient
	FrontendURL string
	JWTSecret   []byte
}

// New wires up Handlers. frontendURL is used only to build the magic link
// logged to stdout in local dev (Auth->>Email in the design doc's sequence
// diagram) — the real send-a-real-email path lands in PR12. jwtSecret signs
// every access token minted by MintToken; backend-api verifies against the
// same value once JWT enforcement lands (PR10).
func New(s Store, b BackendClient, frontendURL, jwtSecret string) *Handlers {
	return &Handlers{Store: s, Backend: b, FrontendURL: frontendURL, JWTSecret: []byte(jwtSecret)}
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
