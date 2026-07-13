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

func (c *HTTPClient) UpdateCurrentBaby(ctx context.Context, input Baby) (Baby, error) {
	var baby Baby
	payload := map[string]string{
		"name":            input.Name,
		"timezone":        input.Timezone,
		"birth_date":      input.BirthDate,
		"birth_weight_kg": input.BirthWeightKg,
		"birth_length_cm": input.BirthLengthCm,
		"sex":             input.Sex,
	}
	if err := c.patchJSONDecode(ctx, "/api/v1/babies/current", payload, &baby); err != nil {
		return Baby{}, err
	}
	return baby, nil
}

func (c *HTTPClient) ArchiveCurrentBaby(ctx context.Context, confirmName string) error {
	resp, err := c.doJSONWithMethod(ctx, http.MethodDelete, "/api/v1/babies/current", map[string]string{"confirm_name": confirmName})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// ListEvents fetches the recent events for the given resource (the plural
// URL segment: "nappies", "feeds", "baths", "observations", ...) into out,
// which must be a pointer to a slice of the caller's typed view of that
// resource (e.g. *[]Nappy).
func (c *HTTPClient) ListEvents(ctx context.Context, resource, date string, out any) error {
	path := "/api/v1/babies/current/" + resource
	if date != "" {
		path += "?date=" + url.QueryEscape(date)
	}
	return c.getJSON(ctx, path, out)
}

func (c *HTTPClient) GetDailyReport(ctx context.Context, date string) (DailyReport, error) {
	path := "/api/v1/babies/current/reports/daily"
	if date != "" {
		path += "?date=" + url.QueryEscape(date)
	}

	var report DailyReport
	if err := c.getJSON(ctx, path, &report); err != nil {
		return DailyReport{}, err
	}
	return report, nil
}

// CreateEvent posts payload (form fields plus "occurred_at") to the given
// resource's create endpoint.
func (c *HTTPClient) CreateEvent(ctx context.Context, resource string, payload map[string]any) error {
	return c.postJSON(ctx, "/api/v1/babies/current/"+resource, payload)
}

// UpdateEvent edits a single event by id via the combined /events endpoint.
func (c *HTTPClient) UpdateEvent(ctx context.Context, id string, payload map[string]any) error {
	return c.patchJSON(ctx, "/api/v1/babies/current/events/"+url.PathEscape(id), payload)
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

// InviteHelper asks backend-api to invite email to help with babyID.
func (c *HTTPClient) InviteHelper(ctx context.Context, babyID, email string) error {
	return c.postJSON(ctx, "/api/v1/babies/"+url.PathEscape(babyID)+"/invite", map[string]string{"email": email})
}

func (c *HTTPClient) ListTimelineMembers(ctx context.Context) (TimelineMembersResult, error) {
	var result TimelineMembersResult
	if err := c.getJSON(ctx, "/api/v1/babies/current/members", &result); err != nil {
		return TimelineMembersResult{}, err
	}
	return result, nil
}

func (c *HTTPClient) UpdateTimelineMemberRelationship(ctx context.Context, userID, relationship string) error {
	return c.patchJSON(ctx, "/api/v1/babies/current/members/"+url.PathEscape(userID), map[string]string{"relationship": relationship})
}

func (c *HTTPClient) RemoveTimelineMember(ctx context.Context, userID string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/api/v1/babies/current/members/"+url.PathEscape(userID), nil)
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

	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, ErrForbidden
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		message := fmt.Sprintf("backend returned status %d", resp.StatusCode)
		var body struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Error != "" {
			message = body.Error
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Message: message}
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

func (c *HTTPClient) patchJSON(ctx context.Context, path string, payload any) error {
	resp, err := c.doJSONWithMethod(ctx, http.MethodPatch, path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *HTTPClient) patchJSONDecode(ctx context.Context, path string, payload any, out any) error {
	resp, err := c.doJSONWithMethod(ctx, http.MethodPatch, path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

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
	return c.doJSONWithMethod(ctx, http.MethodPost, path, payload)
}

func (c *HTTPClient) doJSONWithMethod(ctx context.Context, method, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	return c.do(ctx, method, path, bytes.NewReader(body))
}
