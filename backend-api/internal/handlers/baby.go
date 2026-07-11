package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/andreistefanciprian/yauli/backend-api/internal/authctx"
	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

// defaultBabyTimezone matches babies.timezone's DB default (0001_init.sql) —
// applied here rather than left to the DB default so an explicit empty
// string in the request doesn't defeat it.
const defaultBabyTimezone = "Australia/Adelaide"

type babyResponse struct {
	ID               string `json:"id"`
	FamilyID         string `json:"family_id"`
	Name             string `json:"name"`
	Timezone         string `json:"timezone"`
	BirthDate        string `json:"birth_date,omitempty"`
	BirthWeightKg    string `json:"birth_weight_kg,omitempty"`
	BirthLengthCm    string `json:"birth_length_cm,omitempty"`
	Sex              string `json:"sex,omitempty"`
	CanInvite        bool   `json:"can_invite,omitempty"`
	HasPendingInvite bool   `json:"has_pending_invite,omitempty"`
}

func babyToResponse(baby store.Baby, canInvite, hasPendingInvite bool) babyResponse {
	return babyResponse{
		ID:               baby.ID.String(),
		FamilyID:         baby.FamilyID.String(),
		Name:             baby.Name,
		Timezone:         baby.Timezone,
		BirthDate:        baby.BirthDate,
		BirthWeightKg:    baby.BirthWeightKg,
		BirthLengthCm:    baby.BirthLengthCm,
		Sex:              baby.Sex,
		CanInvite:        canInvite,
		HasPendingInvite: hasPendingInvite,
	}
}

// requireClaims returns the caller's claims, writing a 401 and returning
// ok=false if the request carried none.
func requireClaims(w http.ResponseWriter, r *http.Request) (authctx.Claims, bool) {
	claims, ok := authctx.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
	}
	return claims, ok
}

// GetCurrentBaby returns the caller's family's "current" baby. A family is
// never named or shown in the UI, so this is the only baby-scoped read the
// frontend needs before it has a specific baby id to work with. It trusts
// claims.FamilyID as decoded from the JWT rather than re-querying family
// membership, so it only sees a family created by CreateBaby once the caller
// holds a token re-minted after that (auth-service's attach-family, a later
// PR) - a stale token from before the family existed still reports 404 here.
func (h *Handlers) GetCurrentBaby(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	if claims.FamilyID == nil {
		writeError(w, http.StatusNotFound, "baby not found")
		return
	}

	baby, err := h.Store.GetCurrentBaby(r.Context(), *claims.FamilyID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "baby not found")
		return
	}
	if err != nil {
		log.Printf("get current baby: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load baby")
		return
	}

	membership, err := h.FamilyStore.GetFamilyMembershipForFamily(r.Context(), claims.UserID, *claims.FamilyID)
	if err != nil {
		log.Printf("get current baby membership: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load membership")
		return
	}
	canInvite := membership.Found && membership.Role == store.MembershipRoleOwner && membership.Status == store.MembershipStatusActive

	hasPendingInvite, err := h.FamilyStore.HasPendingInviteOutsideFamily(r.Context(), claims.UserID, baby.FamilyID)
	if err != nil {
		log.Printf("check pending invite: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load membership")
		return
	}

	writeJSON(w, http.StatusOK, babyToResponse(baby, canInvite, hasPendingInvite))
}

