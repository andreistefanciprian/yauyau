package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// HTTPClient is the HTTP-backed implementation of backend-api's narrow
// auth-service dependency. backend-api owns membership decisions; auth-service
// owns the session rows that must be revoked after active access removal.
type HTTPClient struct {
	baseURL string
	secret  string
	http    *http.Client
}

func New(baseURL, secret string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		secret:  secret,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *HTTPClient) RevokeFamilyMemberSessions(ctx context.Context, userID, familyID uuid.UUID) error {
	payload := map[string]string{
		"user_id":   userID.String(),
		"family_id": familyID.String(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding revoke sessions request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/auth/sessions/revoke-family-member", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building revoke sessions request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling auth-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("auth-service returned status %d", resp.StatusCode)
	}

	return nil
}
