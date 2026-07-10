package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/auth-service/internal/store"
)

// generateRawToken returns a URL-safe random token — the value that ends up
// in the emailed link. Only its hash is ever stored (see hashToken), so a
// DB dump can't be replayed as a live link.
func generateRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken returns the SHA-256 hex digest of a raw token, used both when
// storing a new magic link and when looking one up by the token a caller
// presents.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type requestMagicLinkRequest struct {
	Email string `json:"email"`
}

// RequestMagicLink upserts the user via backend-api, issues a magic link,
// and (in local dev) logs it to stdout instead of emailing it — see PR14
// for the real Mailgun send. The response is identical whether or not the
// email already had an account, so this endpoint never reveals which
// emails are registered.
func (h *Handlers) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	var req requestMagicLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	user, err := h.Backend.UpsertUser(r.Context(), req.Email)
	if err != nil {
		log.Printf("upsert user: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to send magic link")
		return
	}

	rawToken, err := generateRawToken()
	if err != nil {
		log.Printf("generate token: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to send magic link")
		return
	}

	if err := h.Store.CreateMagicLink(r.Context(), user.ID, hashToken(rawToken)); err != nil {
		log.Printf("create magic link: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to send magic link")
		return
	}

	log.Printf("magic link for %s: %s/auth/verify?token=%s", req.Email, h.FrontendURL, rawToken)

	writeJSON(w, http.StatusOK, map[string]string{"message": "magic link sent"})
}

type verifyMagicLinkRequest struct {
	Token string `json:"token"`
}

type verifyMagicLinkResponse struct {
	SessionID uuid.UUID  `json:"session_id"`
	UserID    uuid.UUID  `json:"user_id"`
	FamilyID  *uuid.UUID `json:"family_id,omitempty"`
}

// VerifyMagicLink consumes a magic link's token exactly once, then resolves
// the user's family membership (activating a pending invite if one exists)
// and opens a session — with family_id already attached if the user was
// invited, or null for a brand-new signup that still needs onboarding.
func (h *Handlers) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	var req verifyMagicLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	userID, err := h.Store.ConsumeMagicLink(r.Context(), hashToken(req.Token))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	if err != nil {
		log.Printf("consume magic link: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to verify token")
		return
	}

	membership, err := h.Backend.GetFamilyMembership(r.Context(), userID, true)
	if err != nil {
		log.Printf("get family membership: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to verify token")
		return
	}

	var familyID *uuid.UUID
	if membership.Found {
		familyID = membership.FamilyID
	}

	sessionID, err := h.Store.CreateSession(r.Context(), userID, familyID)
	if err != nil {
		log.Printf("create session: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to verify token")
		return
	}

	// Audit logging failure doesn't invalidate an otherwise-successful
	// login — the session already exists and is real; only the log entry
	// is lost, so this is reported but not treated as a request failure.
	if err := h.Store.WriteAuditLog(r.Context(), userID, &sessionID, "login"); err != nil {
		log.Printf("write audit log: %v", err)
	}

	writeJSON(w, http.StatusOK, verifyMagicLinkResponse{
		SessionID: sessionID,
		UserID:    userID,
		FamilyID:  familyID,
	})
}
