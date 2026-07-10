package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
)

type babySettingsPageData struct {
	Baby        backendclient.Baby
	UpdateError string
	DeleteError string
	Notice      string
}

func (h *Handlers) ShowBabySettings(w http.ResponseWriter, r *http.Request) {
	h.renderBabySettings(w, r, babySettingsPageData{})
}

func (h *Handlers) UpdateBabySettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	timezone := strings.TrimSpace(r.FormValue("timezone"))
	if name == "" {
		h.renderBabySettings(w, r, babySettingsPageData{UpdateError: "Baby name is required."})
		return
	}

	baby, err := h.Backend.UpdateCurrentBaby(r.Context(), name, timezone)
	if err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			http.Error(w, "only the owner can update baby settings", http.StatusForbidden)
			return
		}
		log.Printf("update baby settings: %v", err)
		h.renderBabySettings(w, r, babySettingsPageData{UpdateError: "Could not save changes. Please try again."})
		return
	}

	h.renderBabySettings(w, r, babySettingsPageData{Baby: baby, Notice: "Baby settings updated."})
}

func (h *Handlers) ArchiveCurrentBaby(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	baby, _, err := h.currentBabyLocation(r.Context())
	if err != nil {
		if errors.Is(err, backendclient.ErrNotFound) {
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
		log.Printf("load baby for archive: %v", err)
		http.Error(w, "failed to load baby", http.StatusBadGateway)
		return
	}

	confirmName := strings.TrimSpace(r.FormValue("confirm_name"))
	if confirmName != baby.Name {
		h.renderBabySettings(w, r, babySettingsPageData{
			Baby:        baby,
			DeleteError: "Type the baby name exactly to delete this timeline.",
		})
		return
	}

	if err := h.Backend.ArchiveCurrentBaby(r.Context(), confirmName); err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			http.Error(w, "only the owner can delete this baby", http.StatusForbidden)
			return
		}
		log.Printf("archive baby: %v", err)
		h.renderBabySettings(w, r, babySettingsPageData{
			Baby:        baby,
			DeleteError: "Could not delete this baby. Please try again.",
		})
		return
	}

	http.SetCookie(w, h.sessionCookie("", -1))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handlers) renderBabySettings(w http.ResponseWriter, r *http.Request, data babySettingsPageData) {
	if data.Baby.ID == "" {
		baby, _, err := h.currentBabyLocation(r.Context())
		if err != nil {
			if errors.Is(err, backendclient.ErrNotFound) {
				http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
				return
			}
			log.Printf("load baby settings: %v", err)
			http.Error(w, "failed to load baby", http.StatusBadGateway)
			return
		}
		data.Baby = baby
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "baby-settings", data); err != nil {
		log.Printf("render baby settings template: %v", err)
	}
}
