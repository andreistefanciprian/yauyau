package reportemail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultMailgunBaseURL = "https://api.mailgun.net"

// Mailgun sends scheduled report emails through Mailgun's HTTP API. Backend
// owns report delivery because it owns report generation, cache, and delivery
// attempt state.
type Mailgun struct {
	apiKey  string
	domain  string
	from    string
	baseURL string
	http    *http.Client
}

func NewMailgun(apiKey, domain, from, baseURL string) *Mailgun {
	if baseURL == "" {
		baseURL = defaultMailgunBaseURL
	}
	return &Mailgun{
		apiKey:  apiKey,
		domain:  domain,
		from:    from,
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (m *Mailgun) SendReportEmail(ctx context.Context, report Report) (string, error) {
	return m.send(ctx, report.RecipientEmail, subject(report), textBody(report), htmlBody(report))
}

func (m *Mailgun) send(ctx context.Context, recipient, subject, textBody, htmlBody string) (string, error) {
	form := url.Values{}
	form.Set("from", m.from)
	form.Set("to", recipient)
	form.Set("subject", subject)
	form.Set("text", textBody)
	form.Set("html", htmlBody)

	endpoint := fmt.Sprintf("%s/v3/%s/messages", m.baseURL, url.PathEscape(m.domain))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building mailgun request: %w", err)
	}
	req.SetBasicAuth("api", m.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling mailgun: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr != nil {
		return "", fmt.Errorf("reading mailgun response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("mailgun returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &parsed); err != nil {
			return "", fmt.Errorf("decoding mailgun response: %w", err)
		}
	}

	return parsed.ID, nil
}
