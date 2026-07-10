package backendclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// User mirrors the relevant fields of backend-api's store.User — this
// package doesn't own that type, so it keeps its own minimal view of the
// JSON shape rather than importing backend-api as a dependency.
type User struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

// FamilyMembership mirrors backend-api's store.FamilyMembership response
// shape from GET /internal/family-membership.
type FamilyMembership struct {
	Found    bool       `json:"found"`
	FamilyID *uuid.UUID `json:"family_id,omitempty"`
	Role     string     `json:"role,omitempty"`
	Status   string     `json:"status,omitempty"`
}

// HTTPClient is the HTTP-backed implementation of the backend-api internal
// API calls that internal/handlers.BackendClient expects. It mirrors
// frontend/internal/backendclient/http.go's shape, with the addition of the
// X-Internal-Secret header every internal-route call must carry.
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

// UpsertUser resolves (creating if necessary) the user for email, called
// right before a magic link is issued so the link's magic_links row has a
// user_id to reference.
func (c *HTTPClient) UpsertUser(ctx context.Context, email string) (User, error) {
	body, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return User{}, fmt.Errorf("encoding request: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPost, "/internal/users", bytes.NewReader(body))
	if err != nil {
		return User{}, err
	}
	defer resp.Body.Close()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return User{}, fmt.Errorf("decoding response: %w", err)
	}

	return user, nil
}

// GetFamilyMembership returns userID's family membership, if any. When
// activateIfInvited is true and the membership is a pending invite,
// backend-api activates it before responding — used by the verify flow to
// resolve-and-activate an invited user's membership in one call.
func (c *HTTPClient) GetFamilyMembership(ctx context.Context, userID uuid.UUID, activateIfInvited bool) (FamilyMembership, error) {
	query := url.Values{}
	query.Set("user_id", userID.String())
	if activateIfInvited {
		query.Set("activate_if_invited", "true")
	}

	resp, err := c.do(ctx, http.MethodGet, "/internal/family-membership?"+query.Encode(), nil)
	if err != nil {
		return FamilyMembership{}, err
	}
	defer resp.Body.Close()

	var membership FamilyMembership
	if err := json.NewDecoder(resp.Body).Decode(&membership); err != nil {
		return FamilyMembership{}, fmt.Errorf("decoding response: %w", err)
	}

	return membership, nil
}

// do builds and executes an HTTP request against backend-api's internal
// API, attaching the shared secret every /internal route requires and
// returning an error for any transport failure or non-2xx response.
// Callers own closing resp.Body on success.
func (c *HTTPClient) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("X-Internal-Secret", c.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling backend: %w", err)
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("backend returned status %d", resp.StatusCode)
	}

	return resp, nil
}
