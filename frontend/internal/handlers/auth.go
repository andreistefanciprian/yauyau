package handlers

import (
	"log"
	"net/http"
	"time"
)

// sessionCookieName holds only an opaque session id — never a JWT. Access
// tokens are minted fresh from auth-service per request and never touch the
// browser (see docs/auth-magic-link.md).
const sessionCookieName = "yauli_session"

func (h *Handlers) ShowLogin(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, struct{ Email, Error string }{})
}

// RequestMagicLink asks auth-service to send a magic link, then always
// shows the same "check your inbox" state regardless of whether the email
// had an existing account — auth-service already guarantees an identical
// response either way, so the frontend has nothing further to normalize.
func (h *Handlers) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")
	if email == "" {
		h.renderLogin(w, struct{ Email, Error string }{Error: "Email is required."})
		return
	}

	if err := h.Auth.RequestMagicLink(r.Context(), email); err != nil {
		log.Printf("request magic link: %v", err)
		h.renderLogin(w, struct{ Email, Error string }{Email: email, Error: "Something went wrong. Please try again."})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "login-sent", struct{ Email string }{Email: email}); err != nil {
		log.Printf("render login-sent template: %v", err)
	}
}

func (h *Handlers) renderLogin(w http.ResponseWriter, data struct{ Email, Error string }) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "login", data); err != nil {
		log.Printf("render login template: %v", err)
	}
}

// ShowVerify renders the confirmation page for a clicked magic link. It
// deliberately does not consume the token on this bare GET — link
// prefetching by email-security scanners (Outlook Safe Links etc.) would
// otherwise burn the link before the user ever clicks it. Only
// ConfirmVerify's POST, triggered by an explicit button press, does that.
func (h *Handlers) ShowVerify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		h.renderVerify(w, struct{ Token, Error string }{Error: "This link is missing its token."})
		return
	}
	h.renderVerify(w, struct{ Token, Error string }{Token: token})
}

// ConfirmVerify consumes the token, opens the yauli_session cookie from the
// returned session, and redirects to the dashboard. Session gating (which
// branches on whether the session has a family yet) lands in PR8 — until
// then this always redirects to "/".
func (h *Handlers) ConfirmVerify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	token := r.FormValue("token")
	if token == "" {
		h.renderVerify(w, struct{ Token, Error string }{Error: "This link is missing its token."})
		return
	}

	result, err := h.Auth.VerifyMagicLink(r.Context(), token)
	if err != nil {
		log.Printf("verify magic link: %v", err)
		h.renderVerify(w, struct{ Token, Error string }{Error: "This link is invalid or has expired."})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    result.SessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) renderVerify(w http.ResponseWriter, data struct{ Token, Error string }) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "auth-verify", data); err != nil {
		log.Printf("render auth-verify template: %v", err)
	}
}

// Logout revokes the current session (best-effort — the cookie is cleared
// regardless of whether the revoke call succeeds, since the user's intent
// is "sign me out of this browser" either way) and redirects to /login.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if err := h.Auth.Logout(r.Context(), cookie.Value); err != nil {
			log.Printf("logout: %v", err)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
