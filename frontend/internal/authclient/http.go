package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient is the HTTP-backed implementation of the auth-service calls
// that internal/handlers.AuthClient expects. Mirrors
// frontend/internal/backendclient/http.go's shape.
type HTTPClient struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// RequestMagicLink asks auth-service to email (or, in local dev, log to its
// own stdout) a magic link for email. The response is identical whether or
// not the email already has an account, so this never reveals which emails
// are registered.
func (c *HTTPClient) RequestMagicLink(ctx context.Context, email string) error {
	return c.postJSON(ctx, "/internal/auth/request", map[string]string{"email": email}, nil)
}

// VerifyMagicLink consumes token exactly once and returns the session it
// opened.
func (c *HTTPClient) VerifyMagicLink(ctx context.Context, token string) (VerifyResult, error) {
	var result VerifyResult
	if err := c.postJSON(ctx, "/internal/auth/verify", map[string]string{"token": token}, &result); err != nil {
		return VerifyResult{}, err
	}
	return result, nil
}

// Logout revokes sessionID. Idempotent on auth-service's side — logging out
// an already-revoked or unknown session still returns success.
func (c *HTTPClient) Logout(ctx context.Context, sessionID string) error {
	return c.postJSON(ctx, "/internal/auth/logout", map[string]string{"session_id": sessionID}, nil)
}

// do builds and executes an HTTP request against auth-service, returning an
// error for any transport failure or non-2xx response. Callers own closing
// resp.Body on success.
func (c *HTTPClient) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling auth-service: %w", err)
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("auth-service returned status %d", resp.StatusCode)
	}

	return resp, nil
}

// postJSON POSTs payload as JSON and, if out is non-nil, decodes the
// response body into it.
func (c *HTTPClient) postJSON(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}
