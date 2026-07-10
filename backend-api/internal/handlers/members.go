package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/authctx"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

type timelineMemberResponse struct {
	UserID       string `json:"user_id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	Relationship string `json:"relationship,omitempty"`
}

type listTimelineMembersResponse struct {
	Members []timelineMemberResponse `json:"members"`
}

type updateTimelineMemberRequest struct {
	Relationship string `json:"relationship"`
}

func timelineMemberToResponse(member store.TimelineMember) timelineMemberResponse {
	return timelineMemberResponse{
		UserID:       member.UserID.String(),
		Email:        member.Email,
		Role:         string(member.Role),
		Status:       string(member.Status),
		Relationship: member.Relationship,
	}
}

// ListTimelineMembers returns the active and invited people with access to
// the current baby's timeline. For this first access-management slice, only
// active owners can view/manage the list.
func (h *Handlers) ListTimelineMembers(w http.ResponseWriter, r *http.Request) {
	_, baby, ok := h.requireTimelineOwner(w, r)
	if !ok {
		return
	}

	members, err := h.FamilyStore.ListTimelineMembers(r.Context(), baby.FamilyID)
	if err != nil {
		log.Printf("list timeline members: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load timeline members")
		return
	}

	resp := listTimelineMembersResponse{Members: make([]timelineMemberResponse, len(members))}
	for i, member := range members {
		resp.Members[i] = timelineMemberToResponse(member)
	}

	writeJSON(w, http.StatusOK, resp)
}

// UpdateTimelineMember stores a relationship label for a timeline member.
// Relationship is human profile context ("Grandpa"), not an authorization
// role.
func (h *Handlers) UpdateTimelineMember(w http.ResponseWriter, r *http.Request) {
	_, baby, ok := h.requireTimelineOwner(w, r)
	if !ok {
		return
	}

	memberID, ok := parseMemberID(w, r)
	if !ok {
		return
	}

	var req updateTimelineMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := h.FamilyStore.UpdateTimelineMemberRelationship(r.Context(), baby.FamilyID, memberID, strings.TrimSpace(req.Relationship)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "timeline member not found")
			return
		}
		log.Printf("update timeline member: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update timeline member")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveTimelineMember removes a person's access to the current baby's
// timeline. Pending invites are deleted immediately; active members have
// their auth-service sessions revoked before the membership row is removed.
func (h *Handlers) RemoveTimelineMember(w http.ResponseWriter, r *http.Request) {
	claims, baby, ok := h.requireTimelineOwner(w, r)
	if !ok {
		return
	}

	memberID, ok := parseMemberID(w, r)
	if !ok {
		return
	}
	if memberID == claims.UserID {
		writeError(w, http.StatusConflict, "owners cannot remove their own access")
		return
	}

	membership, err := h.FamilyStore.GetFamilyMembershipForFamily(r.Context(), memberID, baby.FamilyID)
	if err != nil {
		log.Printf("get target membership for removal: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load timeline member")
		return
	}
	if !membership.Found {
		writeError(w, http.StatusNotFound, "timeline member not found")
		return
	}
	if membership.Role == store.MembershipRoleOwner {
		writeError(w, http.StatusConflict, "owners cannot be removed yet")
		return
	}
	if membership.Status == store.MembershipStatusActive {
		if err := h.Auth.RevokeFamilyMemberSessions(r.Context(), memberID, baby.FamilyID); err != nil {
			log.Printf("revoke removed member sessions: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to remove timeline member")
			return
		}
	}

	if err := h.FamilyStore.RemoveTimelineMember(r.Context(), baby.FamilyID, memberID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "timeline member not found")
			return
		}
		log.Printf("remove timeline member: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to remove timeline member")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseMemberID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	memberID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return uuid.Nil, false
	}
	return memberID, true
}

func (h *Handlers) requireTimelineOwner(w http.ResponseWriter, r *http.Request) (authctx.Claims, store.Baby, bool) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return authctx.Claims{}, store.Baby{}, false
	}
	if claims.FamilyID == nil {
		writeError(w, http.StatusNotFound, "baby not found")
		return authctx.Claims{}, store.Baby{}, false
	}

	baby, err := h.Store.GetCurrentBaby(r.Context(), *claims.FamilyID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "baby not found")
		return authctx.Claims{}, store.Baby{}, false
	}
	if err != nil {
		log.Printf("get current baby for timeline members: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load baby")
		return authctx.Claims{}, store.Baby{}, false
	}

	membership, err := h.FamilyStore.GetFamilyMembershipForFamily(r.Context(), claims.UserID, baby.FamilyID)
	if err != nil {
		log.Printf("get membership for timeline members: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load membership")
		return authctx.Claims{}, store.Baby{}, false
	}
	if !membership.Found || membership.Role != store.MembershipRoleOwner || membership.Status != store.MembershipStatusActive {
		writeError(w, http.StatusForbidden, "only the owner can manage timeline access")
		return authctx.Claims{}, store.Baby{}, false
	}

	return claims, baby, true
}
