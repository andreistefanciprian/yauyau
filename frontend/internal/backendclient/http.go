package backendclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	payload, err := json.Marshal(map[string]string{
		"kind":        kind,
		"colour":      colour,
		"occurred_at": occurredAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/babies/current/nappies", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling backend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("backend returned status %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling backend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("backend returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}
