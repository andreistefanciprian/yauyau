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
	// CanManageDailyReportEmail tells the frontend whether to show the
	// checkbox at all. The preference is currently owner-only, so non-owners
	// get false even if a future DB value exists for them.
	CanManageDailyReportEmail bool `json:"can_manage_daily_report_email"`
	DailyReportEmailEnabled   bool `json:"daily_report_email_enabled"`
}

type updateCurrentUserRequest struct {
	DisplayName string `json:"display_name"`
}

type updateReportPreferencesRequest struct {
	DailyReportEmailEnabled bool `json:"daily_report_email_enabled"`
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

	membership := store.FamilyMembership{Found: false}
	if claims.FamilyID != nil {
		membership, err = h.FamilyStore.GetFamilyMembershipForFamily(r.Context(), claims.UserID, *claims.FamilyID)
		if err != nil {
			log.Printf("get current user membership: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to load membership")
			return
		}
	}

	writeJSON(w, http.StatusOK, currentUserToResponse(user, membership))
}

// currentUserToResponse combines account identity with membership-scoped
// settings. Daily report email is intentionally derived through membership
// because delivery eligibility depends on the user's role in this family, not
// on the global user account.
func currentUserToResponse(user store.User, membership store.FamilyMembership) currentUserResponse {
	canManageDailyReportEmail := membership.Found &&
		membership.Role == store.MembershipRoleOwner &&
		membership.Status == store.MembershipStatusActive
	return currentUserResponse{
		ID:                        user.ID.String(),
		Email:                     user.Email,
		DisplayName:               user.DisplayName,
		CanManageDailyReportEmail: canManageDailyReportEmail,
		DailyReportEmailEnabled:   canManageDailyReportEmail && membership.DailyReportEmailEnabled,
	}
}

// UpdateReportPreferences stores owner-only report delivery preferences for
// the current user's active family membership. Scheduled delivery itself is a
// later feature; this endpoint only records whether this owner should receive
// the daily email once the scheduler exists.
func (h *Handlers) UpdateReportPreferences(w http.ResponseWriter, r *http.Request) {
	claims, ok := authctx.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing auth context")
		return
	}
	if claims.FamilyID == nil {
		writeError(w, http.StatusForbidden, "only the owner can update report preferences")
		return
	}

	var req updateReportPreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	membership, err := h.FamilyStore.UpdateDailyReportEmailPreference(r.Context(), *claims.FamilyID, claims.UserID, req.DailyReportEmailEnabled)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusForbidden, "only the owner can update report preferences")
		return
	}
	if err != nil {
		log.Printf("update report preferences: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update report preferences")
		return
	}

	user, err := h.FamilyStore.GetUser(r.Context(), claims.UserID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		log.Printf("get current user after report preferences update: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	writeJSON(w, http.StatusOK, currentUserToResponse(user, membership))
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

	membership := store.FamilyMembership{Found: false}
	if claims.FamilyID != nil {
		membership, err = h.FamilyStore.GetFamilyMembershipForFamily(r.Context(), claims.UserID, *claims.FamilyID)
		if err != nil {
			log.Printf("get current user membership after profile update: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to load membership")
			return
		}
	}

	writeJSON(w, http.StatusOK, currentUserToResponse(user, membership))
}
