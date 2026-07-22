package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/andreistefanciprian/yauli/backend-api/internal/authctx"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

type currentUserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
}

type updateCurrentUserRequest struct {
	DisplayName string `json:"display_name"`
}

// GetCurrentUser returns the identity behind the current access token.
func (h *Handlers) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := authctx.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing auth context")
		return
	}

	user, err := h.FamilyStore.GetUser(r.Context(), claims.UserID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		log.Printf("get current user: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	writeJSON(w, http.StatusOK, currentUserToResponse(user))
}

func currentUserToResponse(user store.User) currentUserResponse {
	return currentUserResponse{
		ID:          user.ID.String(),
		Email:       user.Email,
		DisplayName: user.DisplayName,
	}
}

// UpdateCurrentUser stores optional account profile fields for the current user.
func (h *Handlers) UpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := authctx.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing auth context")
		return
	}

	var req updateCurrentUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if utf8.RuneCountInString(displayName) > 80 {
		writeError(w, http.StatusBadRequest, "display_name is too long")
		return
	}

	user, err := h.FamilyStore.UpdateUserDisplayName(r.Context(), claims.UserID, displayName)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		log.Printf("update current user: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	writeJSON(w, http.StatusOK, currentUserToResponse(user))
}
