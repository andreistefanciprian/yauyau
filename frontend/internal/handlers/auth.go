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

// sessionCookieTTL matches auth-service's session expiry (see
// docs/auth-magic-link.md's "Session TTL").
const sessionCookieTTL = 30 * 24 * time.Hour

// loginPageData is the login page/partial's template data — named instead
// of an inline anonymous struct since it's shared by ShowLogin, the two
// renderLogin error paths in RequestMagicLink, and renderLogin's own
// parameter, so every call site is guaranteed to agree on its shape.
type loginPageData struct {
	Email, Error string
	// SignedOut swaps the tagline/subtitle for a "Welcome back" greeting
	// when arriving here right after signing out, rather than the
	// first-time-visitor pitch.
	SignedOut bool
}

// verifyPageData is the verify page's template data, named for the same
// reason as loginPageData.
type verifyPageData struct {
	Token, Error string
}

func (h *Handlers) ShowLogin(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, loginPageData{SignedOut: r.URL.Query().Get("signed_out") == "1"})
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
	signedOut := r.FormValue("signed_out") == "1"
	if email == "" {
		h.renderLogin(w, loginPageData{Error: "Email is required.", SignedOut: signedOut})
		return
	}

	if err := h.Auth.RequestMagicLink(r.Context(), email); err != nil {
		log.Printf("request magic link: %v", err)
		h.renderLogin(w, loginPageData{Email: email, Error: "Something went wrong. Please try again.", SignedOut: signedOut})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, "login-sent", struct{ Email string }{Email: email}); err != nil {
		log.Printf("render login-sent template: %v", err)
	}
}

func (h *Handlers) renderLogin(w http.ResponseWriter, data loginPageData) {
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
		h.renderVerify(w, verifyPageData{Error: "This link is missing its token."})
		return
	}
	h.renderVerify(w, verifyPageData{Token: token})
}

// ConfirmVerify consumes the token, opens the yauli_session cookie from the
// returned session, and redirects to the public front door, which sends
// already-authenticated users to the dashboard or onboarding as appropriate.
func (h *Handlers) ConfirmVerify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	// PostFormValue, not FormValue: FormValue falls back to the URL query
	// string, which would let a POST /auth/verify?token=... consume the
	// token via the query string instead of the hidden form field it's
	// supposed to come from. cmd/server/main.go additionally keeps this
	// whole route out of the standard logger regardless, since a query
	// string can be present on the request even when this handler ignores
	// it.
	token := r.PostFormValue("token")
	if token == "" {
		h.renderVerify(w, verifyPageData{Error: "This link is missing its token."})
		return
	}

	result, err := h.Auth.VerifyMagicLink(r.Context(), token)
	if err != nil {
		log.Printf("verify magic link: %v", err)
		h.renderVerify(w, verifyPageData{Error: "This link is invalid or has expired."})
		return
	}

	http.SetCookie(w, h.sessionCookie(result.SessionID, sessionCookieTTL))

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) renderVerify(w http.ResponseWriter, data verifyPageData) {
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

	http.SetCookie(w, h.sessionCookie("", -1))

	http.Redirect(w, r, "/login?signed_out=1", http.StatusSeeOther)
}

// sessionCookie builds the yauli_session cookie, the single point of truth
// for its Path/HttpOnly/Secure/SameSite attributes so ConfirmVerify's set
// and Logout's clear can never drift out of sync with each other. A
// negative ttl clears the cookie (MaxAge=-1); otherwise it expires ttl from
// now.
func (h *Handlers) sessionCookie(value string, ttl time.Duration) *http.Cookie {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	}
	if ttl < 0 {
		cookie.MaxAge = -1
	} else {
		cookie.Expires = time.Now().Add(ttl)
	}
	return cookie
}
