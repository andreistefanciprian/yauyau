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
	SexOptions  []babySexOption
	UpdateError string
	DeleteError string
	Notice      string
}

type babySexOption struct {
	Value    string
	Label    string
	Selected bool
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
	birthDate := strings.TrimSpace(r.FormValue("birth_date"))
	birthWeightKg := strings.TrimSpace(r.FormValue("birth_weight_kg"))
	birthLengthCm := strings.TrimSpace(r.FormValue("birth_length_cm"))
	sex := strings.TrimSpace(r.FormValue("sex"))
	if name == "" {
		h.renderBabySettings(w, r, babySettingsPageData{UpdateError: "Baby name is required."})
		return
	}

	baby, err := h.Backend.UpdateCurrentBaby(r.Context(), backendclient.Baby{
		Name:          name,
		Timezone:      timezone,
		BirthDate:     birthDate,
		BirthWeightKg: birthWeightKg,
		BirthLengthCm: birthLengthCm,
		Sex:           sex,
	})
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
	data.SexOptions = sexOptions(data.Baby.Sex)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "baby-settings", data); err != nil {
		log.Printf("render baby settings template: %v", err)
	}
}

func sexOptions(selected string) []babySexOption {
	options := []babySexOption{
		{Value: "", Label: "Not set"},
		{Value: "female", Label: "Female"},
		{Value: "male", Label: "Male"},
		{Value: "not_specified", Label: "Prefer not to say"},
	}
	for i := range options {
		options[i].Selected = options[i].Value == selected
	}
	return options
}
