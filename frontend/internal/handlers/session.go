package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/andreistefanciprian/yauli/frontend/internal/authclient"
	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
)

// sessionIDContextKey carries the yauli_session cookie value that
// mintAccessToken already read and validated, so handlers past the
// middleware (e.g. onboarding's CreateFirstBaby, which needs it for
// AttachFamily) never have to re-read the cookie themselves — a context
// lookup fails safely (ok=false) if that invariant is ever broken, unlike a
// blind second r.Cookie call.
type sessionIDContextKey struct{}

func sessionIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionIDContextKey{}).(string)
	return id, ok
}

// RequireSession mints a fresh access token from auth-service on every
// request — no caching, so the frontend stays fully stateless (see
// docs/auth-magic-link.md) — and attaches it, plus the session id, to the
// request context. A session with no family yet is routed to /onboarding
// instead of the requested page, since nothing past this point has a baby
// to show.
func (h *Handlers) RequireSession(next http.Handler) http.Handler {
	return h.requireSession(true, "/onboarding", next)
}

// RequireOnboardingSession is RequireSession's counterpart for the
// onboarding routes themselves — the one place a session with no family yet
// is expected, rather than redirected away. A session that already has a
// family is sent to "/" instead, since onboarding has nothing left to do
// for it.
func (h *Handlers) RequireOnboardingSession(next http.Handler) http.Handler {
	return h.requireSession(false, "/", next)
}

// requireSession is the shared implementation behind RequireSession and
// RequireOnboardingSession, which differ only in which family_id state
// belongs on their routes vs. should be redirected elsewhere.
func (h *Handlers) requireSession(wantFamily bool, elseRedirect string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID, result, ok := h.mintAccessToken(w, r)
		if !ok {
			return
		}
		if (result.FamilyID != nil) != wantFamily {
			http.Redirect(w, r, elseRedirect, http.StatusSeeOther)
			return
		}

		ctx := backendclient.ContextWithToken(r.Context(), result.AccessToken)
		ctx = context.WithValue(ctx, sessionIDContextKey{}, sessionID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// mintAccessToken exchanges the yauli_session cookie for a fresh access
// token, handling the shared failure modes itself: no cookie, or
// auth-service reporting the session invalid/revoked/expired, both redirect
// to /login (clearing the cookie in the latter case, since it's confirmed
// dead rather than just unconfirmed). A transport/server error leaves the
// cookie alone and serves a 502 instead, so a momentary auth-service hiccup
// doesn't sign a valid session out. Callers just return when ok is false.
func (h *Handlers) mintAccessToken(w http.ResponseWriter, r *http.Request) (sessionID string, result authclient.MintResult, ok bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return "", authclient.MintResult{}, false
	}

	result, err = h.Auth.MintToken(r.Context(), cookie.Value)
	if errors.Is(err, authclient.ErrUnauthorized) {
		http.SetCookie(w, h.sessionCookie("", -1))
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return "", authclient.MintResult{}, false
	}
	if err != nil {
		log.Printf("mint access token: %v", err)
		http.Error(w, "failed to load session", http.StatusBadGateway)
		return "", authclient.MintResult{}, false
	}

	return cookie.Value, result, true
}
