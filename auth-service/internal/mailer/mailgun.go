package mailer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultMailgunBaseURL = "https://api.mailgun.net"

// Mailgun sends production magic-link emails using Mailgun's HTTP API.
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

func (m *Mailgun) SendMagicLink(ctx context.Context, email, link string) error {
	form := url.Values{}
	form.Set("from", m.from)
	form.Set("to", email)
	form.Set("subject", "Sign in to Yauli")
	form.Set("text", textMagicLink(link))
	form.Set("html", htmlMagicLink(link))

	endpoint := fmt.Sprintf("%s/v3/%s/messages", m.baseURL, url.PathEscape(m.domain))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("building mailgun request: %w", err)
	}
	req.SetBasicAuth("api", m.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling mailgun: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("mailgun returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func textMagicLink(link string) string {
	return "Sign in to Yauli:\n\n" + link + "\n\nThis link expires in 15 minutes. If you did not request it, you can ignore this email."
}

func htmlMagicLink(link string) string {
	escaped := htmlEscape(link)
	return `<!doctype html>
<html>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; line-height: 1.5;">
  <p>Sign in to Yauli:</p>
  <p><a href="` + escaped + `">Open Yauli</a></p>
  <p>This link expires in 15 minutes. If you did not request it, you can ignore this email.</p>
</body>
</html>`
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}
