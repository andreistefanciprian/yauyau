package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
)

type timelineSettingsPageData struct {
	Baby    backendclient.Baby
	Members []backendclient.TimelineMember
	Invite  inviteStatus
	Notice  string
}

func (h *Handlers) ShowTimelineSettings(w http.ResponseWriter, r *http.Request) {
	h.renderTimelineSettings(w, r, inviteStatus{}, "")
}

func (h *Handlers) renderTimelineSettings(w http.ResponseWriter, r *http.Request, invite inviteStatus, notice string) {
	baby, _, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	result, err := h.Backend.ListTimelineMembers(r.Context())
	if err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			http.Error(w, fmt.Sprintf("only the person who added %s can manage timeline access", baby.Name), http.StatusForbidden)
			return
		}
		log.Printf("list timeline members: %v", err)
		http.Error(w, "failed to load timeline access", http.StatusBadGateway)
		return
	}

	data := timelineSettingsPageData{
		Baby:    baby,
		Members: result.Members,
		Invite:  invite,
		Notice:  notice,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "timeline-settings", data); err != nil {
		log.Printf("render timeline settings template: %v", err)
	}
}

func (h *Handlers) CreateTimelineInvite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		h.renderTimelineSettings(w, r, inviteStatus{Error: "Email is required."}, "")
		return
	}

	baby, _, err := h.currentBabyLocation(r.Context())
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	if err := h.Backend.InviteHelper(r.Context(), baby.ID, email); err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			http.Error(w, fmt.Sprintf("only the person who added %s can invite helpers", baby.Name), http.StatusForbidden)
			return
		}
		log.Printf("invite helper: %v", err)
		h.renderTimelineSettings(w, r, inviteStatus{Error: "Something went wrong. Please try again."}, "")
		return
	}
	if err := h.Auth.RequestInviteMagicLink(r.Context(), email, baby.Name); err != nil {
		log.Printf("send invite magic link: %v", err)
		h.renderTimelineSettings(w, r, inviteStatus{Error: "The invite was saved, but the email could not be sent. Please try again."}, "")
		return
	}

	h.renderTimelineSettings(w, r, inviteStatus{Message: fmt.Sprintf("Invite sent to %s.", email)}, "")
}

func (h *Handlers) UpdateTimelineMemberRelationship(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	userID := chi.URLParam(r, "userID")
	relationship := strings.TrimSpace(r.FormValue("relationship"))
	if err := h.Backend.UpdateTimelineMemberRelationship(r.Context(), userID, relationship); err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			http.Error(w, "only the owner can update timeline access", http.StatusForbidden)
			return
		}
		log.Printf("update timeline member relationship: %v", err)
		h.renderTimelineSettings(w, r, inviteStatus{}, "Could not update relationship. Please try again.")
		return
	}

	h.renderTimelineSettings(w, r, inviteStatus{}, "Relationship updated.")
}

func (h *Handlers) RemoveTimelineMember(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if err := h.Backend.RemoveTimelineMember(r.Context(), userID); err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			http.Error(w, "only the owner can update timeline access", http.StatusForbidden)
			return
		}
		log.Printf("remove timeline member: %v", err)
		h.renderTimelineSettings(w, r, inviteStatus{}, "Could not update access. Please try again.")
		return
	}

	h.renderTimelineSettings(w, r, inviteStatus{}, "Access updated.")
}
