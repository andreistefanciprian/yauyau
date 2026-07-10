package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/andreistefanciprian/yauli/frontend/internal/authclient"
)

// onboardingPageData is the onboarding page's template data, named for the
// same reason as loginPageData/verifyPageData.
type onboardingPageData struct {
	Error string
}

func (h *Handlers) ShowOnboarding(w http.ResponseWriter, r *http.Request) {
	h.renderOnboarding(w, onboardingPageData{})
}

func (h *Handlers) renderOnboarding(w http.ResponseWriter, data onboardingPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "onboarding", data); err != nil {
		log.Printf("render onboarding template: %v", err)
	}
}

// CreateFirstBaby is onboarding's single step: create the caller's first
// baby (backend-api creates their family implicitly in the same call, see
// backend-api's CreateBaby), attach the resulting family to their session,
// and redirect to the dashboard — RequireSession mints a fresh, now
// family-carrying token on the next request, so there's nothing left to
// hand off here.
//
// Known, accepted gap: CreateBaby and AttachFamily are two independent
// calls with no compensation between them. If AttachFamily fails after
// CreateBaby already succeeded (e.g. a transient auth-service error) and
// the user retries, backend-api's CreateBaby reuses the family it already
// created for them (see its familyID==nil branch) but still inserts a
// second baby row — a duplicate baby, not an orphaned family, and one the
// user can delete from the dashboard once they're in. Not worth an
// idempotency key for a single-step form with a narrow failure window.
func (h *Handlers) CreateFirstBaby(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.renderOnboarding(w, onboardingPageData{Error: "Your baby's name is required."})
		return
	}

	baby, err := h.Backend.CreateBaby(r.Context(), name)
	if err != nil {
		log.Printf("create first baby: %v", err)
		h.renderOnboarding(w, onboardingPageData{Error: "Something went wrong. Please try again."})
		return
	}

	sessionID, ok := sessionIDFromContext(r.Context())
	if !ok {
		// RequireOnboardingSession always sets this — only unreachable if
		// that invariant is ever broken, in which case failing safely back
		// to /login beats a nil-pointer panic.
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := h.Auth.AttachFamily(r.Context(), sessionID, baby.FamilyID); err != nil && !errors.Is(err, authclient.ErrAlreadyAttached) {
		log.Printf("attach family: %v", err)
		h.renderOnboarding(w, onboardingPageData{Error: "Something went wrong. Please try again."})
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