type createBabyRequest struct {
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

type updateBabyRequest struct {
	Name          string `json:"name"`
	Timezone      string `json:"timezone"`
	BirthDate     string `json:"birth_date"`
	BirthWeightKg string `json:"birth_weight_kg"`
	BirthLengthCm string `json:"birth_length_cm"`
	Sex           string `json:"sex"`
}

type archiveBabyRequest struct {
	ConfirmName string `json:"confirm_name"`
}

// CreateBaby adds a baby for the caller. A user with no existing family
// membership gets one created implicitly, as this baby's owner, in the same
// call — family is a backend-only tenancy boundary, never a concept the UI
// asks the user about (see the PR plan). A user who already belongs to a
// family just gets a sibling baby added to it.
func (h *Handlers) CreateBaby(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}

	var req createBabyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	timezone, ok := normalizeBabyTimezone(w, req.Timezone)
	if !ok {
		return
	}
	req.Timezone = timezone

	membership, err := h.FamilyStore.GetFamilyMembership(r.Context(), claims.UserID)
	if err != nil {
		log.Printf("get family membership for create baby: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load family membership")
		return
	}

	familyID := membership.FamilyID
	canInvite := membership.Role == store.MembershipRoleOwner && membership.Status == store.MembershipStatusActive
	switch {
	case familyID == nil:
		// familyName is never shown to users - it only exists to satisfy
		// families.name NOT NULL.
		newFamilyID, err := h.FamilyStore.CreateFamilyWithOwner(r.Context(), claims.UserID, fmt.Sprintf("family-%s", claims.UserID))
		if errors.Is(err, store.ErrActiveMembershipExists) {
			// Lost a race with a concurrent CreateBaby call for the same
			// brand-new user (e.g. a double-submitted "add your baby" form)
			// - re-fetch rather than fail, since the other call already
			// created the family.
			membership, err = h.FamilyStore.GetFamilyMembership(r.Context(), claims.UserID)
			if err != nil {
				log.Printf("get family membership after create-family race: %v", err)
				writeError(w, http.StatusInternalServerError, "failed to load family membership")
				return
			}
			if membership.FamilyID == nil {
				writeError(w, http.StatusInternalServerError, "failed to create family")
				return
			}
			familyID = membership.FamilyID
			canInvite = membership.Role == store.MembershipRoleOwner && membership.Status == store.MembershipStatusActive
		} else if err != nil {
			log.Printf("create family for new baby: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to create family")
			return
		} else {
			familyID = &newFamilyID
			canInvite = true
		}
	case membership.Status == store.MembershipStatusInvited:
		// The caller's only membership is a pending invite they haven't
		// logged in to accept yet (ActivateInvitedMembership normally runs
		// at login) - activate it now rather than let them write into a
		// family they're not formally an active member of.
		if err := h.FamilyStore.ActivateInvitedMembership(r.Context(), claims.UserID, *familyID); err != nil {
			log.Printf("activate invited membership for create baby: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to activate family membership")
			return
		}
		canInvite = false
	}

	baby, err := h.Store.CreateBaby(r.Context(), *familyID, req.Name, req.Timezone)
	if err != nil {
		log.Printf("create baby: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create baby")
		return
	}

	writeJSON(w, http.StatusCreated, babyToResponse(baby, canInvite, false))
}

// UpdateCurrentBaby lets an active owner edit the current baby's profile
// fields. The "current" baby is resolved from the session's family_id rather
// than supplied by the caller, matching the rest of the frontend-facing
// /babies/current API.
func (h *Handlers) UpdateCurrentBaby(w http.ResponseWriter, r *http.Request) {
	_, baby, ok := h.requireCurrentBabyOwner(w, r, "only the owner can update baby settings")
	if !ok {
		return
	}

	var req updateBabyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Timezone = strings.TrimSpace(req.Timezone)
	req.BirthDate = strings.TrimSpace(req.BirthDate)
	req.BirthWeightKg = strings.TrimSpace(req.BirthWeightKg)
	req.BirthLengthCm = strings.TrimSpace(req.BirthLengthCm)
	req.Sex = strings.TrimSpace(req.Sex)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	timezone, ok := normalizeBabyTimezone(w, req.Timezone)
	if !ok {
		return
	}

	if !validateOptionalDate(w, req.BirthDate) {
		return
	}
	if !validateOptionalPositiveDecimal(w, req.BirthWeightKg, "birth_weight_kg", 999.99) {
		return
	}
	if !validateOptionalPositiveDecimal(w, req.BirthLengthCm, "birth_length_cm", 9999.9) {
		return
	}
	if !validateBabySex(w, req.Sex) {
		return
	}

	updated, err := h.Store.UpdateBaby(r.Context(), baby.FamilyID, baby.ID, store.Baby{
		Name:          req.Name,
		Timezone:      timezone,
		BirthDate:     req.BirthDate,
		BirthWeightKg: req.BirthWeightKg,
		BirthLengthCm: req.BirthLengthCm,
		Sex:           req.Sex,
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "baby not found")
		return
	}
	if err != nil {
		log.Printf("update current baby: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update baby")
		return
	}

	writeJSON(w, http.StatusOK, babyToResponse(updated, true, false))
}

