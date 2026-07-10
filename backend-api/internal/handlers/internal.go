package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

// This file holds the internal, auth-service-facing API: upserting users by
// email and reading/writing family membership. It is never called by an
// end user's browser — only by other services, over the network path gated
// by the X-Internal-Secret middleware mounted in cmd/server/main.go.

// parseUUIDField parses raw as a uuid, writing a 400 naming field and
// returning ok=false if it isn't one — shared by every internal-API handler
// that takes a uuid path/query/body parameter.
func parseUUIDField(w http.ResponseWriter, field, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, field+" must be a valid uuid")
		return uuid.UUID{}, false
	}
	return id, true
}

type upsertUserRequest struct {
	Email string `json:"email"`
}

// UpsertUser resolves (creating if necessary) the user for an email —
// called by auth-service right before it issues a magic link, since the
// user must exist before a magic_links row can reference it.
func (h *Handlers) UpsertUser(w http.ResponseWriter, r *http.Request) {
	var req upsertUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	user, err := h.FamilyStore.UpsertUserByEmail(r.Context(), req.Email)
	if err != nil {
		log.Printf("upsert user: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to upsert user")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// GetFamilyMembership returns the caller's family membership, if any. When
// activate_if_invited=true and the membership is a pending invite, it is
// activated first — this lets auth-service's verify flow resolve-and-activate
// an invited user's membership in a single call.
func (h *Handlers) GetFamilyMembership(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUUIDField(w, "user_id", r.URL.Query().Get("user_id"))
	if !ok {
		return
	}
	activateIfInvited := r.URL.Query().Get("activate_if_invited") == "true"

	membership, err := h.FamilyStore.GetFamilyMembership(r.Context(), userID)
	if err != nil {
		log.Printf("get family membership: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load family membership")
		return
	}

	if activateIfInvited && membership.Found && membership.Status == store.MembershipStatusInvited {
		if err := h.FamilyStore.ActivateInvitedMembership(r.Context(), userID, *membership.FamilyID); err != nil && !errors.Is(err, store.ErrNotFound) {
			log.Printf("activate invited membership: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to activate family membership")
			return
		}
		// Re-fetch rather than assume what changed: a concurrent/duplicate
		// call (e.g. a double-fired verify request) may have already
		// activated this same row, in which case ActivateInvitedMembership
		// returns ErrNotFound even though the desired state already holds -
		// re-fetching gives the authoritative current state either way.
		membership, err = h.FamilyStore.GetFamilyMembership(r.Context(), userID)
		if err != nil {
			log.Printf("get family membership after activation: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to load family membership")
			return
		}
	}

	writeJSON(w, http.StatusOK, membership)
}

type createInviteRequest struct {
	FamilyID string `json:"family_id"`
	Email    string `json:"email"`
}

// CreateInvite resolves (creating if necessary) the invitee's user record
// and grants them a pending membership in family_id, atomically (see
// store.CreateInvite) — safe to call again for the same email/family_id
// (e.g. a retry), which is a no-op rather than an error.
func (h *Handlers) CreateInvite(w http.ResponseWriter, r *http.Request) {
	var req createInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	familyID, ok := parseUUIDField(w, "family_id", req.FamilyID)
	if !ok {
		return
	}

	if err := h.FamilyStore.CreateInvite(r.Context(), familyID, req.Email); err != nil {
		log.Printf("create invite: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create invite")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "invited"})
}
