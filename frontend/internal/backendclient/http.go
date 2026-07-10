package backendclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient is the HTTP-backed implementation of the backend-api calls that
// internal/handlers.Backend expects.
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

// tokenContextKey is unexported so ContextWithToken is the only way to set
// it — internal/handlers' session middleware mints a fresh access token
// per request (see docs/auth-magic-link.md) and stashes it here so every
// backend-api call in that request's lifecycle picks it up automatically.
type tokenContextKey struct{}

// ContextWithToken returns a context carrying token, which do attaches as
// this request's Authorization: Bearer header.
func ContextWithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenContextKey{}, token)
}

func (c *HTTPClient) GetCurrentBaby(ctx context.Context) (Baby, error) {
	var baby Baby
	if err := c.getJSON(ctx, "/api/v1/babies/current", &baby); err != nil {
		return Baby{}, err
	}
	return baby, nil
}

// CreateBaby adds name as the caller's first baby. backend-api creates
// their family implicitly in the same call (see backend-api's CreateBaby)
// and returns it as the baby's family_id.
func (c *HTTPClient) CreateBaby(ctx context.Context, name string) (Baby, error) {
	var baby Baby
	if err := c.postJSONDecode(ctx, "/api/v1/babies", map[string]any{"name": name}, &baby); err != nil {
		return Baby{}, err
	}
	return baby, nil
}

// ListEvents fetches the recent events for the given resource (the plural
// URL segment: "nappies", "feeds", "baths", "observations", ...) into out,
// which must be a pointer to a slice of the caller's typed view of that
// resource (e.g. *[]Nappy).
func (c *HTTPClient) ListEvents(ctx context.Context, resource string, out any) error {
	return c.getJSON(ctx, "/api/v1/babies/current/"+resource, out)
}

// CreateEvent posts payload (form fields plus "occurred_at") to the given
// resource's create endpoint.
func (c *HTTPClient) CreateEvent(ctx context.Context, resource string, payload map[string]any) error {
	return c.postJSON(ctx, "/api/v1/babies/current/"+resource, payload)
}

// DeleteEvent removes a single event by id via the combined /events
// endpoint, regardless of which resource it was created under.
func (c *HTTPClient) DeleteEvent(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/api/v1/babies/current/events/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// do builds and executes an HTTP request against backend-api, returning an
// error for any transport failure or non-2xx response. Callers own closing
// resp.Body on success.
func (c *HTTPClient) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if token, ok := ctx.Value(tokenContextKey{}).(string); ok {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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

func (c *HTTPClient) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

// postJSON POSTs payload as JSON and discards the response body.
func (c *HTTPClient) postJSON(ctx context.Context, path string, payload any) error {
	resp, err := c.doJSON(ctx, path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// postJSONDecode POSTs payload as JSON and decodes the response body into
// out.
func (c *HTTPClient) postJSONDecode(ctx context.Context, path string, payload any, out any) error {
	resp, err := c.doJSON(ctx, path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

func (c *HTTPClient) doJSON(ctx context.Context, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	return c.do(ctx, http.MethodPost, path, bytes.NewReader(body))
}
