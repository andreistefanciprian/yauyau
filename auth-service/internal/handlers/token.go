package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/auth-service/internal/store"
)

// accessTokenTTL is deliberately short (see docs/auth-magic-link.md) since
// the frontend mints a fresh token on every request rather than caching one
// — a leaked token is only useful for a few minutes.
const accessTokenTTL = 10 * time.Minute

// jwtClaims mirrors the shape backend-api's authctx package decodes: sub is
// the standard registered "subject" claim (set to user_id), plus an
// optional family_id claim. FamilyID is a plain string so it's simply
// omitted (via omitempty) when nil rather than encoded as an explicit null.
type jwtClaims struct {
	FamilyID string `json:"family_id,omitempty"`
	jwt.RegisteredClaims
}

type mintTokenRequest struct {
	SessionID string `json:"session_id"`
}

type mintTokenResponse struct {
	AccessToken string     `json:"access_token"`
	FamilyID    *uuid.UUID `json:"family_id,omitempty"`
}

// MintToken exchanges a session for a short-lived signed JWT access token.
// family_id is surfaced as plain response data, not just a JWT claim, so
// the frontend never needs to decode the token itself to decide whether a
// session still needs onboarding.
func (h *Handlers) MintToken(w http.ResponseWriter, r *http.Request) {
	var req mintTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "session_id must be a valid uuid")
		return
	}

	userID, familyID, err := h.Store.GetValidSession(r.Context(), sessionID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid, revoked, or expired session")
		return
	}
	if err != nil {
		log.Printf("get valid session: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to mint token")
		return
	}

	accessToken, err := signAccessToken(userID, familyID, h.JWTSecret)
	if err != nil {
		log.Printf("sign access token: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to mint token")
		return
	}

	writeJSON(w, http.StatusOK, mintTokenResponse{AccessToken: accessToken, FamilyID: familyID})
}

// signAccessToken builds and signs the JWT MintToken hands back — pulled
// out of the handler as a pure function so token_test.go can lock down the
// exact wire shape (claim names/types) without needing a Store/BackendClient
// mock. That wire shape is a cross-service contract with backend-api's
// authctx.go decoder (backend-api/internal/authctx/authctx.go), which has
// no compiler link to this package — a rename or type change on either side
// only surfaces at runtime otherwise, so keep the two in sync deliberately.
func signAccessToken(userID uuid.UUID, familyID *uuid.UUID, secret []byte) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	if familyID != nil {
		claims.FamilyID = familyID.String()
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

type logoutRequest struct {
	SessionID string `json:"session_id"`
}

// Logout revokes a session and audit-logs it. Idempotent: revoking an
// already-revoked or nonexistent session is a no-op success, not an error —
// the client's goal ("no valid session") is already satisfied either way.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "session_id must be a valid uuid")
		return
	}

	userID, err := h.Store.RevokeSession(r.Context(), sessionID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if err != nil {
		log.Printf("revoke session: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to log out")
		return
	}

	if err := h.Store.WriteAuditLog(r.Context(), userID, &sessionID, "logout"); err != nil {
		log.Printf("write audit log: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type revokeFamilyMemberSessionsRequest struct {
	UserID   string `json:"user_id"`
	FamilyID string `json:"family_id"`
}

type revokeFamilyMemberSessionsResponse struct {
	RevokedSessions int64 `json:"revoked_sessions"`
}

// RevokeFamilyMemberSessions revokes all still-valid sessions for a
// user/family pair after backend-api has decided that member should lose
// timeline access. auth-service treats the IDs as opaque; backend-api owns
// whether the removal is allowed.
func (h *Handlers) RevokeFamilyMemberSessions(w http.ResponseWriter, r *http.Request) {
	var req revokeFamilyMemberSessionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_id must be a valid uuid")
		return
	}
	familyID, err := uuid.Parse(req.FamilyID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "family_id must be a valid uuid")
		return
	}

	count, err := h.Store.RevokeFamilyMemberSessions(r.Context(), userID, familyID)
	if err != nil {
		log.Printf("revoke family member sessions: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}

	writeJSON(w, http.StatusOK, revokeFamilyMemberSessionsResponse{RevokedSessions: count})
}

type attachFamilyRequest struct {
	FamilyID string `json:"family_id"`
}

// AttachFamily binds a null-family session to family_id, once — called
// right after onboarding's "add your baby" step returns a family_id.
func (h *Handlers) AttachFamily(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "session id must be a valid uuid")
		return
	}

	var req attachFamilyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	familyID, err := uuid.Parse(req.FamilyID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "family_id must be a valid uuid")
		return
	}

	if err := h.Store.AttachFamily(r.Context(), sessionID, familyID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, store.ErrAlreadyAttached) {
			writeError(w, http.StatusConflict, "session already has a family")
			return
		}
		log.Printf("attach family: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to attach family")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
