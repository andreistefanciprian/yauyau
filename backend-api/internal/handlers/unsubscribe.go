package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

// signUnsubscribeToken and verifyUnsubscribeToken implement a stateless,
// HMAC-signed unsubscribe link: no DB row, no expiry — unsubscribe links
// must keep working indefinitely. The signature is the only thing standing
// between this endpoint and an authenticated session.
func signUnsubscribeToken(secret string, familyID, userID uuid.UUID) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(familyID.String() + ":" + userID.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyUnsubscribeToken(secret string, familyID, userID uuid.UUID, sig string) bool {
	want := signUnsubscribeToken(secret, familyID, userID)
	return hmac.Equal([]byte(want), []byte(sig))
}

// unsubscribeURL builds the link embedded in report emails' List-Unsubscribe
// header. It points at the public frontend — backend-api itself isn't
// reachable from the internet — whose /unsubscribe route is a thin
// pass-through to UnsubscribeReportEmail below. Returns "" if either input
// needed to build a valid link is missing, so callers can skip the header
// entirely rather than send a broken link.
func unsubscribeURL(frontendURL, secret string, familyID, userID uuid.UUID) string {
	if frontendURL == "" || secret == "" {
		return ""
	}
	sig := signUnsubscribeToken(secret, familyID, userID)
	return fmt.Sprintf("%s/unsubscribe?family=%s&user=%s&sig=%s", strings.TrimRight(frontendURL, "/"), familyID, userID, sig)
}

type unsubscribeRequest struct {
	FamilyID uuid.UUID `json:"family_id"`
	UserID   uuid.UUID `json:"user_id"`
	Sig      string    `json:"sig"`
}

// UnsubscribeReportEmail turns off daily report emails for one recipient in
// one family. It's reached via frontend's public, unauthenticated
// /unsubscribe pass-through — there's no user session here, so the HMAC
// signature is the entire trust boundary and must be checked before
// touching the store.
func (h *Handlers) UnsubscribeReportEmail(w http.ResponseWriter, r *http.Request) {
	var req unsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if h.UnsubscribeSecret == "" || !verifyUnsubscribeToken(h.UnsubscribeSecret, req.FamilyID, req.UserID, req.Sig) {
		writeError(w, http.StatusBadRequest, "invalid unsubscribe link")
		return
	}

	if _, err := h.FamilyStore.UpdateDailyReportEmailPreference(r.Context(), req.FamilyID, req.UserID, false); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Already inactive/removed: nothing left to unsubscribe from,
			// but that's still a success from the recipient's perspective.
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		log.Printf("unsubscribe report email: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update report email preference")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
