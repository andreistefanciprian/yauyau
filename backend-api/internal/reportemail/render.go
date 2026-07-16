package reportemail

import (
	"strings"
)

const reportEncouragement = "You're doing great. You've got this."

func textBody(report Report) string {
	var b strings.Builder
	b.WriteString(report.Output.Title)
	b.WriteString("\n\n")
	b.WriteString(report.Output.Summary)
	b.WriteString("\n")

	writeTextList(&b, "Highlights", report.Output.Highlights)
	writeTextList(&b, "Patterns", report.Output.Patterns)
	writeTextList(&b, "Comparison", report.Output.Comparison)
	writeTextList(&b, "Caveats", report.Output.Caveats)

	b.WriteString("\n")
	b.WriteString(reportEncouragement)
	b.WriteString("\n")

	b.WriteString("\nReport window: ")
	b.WriteString(report.StartDate)
	if report.EndDate != "" && report.EndDate != report.StartDate {
		b.WriteString(" to ")
		b.WriteString(report.EndDate)
	}
	b.WriteString("\n")

	return b.String()
}

func writeTextList(b *strings.Builder, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString("\n")
	b.WriteString(heading)
	b.WriteString(":\n")
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
}

func htmlBody(report Report) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>`)
	b.WriteString(htmlEscape(subject(report)))
	b.WriteString(`</title>
</head>
<body style="margin:0;padding:0;background:#FCFBF8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#334155;">
  <table width="100%" cellpadding="0" cellspacing="0" style="background:#FCFBF8;">
    <tr>
      <td align="center" style="padding:40px 20px;">
        <table width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;">
          <tr>
            <td style="padding:0 0 18px;text-align:center;">
              <p style="margin:0;font-size:1.25rem;font-weight:700;color:#56789D;">Yauli</p>
            </td>
          </tr>
          <tr>
            <td style="background:#FFFFFF;border:1px solid #E6EEF0;border-radius:14px;padding:32px;box-shadow:0 1px 3px rgba(0,0,0,0.04);">
              <p style="margin:0 0 8px;font-size:0.82rem;font-weight:700;color:#74C7C3;text-transform:uppercase;letter-spacing:0;">`)
	b.WriteString(htmlEscape(report.ReportType))
	b.WriteString(` report</p>
              <h1 style="margin:0 0 18px;font-size:1.45rem;line-height:1.3;color:#334155;">`)
	b.WriteString(htmlEscape(report.Output.Title))
	b.WriteString(`</h1>
              <p style="margin:0 0 22px;font-size:0.96rem;line-height:1.65;color:#475569;">`)
	b.WriteString(htmlEscape(report.Output.Summary))
	b.WriteString(`</p>`)

	writeHTMLList(&b, "Highlights", report.Output.Highlights)
	writeHTMLList(&b, "Patterns", report.Output.Patterns)
	writeHTMLList(&b, "Comparison", report.Output.Comparison)
	writeHTMLList(&b, "Caveats", report.Output.Caveats)

	b.WriteString(`
              <p style="margin:24px 0 0;font-size:0.96rem;line-height:1.6;color:#475569;font-weight:700;">`)
	b.WriteString(htmlEscape(reportEncouragement))
	b.WriteString(`</p>
              <p style="margin:26px 0 0;padding-top:18px;border-top:1px solid #E6EEF0;font-size:0.8rem;line-height:1.6;color:#94A3B8;">
                Report window: `)
	b.WriteString(htmlEscape(report.StartDate))
	if report.EndDate != "" && report.EndDate != report.StartDate {
		b.WriteString(` to `)
		b.WriteString(htmlEscape(report.EndDate))
	}
	b.WriteString(`<br>
                Sent to `)
	b.WriteString(htmlEscape(report.RecipientEmail))
	b.WriteString(`
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`)
	return b.String()
}

func writeHTMLList(b *strings.Builder, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(`
              <h2 style="margin:22px 0 10px;font-size:0.96rem;color:#56789D;">`)
	b.WriteString(htmlEscape(heading))
	b.WriteString(`</h2>
              <ul style="margin:0;padding-left:20px;color:#475569;font-size:0.92rem;line-height:1.65;">`)
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		b.WriteString(`<li>`)
		b.WriteString(htmlEscape(item))
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)
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
