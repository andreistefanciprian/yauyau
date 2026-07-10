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
	form.Set("html", htmlMagicLink(email, link))

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

func htmlMagicLink(email, link string) string {
	escapedEmail := htmlEscape(email)
	escaped := htmlEscape(link)
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Sign in to Yauli</title>
</head>
<body style="margin:0;padding:0;background:#FCFBF8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#334155;">
  <table width="100%" cellpadding="0" cellspacing="0" style="background:#FCFBF8;">
    <tr>
      <td align="center" style="padding:48px 24px;">
        <table width="100%" cellpadding="0" cellspacing="0" style="max-width:480px;">

          <tr>
            <td style="background:#FFFFFF;border:1px solid #E6EEF0;border-radius:14px;padding:40px 36px;text-align:center;box-shadow:0 1px 3px rgba(0,0,0,0.04);">
              <p style="margin:0 0 8px;font-size:1.35rem;font-weight:700;color:#56789D;">Yauli</p>
              <p style="margin:0 0 24px;font-size:0.96rem;font-weight:600;color:#56789D;line-height:1.55;">
                Your parenting companion, from day one.
              </p>
              <p style="margin:0 0 30px;font-size:0.92rem;color:#64748B;line-height:1.65;">
                Click the button below to sign in securely. This link expires in 15 minutes and can only be used once.
              </p>
              <a href="` + escaped + `"
                 style="display:inline-block;padding:14px 32px;background:#74C7C3;color:#ffffff;font-size:0.95rem;font-weight:700;border-radius:999px;text-decoration:none;box-shadow:0 4px 14px rgba(116,199,195,0.32);">
                Open Yauli
              </a>
              <p style="margin:28px 0 0;font-size:0.8rem;color:#94A3B8;line-height:1.6;">
                If the button does not work, copy and paste this link into your browser:<br>
                <a href="` + escaped + `" style="color:#F28B72;text-decoration:none;word-break:break-all;">` + escaped + `</a>
              </p>
            </td>
          </tr>

          <tr>
            <td align="center" style="padding-top:24px;">
              <p style="margin:0;font-size:0.78rem;color:#94A3B8;line-height:1.6;">
                If you did not request this email, you can safely ignore it.<br>
                This link was requested for <strong style="color:#64748B;">` + escapedEmail + `</strong>.
              </p>
            </td>
          </tr>

        </table>
      </td>
    </tr>
  </table>
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
