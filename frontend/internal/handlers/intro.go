package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/andreistefanciprian/yauli/frontend/internal/authclient"
)

// ShowIntro renders the public front door. If a valid session cookie is
// already present, it quietly sends the user to the right authenticated page.
func (h *Handlers) ShowIntro(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		h.renderIntro(w)
		return
	}

	result, err := h.Auth.MintToken(r.Context(), cookie.Value)
	if errors.Is(err, authclient.ErrUnauthorized) {
		http.SetCookie(w, h.sessionCookie("", -1))
		h.renderIntro(w)
		return
	}
	if err != nil {
		log.Printf("mint access token for intro redirect: %v", err)
		h.renderIntro(w)
		return
	}

	if result.FamilyID == nil {
		http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (h *Handlers) renderIntro(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "intro", nil); err != nil {
		log.Printf("render intro template: %v", err)
	}
}
