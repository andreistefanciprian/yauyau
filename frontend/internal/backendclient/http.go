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

func (c *HTTPClient) GetCurrentBaby(ctx context.Context) (Baby, error) {
	var baby Baby
	if err := c.getJSON(ctx, "/api/v1/babies/current", &baby); err != nil {
		return Baby{}, err
	}
	return baby, nil
}

func (c *HTTPClient) ListNappies(ctx context.Context) ([]Nappy, error) {
	var nappies []Nappy
	if err := c.getJSON(ctx, "/api/v1/babies/current/nappies", &nappies); err != nil {
		return nil, err
	}
	return nappies, nil
}

func (c *HTTPClient) CreateNappy(ctx context.Context, kind, colour string, occurredAt time.Time) error {
	return c.postJSON(ctx, "/api/v1/babies/current/nappies", map[string]string{
		"kind":        kind,
		"colour":      colour,
		"occurred_at": occurredAt.Format(time.RFC3339),
	})
}

func (c *HTTPClient) ListFeeds(ctx context.Context) ([]Feed, error) {
	var feeds []Feed
	if err := c.getJSON(ctx, "/api/v1/babies/current/feeds", &feeds); err != nil {
		return nil, err
	}
	return feeds, nil
}

func (c *HTTPClient) CreateFeed(ctx context.Context, feedType string, amountMl, durationMinutes *int, occurredAt time.Time) error {
	return c.postJSON(ctx, "/api/v1/babies/current/feeds", map[string]any{
		"type":             feedType,
		"amount_ml":        amountMl,
		"duration_minutes": durationMinutes,
		"occurred_at":      occurredAt.Format(time.RFC3339),
	})
}

func (c *HTTPClient) ListBaths(ctx context.Context) ([]Bath, error) {
	var baths []Bath
	if err := c.getJSON(ctx, "/api/v1/babies/current/baths", &baths); err != nil {
		return nil, err
	}
	return baths, nil
}

func (c *HTTPClient) CreateBath(ctx context.Context, bathType, notes string, durationMinutes *int, occurredAt time.Time) error {
	return c.postJSON(ctx, "/api/v1/babies/current/baths", map[string]any{
		"type":             bathType,
		"notes":            notes,
		"duration_minutes": durationMinutes,
		"occurred_at":      occurredAt.Format(time.RFC3339),
	})
}

// do builds and executes an HTTP request against backend-api, returning an
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

func (c *HTTPClient) postJSON(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