func normalizeBabyTimezone(w http.ResponseWriter, raw string) (string, bool) {
	timezone := strings.TrimSpace(raw)
	if timezone == "" {
		timezone = defaultBabyTimezone
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		writeError(w, http.StatusBadRequest, "timezone is invalid")
		return "", false
	}
	return timezone, true
}

func validateOptionalDate(w http.ResponseWriter, value string) bool {
	if value == "" {
		return true
	}
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		writeError(w, http.StatusBadRequest, "birth_date must be YYYY-MM-DD")
		return false
	}
	return true
}

func validateOptionalPositiveDecimal(w http.ResponseWriter, value, field string, max float64) bool {
	if value == "" {
		return true
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		writeError(w, http.StatusBadRequest, field+" must be a positive number")
		return false
	}
	if parsed > max {
		writeError(w, http.StatusBadRequest, field+" is too large")
		return false
	}
	return true
}

func validateBabySex(w http.ResponseWriter, sex string) bool {
	switch sex {
	case "", "female", "male", "not_specified":
		return true
	default:
		writeError(w, http.StatusBadRequest, "sex is invalid")
		return false
	}
}

// ArchiveCurrentBaby soft-deletes the current baby's timeline after the
// owner types the baby's exact name. The data remains in Postgres for future
// recovery/audit, but active baby and event routes stop returning it.
func (h *Handlers) ArchiveCurrentBaby(w http.ResponseWriter, r *http.Request) {
	claims, baby, ok := h.requireCurrentBabyOwner(w, r, "only the owner can delete this baby")
	if !ok {
		return
	}

	var req archiveBabyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.ConfirmName) != baby.Name {
		writeError(w, http.StatusBadRequest, "confirmation name does not match")
		return
	}

	if err := h.Store.ArchiveBaby(r.Context(), baby.FamilyID, baby.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "baby not found")
			return
		}
		log.Printf("archive current baby: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete baby")
		return
	}

	if err := h.Auth.RevokeFamilyMemberSessions(r.Context(), claims.UserID, baby.FamilyID); err != nil {
		log.Printf("revoke archived baby owner sessions: %v", err)
	}
	if err := h.FamilyStore.RemoveTimelineMember(r.Context(), baby.FamilyID, claims.UserID); err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Printf("remove archived baby owner membership: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete baby")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) requireCurrentBabyOwner(w http.ResponseWriter, r *http.Request, forbiddenMessage string) (authctx.Claims, store.Baby, bool) {
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
		log.Printf("get current baby for owner check: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load baby")
		return authctx.Claims{}, store.Baby{}, false
	}

	membership, err := h.FamilyStore.GetFamilyMembershipForFamily(r.Context(), claims.UserID, baby.FamilyID)
	if err != nil {
		log.Printf("get membership for current baby owner check: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load membership")
		return authctx.Claims{}, store.Baby{}, false
	}
	if !membership.Found || membership.Role != store.MembershipRoleOwner || membership.Status != store.MembershipStatusActive {
		writeError(w, http.StatusForbidden, forbiddenMessage)
		return authctx.Claims{}, store.Baby{}, false
	}

	return claims, baby, true
}
