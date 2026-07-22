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
	return m.send(ctx, email, "Sign in to Yauli", textMagicLink(link), htmlMagicLink(email, link))
}

func (m *Mailgun) SendInviteMagicLink(ctx context.Context, email, babyName, link string) error {
	return m.send(ctx, email, "You're invited to join Yauli", textInviteMagicLink(babyName, link), htmlInviteMagicLink(email, babyName, link))
}

func (m *Mailgun) send(ctx context.Context, email, subject, textBody, htmlBody string) error {
	form := url.Values{}
	form.Set("from", m.from)
	form.Set("to", email)
	form.Set("subject", subject)
	form.Set("text", textBody)
	form.Set("html", htmlBody)

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

func textInviteMagicLink(babyName, link string) string {
	return "You have been invited to help care for " + babyName + " on Yauli.\n\nIf you already created a baby in Yauli, delete that timeline from Baby settings before using this invite.\n\nOpen Yauli:\n\n" + link + "\n\nThis link expires in 15 minutes. If you did not expect this invitation, you can ignore this email."
}

func htmlMagicLink(email, link string) string {
	escapedEmail := htmlEscape(email)
	escaped := htmlEscape(link)
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>Sign in to Yauli</title>
<style>
  body, table, td { font-family: Arial, Helvetica, sans-serif; }
  a { text-decoration: none; }
</style>
</head>
<body style="margin:0; padding:0; background-color:#FAF6F1;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#FAF6F1;">
    <tr>
      <td align="center" style="padding:40px 16px;">
        <table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="max-width:600px; width:100%;">
          <tr>
            <td align="center" style="padding-bottom:24px;">
              <span style="font-family:Arial, Helvetica, sans-serif; font-size:26px; font-weight:bold; color:#3D7A9C;">Yau<span style="color:#E2694A;">li</span></span>
            </td>
          </tr>

          <tr>
            <td style="background-color:#FFFDFA; border:1px solid #EDE2D6; border-radius:20px; padding:44px 40px;" bgcolor="#FFFDFA">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td align="center" style="padding-bottom:12px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:15px; font-weight:bold; color:#3D6D91; mso-line-height-rule:exactly; line-height:22px;">Your parenting companion, from day one.</p>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding-bottom:32px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:15px; color:#5C6B7A; mso-line-height-rule:exactly; line-height:24px;">Click the button below to sign in securely.<br>This link expires in 15 minutes and can only be used once.</p>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding-bottom:32px;">
                    <table role="presentation" cellpadding="0" cellspacing="0" border="0">
                      <tr>
                        <td align="center" bgcolor="#5FBCB0" style="border-radius:999px;">
                          <a href="` + escaped + `" target="_blank" style="display:block; padding:16px 40px; font-family:Arial, Helvetica, sans-serif; font-size:16px; font-weight:bold; color:#FFFFFF; border-radius:999px;">Open Yauli</a>
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding-bottom:6px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#9C9184; mso-line-height-rule:exactly; line-height:20px;">If the button does not work, copy and paste this link into your browser:</p>
                  </td>
                </tr>
                <tr>
                  <td align="center">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:13px; word-break:break-all;"><a href="` + escaped + `" style="color:#B5652F;">` + escaped + `</a></p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <tr>
            <td align="center" style="padding-top:28px;">
              <p style="margin:0 0 6px; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#9C9184; mso-line-height-rule:exactly; line-height:20px;">If you did not request this email, you can safely ignore it.</p>
              <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#9C9184; mso-line-height-rule:exactly; line-height:20px;">This link was requested for <a href="mailto:` + escapedEmail + `" style="color:#3D7A9C; font-weight:bold;">` + escapedEmail + `</a>.</p>
            </td>
          </tr>

          <tr>
            <td align="center" style="padding-top:32px;">
              <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:12px; color:#B7AC9C;">Yauli &middot; getyauli.com</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}

func htmlInviteMagicLink(email, babyName, link string) string {
	escapedEmail := htmlEscape(email)
	escapedBabyName := htmlEscape(babyName)
	escaped := htmlEscape(link)
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>You're invited to join Yauli</title>
<style>
  body, table, td { font-family: Arial, Helvetica, sans-serif; }
  a { text-decoration: none; }
</style>
</head>
<body style="margin:0; padding:0; background-color:#FAF6F1;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#FAF6F1;">
    <tr>
      <td align="center" style="padding:40px 16px;">
        <table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="max-width:600px; width:100%;">
          <tr>
            <td align="center" style="padding-bottom:24px;">
              <span style="font-family:Arial, Helvetica, sans-serif; font-size:26px; font-weight:bold; color:#3D7A9C;">Yau<span style="color:#E2694A;">li</span></span>
            </td>
          </tr>

          <tr>
            <td style="background-color:#FFFDFA; border:1px solid #EDE2D6; border-radius:20px; padding:44px 40px;" bgcolor="#FFFDFA">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td align="center" style="padding-bottom:12px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:15px; font-weight:bold; color:#3D6D91; mso-line-height-rule:exactly; line-height:22px;">Your parenting companion, from day one.</p>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding-bottom:12px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:17px; font-weight:bold; color:#3A332C; mso-line-height-rule:exactly; line-height:24px;">You've been invited to help care for ` + escapedBabyName + `.</p>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding-bottom:24px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:15px; color:#5C6B7A; mso-line-height-rule:exactly; line-height:24px;">Open Yauli with the secure link below to join ` + escapedBabyName + `'s timeline.<br>This link expires in 15 minutes and can only be used once.</p>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding-bottom:28px;">
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" bgcolor="#F3ECE1" style="background-color:#F3ECE1; border-radius:12px;">
                      <tr>
                        <td style="padding:14px 16px;">
                          <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#6B7280; mso-line-height-rule:exactly; line-height:20px;">Already created a baby in Yauli? Delete that timeline from Baby settings first, then use this invite to join ` + escapedBabyName + `'s timeline.</p>
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding-bottom:32px;">
                    <table role="presentation" cellpadding="0" cellspacing="0" border="0">
                      <tr>
                        <td align="center" bgcolor="#5FBCB0" style="border-radius:999px;">
                          <a href="` + escaped + `" target="_blank" style="display:block; padding:16px 40px; font-family:Arial, Helvetica, sans-serif; font-size:16px; font-weight:bold; color:#FFFFFF; border-radius:999px;">Join on Yauli</a>
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding-bottom:6px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#9C9184; mso-line-height-rule:exactly; line-height:20px;">If the button does not work, copy and paste this link into your browser:</p>
                  </td>
                </tr>
                <tr>
                  <td align="center">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:13px; word-break:break-all;"><a href="` + escaped + `" style="color:#B5652F;">` + escaped + `</a></p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <tr>
            <td align="center" style="padding-top:28px;">
              <p style="margin:0 0 6px; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#9C9184; mso-line-height-rule:exactly; line-height:20px;">If you did not expect this invitation, you can safely ignore it.</p>
              <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#9C9184; mso-line-height-rule:exactly; line-height:20px;">This invitation was sent to <a href="mailto:` + escapedEmail + `" style="color:#3D7A9C; font-weight:bold;">` + escapedEmail + `</a>.</p>
            </td>
          </tr>

          <tr>
            <td align="center" style="padding-top:32px;">
              <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:12px; color:#B7AC9C;">Yauli &middot; getyauli.com</p>
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
