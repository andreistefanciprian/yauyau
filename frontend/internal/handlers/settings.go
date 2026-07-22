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

type settingsPageData struct {
	Baby            backendclient.Baby
	Account         accountViewData
	SexOptions      []babySexOption
	Members         []backendclient.TimelineMember
	CanManageBaby   bool
	CanManageAccess bool
	Invite          inviteStatus

	SettingsNotice string
	AccountNotice  string
	AccountError   string
	BabyNotice     string
	UpdateError    string
	DeleteError    string
	TimelineNotice string
}

type babySexOption struct {
	Value    string
	Label    string
	Selected bool
}

func (h *Handlers) ShowSettings(w http.ResponseWriter, r *http.Request) {
	h.renderSettings(w, r, settingsPageData{})
}

func (h *Handlers) UpdateAccountSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	user, err := h.Backend.UpdateCurrentUser(r.Context(), displayName)
	if err != nil {
		log.Printf("update account settings: %v", err)
		h.renderSettings(w, r, settingsPageData{AccountError: "Could not save account settings. Please try again."})
		return
	}

	h.renderSettings(w, r, settingsPageData{
		Account:       accountFromUser(user),
		AccountNotice: "Account settings updated.",
	})
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
		h.renderSettings(w, r, settingsPageData{UpdateError: "Baby name is required."})
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
			h.renderSettings(w, r, settingsPageData{SettingsNotice: "Only the timeline owner can update baby settings."})
			return
		}
		log.Printf("update baby settings: %v", err)
		h.renderSettings(w, r, settingsPageData{UpdateError: "Could not save changes. Please try again."})
		return
	}

	h.renderSettings(w, r, settingsPageData{Baby: baby, BabyNotice: "Baby settings updated."})
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
		h.renderSettings(w, r, settingsPageData{
			Baby:        baby,
			DeleteError: "Type the baby name exactly to delete this timeline.",
		})
		return
	}

	if err := h.Backend.ArchiveCurrentBaby(r.Context(), confirmName); err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			h.renderSettings(w, r, settingsPageData{SettingsNotice: "Only the timeline owner can delete this timeline."})
			return
		}
		log.Printf("archive baby: %v", err)
		h.renderSettings(w, r, settingsPageData{
			Baby:        baby,
			DeleteError: "Could not delete this baby. Please try again.",
		})
		return
	}

	http.SetCookie(w, h.sessionCookie("", -1))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handlers) CreateTimelineInvite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		h.renderSettings(w, r, settingsPageData{Invite: inviteStatus{Error: "Email is required."}})
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
		h.renderSettings(w, r, settingsPageData{Invite: inviteStatus{Error: "Something went wrong. Please try again."}})
		return
	}
	if err := h.Auth.RequestInviteMagicLink(r.Context(), email, baby.Name); err != nil {
		log.Printf("send invite magic link: %v", err)
		h.renderSettings(w, r, settingsPageData{Invite: inviteStatus{Error: "The invite was saved, but the email could not be sent. Please try again."}})
		return
	}

	h.renderSettings(w, r, settingsPageData{Invite: inviteStatus{Message: fmt.Sprintf("Invite sent to %s.", email)}})
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
		h.renderSettings(w, r, settingsPageData{TimelineNotice: "Could not update relationship. Please try again."})
		return
	}

	h.renderSettings(w, r, settingsPageData{TimelineNotice: "Relationship updated."})
}

// UpdateTimelineMembers saves the editable People with access fields from a
// single form. It intentionally reuses the existing per-member backend calls
// because this is a settings-page convenience, not a new backend bulk domain
// operation.
func (h *Handlers) UpdateTimelineMembers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	for _, userID := range r.PostForm["member_ids"] {
		relationship := strings.TrimSpace(r.FormValue("relationship_" + userID))
		if err := h.Backend.UpdateTimelineMemberRelationship(r.Context(), userID, relationship); err != nil {
			if errors.Is(err, backendclient.ErrForbidden) {
				http.Error(w, "only the owner can update timeline access", http.StatusForbidden)
				return
			}
			log.Printf("update timeline member relationship: %v", err)
			h.renderSettings(w, r, settingsPageData{TimelineNotice: "Could not update people with access. Please try again."})
			return
		}
	}

	for _, userID := range r.PostForm["report_email_member_ids"] {
		enabled := r.FormValue("daily_report_email_enabled_"+userID) == "on"
		if err := h.Backend.UpdateTimelineMemberReportPreferences(r.Context(), userID, enabled); err != nil {
			if errors.Is(err, backendclient.ErrForbidden) {
				http.Error(w, "only the owner can update report emails", http.StatusForbidden)
				return
			}
			if errors.Is(err, backendclient.ErrNotFound) {
				h.renderSettings(w, r, settingsPageData{TimelineNotice: "Only active members can receive report emails."})
				return
			}
			log.Printf("update timeline member report emails: %v", err)
			h.renderSettings(w, r, settingsPageData{TimelineNotice: "Could not update people with access. Please try again."})
			return
		}
	}

	h.renderSettings(w, r, settingsPageData{TimelineNotice: "People with access updated."})
}

// UpdateTimelineMemberReportEmails saves the owner's recipient choice for one
// active timeline member. This is separate from UpdateReportEmailSettings so
// a helper cannot self-subscribe through the account settings endpoint.
func (h *Handlers) UpdateTimelineMemberReportEmails(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	userID := chi.URLParam(r, "userID")
	enabled := r.FormValue("daily_report_email_enabled") == "on"
	if err := h.Backend.UpdateTimelineMemberReportPreferences(r.Context(), userID, enabled); err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			http.Error(w, "only the owner can update report emails", http.StatusForbidden)
			return
		}
		if errors.Is(err, backendclient.ErrNotFound) {
			h.renderSettings(w, r, settingsPageData{TimelineNotice: "Only active members can receive report emails."})
			return
		}
		log.Printf("update timeline member report emails: %v", err)
		h.renderSettings(w, r, settingsPageData{TimelineNotice: "Could not update report emails. Please try again."})
		return
	}

	h.renderSettings(w, r, settingsPageData{TimelineNotice: "Report email settings updated."})
}

func (h *Handlers) RemoveTimelineMember(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if err := h.Backend.RemoveTimelineMember(r.Context(), userID); err != nil {
		if errors.Is(err, backendclient.ErrForbidden) {
			http.Error(w, "only the owner can update timeline access", http.StatusForbidden)
			return
		}
		log.Printf("remove timeline member: %v", err)
		h.renderSettings(w, r, settingsPageData{TimelineNotice: "Could not update access. Please try again."})
		return
	}

	h.renderSettings(w, r, settingsPageData{TimelineNotice: "Access updated."})
}

func (h *Handlers) renderSettings(w http.ResponseWriter, r *http.Request, data settingsPageData) {
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
	if data.Account.Email == "" {
		data.Account = h.loadAccount(r.Context())
	}
	data.CanManageBaby = data.Baby.CanInvite
	data.SexOptions = sexOptions(data.Baby.Sex)

	// Only the owner can list timeline members; a non-owner member should
	// still see their account and timeline context, so a forbidden response
	// here degrades the People/Invite sections instead of failing the page.
	result, err := h.Backend.ListTimelineMembers(r.Context())
	if err != nil {
		if !errors.Is(err, backendclient.ErrForbidden) {
			log.Printf("list timeline members: %v", err)
			http.Error(w, "failed to load timeline access", http.StatusBadGateway)
			return
		}
	} else {
		data.Members = result.Members
		data.CanManageAccess = true
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "settings", data); err != nil {
		log.Printf("render settings template: %v", err)
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
