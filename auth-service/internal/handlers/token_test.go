package handlers

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestSignAccessToken_ClaimShapeMatchesBackendAPIContract locks down the
// raw wire shape signAccessToken produces: "sub" (the standard registered
// claim) and "family_id" (a plain string, omitted entirely rather than
// null when absent). backend-api's authctx.go decoder
// (backend-api/internal/authctx/authctx.go) expects exactly this shape but
// has no compiler-checked link to this package — this test is the
// closest thing to a contract test across that boundary without a shared
// module. If this test's assertions ever need to change, authctx.go's
// decoder must change in the same PR, or backend-api starts silently
// rejecting every access token auth-service mints.
func TestSignAccessToken_ClaimShapeMatchesBackendAPIContract(t *testing.T) {
	userID := uuid.New()
	familyID := uuid.New()

	token, err := signAccessToken(userID, &familyID, []byte("test-signing-key"))
	if err != nil {
		t.Fatalf("signAccessToken: %v", err)
	}

	claims := decodeClaims(t, token)

	if claims["sub"] != userID.String() {
		t.Errorf(`claims["sub"] = %v, want %q`, claims["sub"], userID.String())
	}
	if claims["family_id"] != familyID.String() {
		t.Errorf(`claims["family_id"] = %v, want %q`, claims["family_id"], familyID.String())
	}
	if _, ok := claims["exp"]; !ok {
		t.Error(`claims["exp"] missing`)
	}
}

// TestSignAccessToken_NilFamilyIDOmitsClaimEntirely guards the specific
// behavior authctx.go's Middleware relies on for a brand-new, family-less
// session: no family_id claim at all (not an explicit null, not an empty
// string), so authctx.Claims.FamilyID decodes to nil.
func TestSignAccessToken_NilFamilyIDOmitsClaimEntirely(t *testing.T) {
	userID := uuid.New()

	token, err := signAccessToken(userID, nil, []byte("test-signing-key"))
	if err != nil {
		t.Fatalf("signAccessToken: %v", err)
	}

	claims := decodeClaims(t, token)

	if _, ok := claims["family_id"]; ok {
		t.Errorf(`claims["family_id"] = %v, want key absent entirely`, claims["family_id"])
	}
}

// decodeClaims base64-decodes a JWT's payload segment directly, rather
// than using golang-jwt's parser, so the assertion is against the actual
// bytes on the wire — exactly what backend-api's decoder receives — not
// against a round-trip through this same package's own struct definition.
func decodeClaims(t *testing.T, token string) map[string]any {
	t.Helper()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload segment: %v", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}

	return claims
}
