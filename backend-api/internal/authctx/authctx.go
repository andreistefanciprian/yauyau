// Package authctx decodes the caller's identity from the Authorization:
// Bearer JWT that auth-service mints, and makes it available via
// context.Context. It deliberately does not check the JWT's signature or
// expiry yet (see docs/auth-magic-link.md and the PR plan) - that
// enforcement lands in a later PR, at which point this is the one place
// that changes. Everything built against Middleware/FromContext between now
// and then is exercising the real claims shape, not a stub.
package authctx

import (
	"context"
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang-jwt/jwt/v5/request"
	"github.com/google/uuid"
)

type contextKey int

const claimsContextKey contextKey = iota

// Claims is what backend-api knows about the caller. FamilyID is nil until
// the user has completed onboarding (see docs/auth-magic-link.md's nullable
// session/JWT family_id) - a brand-new user's token carries no family_id
// claim at all.
type Claims struct {
	UserID   uuid.UUID  `json:"user_id"`
	FamilyID *uuid.UUID `json:"family_id,omitempty"`
}

// jwtClaims mirrors the shape auth-service signs: sub=user_id, plus an
// optional family_id claim. FamilyID is decoded as a string, not
// *uuid.UUID, so that an empty-string value (as opposed to an omitted
// field) can be treated as "absent" in Middleware rather than failing the
// whole claims decode.
type jwtClaims struct {
	FamilyID string `json:"family_id,omitempty"`
	jwt.RegisteredClaims
}

// parser holds no per-call mutable state, so it's safe to share across
// requests instead of constructing a new one every time.
var parser = jwt.NewParser()

// bearerExtractor pulls the token out of "Authorization: Bearer <token>",
// matching the scheme case-insensitively per RFC 7235.
var bearerExtractor = request.BearerExtractor{}

// Middleware decodes the Authorization: Bearer JWT's claims into context,
// without verifying its signature or expiry. A missing or unparseable
// token simply leaves no claims in context - callers use FromContext to
// tell "no claims" apart from "authenticated as this user". Every rejection
// is logged (even though it isn't yet acted on) so an auth-service/
// backend-api contract break during rollout is visible in logs rather than
// silently downgrading every request to anonymous.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := bearerExtractor.ExtractToken(r)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		var claims jwtClaims
		if _, _, err := parser.ParseUnverified(token, &claims); err != nil {
			log.Printf("authctx: decode bearer token: %v", err)
			next.ServeHTTP(w, r)
			return
		}

		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			log.Printf("authctx: claims sub is not a valid uuid: %v", err)
			next.ServeHTTP(w, r)
			return
		}

		var familyID *uuid.UUID
		if claims.FamilyID != "" {
			parsed, err := uuid.Parse(claims.FamilyID)
			if err != nil {
				log.Printf("authctx: claims family_id is not a valid uuid: %v", err)
				next.ServeHTTP(w, r)
				return
			}
			familyID = &parsed
		}

		ctx := context.WithValue(r.Context(), claimsContextKey, Claims{
			UserID:   userID,
			FamilyID: familyID,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext returns the claims Middleware decoded, if any.
func FromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(Claims)
	return claims, ok
}
