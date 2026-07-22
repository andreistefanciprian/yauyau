package reportemail

import (
	"fmt"
	"strings"
	"time"
)

const reportEncouragement = "You're doing great. You've got this."

// cardMetricColors are the label tints for the KPI card's columns, cycling
// if there are ever more metrics than colors. They mirror the event-type
// accent hues used elsewhere in Yauli's brand (web app card tints), adapted
// to hold up against the card's light blue background.
var cardMetricColors = []string{"#8F5A2B", "#6E4E96", "#B5652F", "#9C7A4E"}

func textBody(report Report) string {
	var b strings.Builder
	b.WriteString(report.Output.Title)
	b.WriteString("\n\n")
	b.WriteString(report.Output.Summary)
	b.WriteString("\n")

	if len(report.Card) > 0 {
		b.WriteString("\n")
		parts := make([]string, 0, len(report.Card))
		for _, metric := range report.Card {
			part := fmt.Sprintf("%s: %d", metric.Label, metric.Count)
			if metric.Detail != "" {
				part += fmt.Sprintf(" (%s)", metric.Detail)
			}
			parts = append(parts, part)
		}
		b.WriteString(strings.Join(parts, " · "))
		b.WriteString("\n")
	}

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
	b.WriteString(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>`)
	b.WriteString(htmlEscape(subject(report)))
	b.WriteString(`</title>
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
            <td style="background-color:#FFFDFA; border:1px solid #EDE2D6; border-radius:20px; padding:40px 36px;" bgcolor="#FFFDFA">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td style="padding-bottom:6px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:12px; font-weight:bold; letter-spacing:0.06em; color:#5FBCB0; text-transform:uppercase; mso-line-height-rule:exactly; line-height:18px;">`)
	b.WriteString(htmlEscape(report.ReportType))
	b.WriteString(` report</p>
                  </td>
                </tr>
                <tr>
                  <td style="padding-bottom:20px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:19px; font-weight:bold; color:#2C5C77; mso-line-height-rule:exactly; line-height:26px;">`)
	b.WriteString(htmlEscape(report.Output.Title))
	b.WriteString(`</p>
                  </td>
                </tr>`)

	writeHTMLCard(&b, report)

	b.WriteString(`
                <tr>
                  <td style="padding-bottom:24px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:15px; color:#3A332C; mso-line-height-rule:exactly; line-height:24px;">`)
	b.WriteString(htmlEscape(report.Output.Summary))
	b.WriteString(`</p>
                  </td>
                </tr>`)

	writeHTMLList(&b, "Highlights", report.Output.Highlights)
	writeHTMLList(&b, "Patterns", report.Output.Patterns)
	writeHTMLList(&b, "Comparison", report.Output.Comparison)
	writeHTMLList(&b, "Caveats", report.Output.Caveats)

	b.WriteString(`
                <tr>
                  <td style="background-color:#DCEEEC; border-radius:14px; padding:20px 22px;" bgcolor="#DCEEEC">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:15px; font-weight:bold; color:#2C6E80; mso-line-height-rule:exactly; line-height:22px;">`)
	b.WriteString(htmlEscape(reportEncouragement))
	b.WriteString(`</p>
                  </td>
                </tr>
                <tr>
                  <td style="padding-top:24px;">
                    <p style="margin:0; padding-top:18px; border-top:1px solid #EDE2D6; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#9C9184; mso-line-height-rule:exactly; line-height:20px;">
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
          <tr>
            <td align="center" style="padding-top:28px;">
              <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:12px; color:#B7AC9C;">Yauli &middot; getyauli.com</p>
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

// writeHTMLCard renders the KPI summary card (feeds/sleep/pump/nappies, or
// whatever metrics the caller supplied) — the same deterministic counts the
// web app's daily report card shows. Omitted entirely when report.Card is
// empty, e.g. if the event counts could not be loaded for this send.
func writeHTMLCard(b *strings.Builder, report Report) {
	if len(report.Card) == 0 {
		return
	}

	b.WriteString(`
                <tr>
                  <td style="padding-bottom:28px;">
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" bgcolor="#E4EEF6" style="background-color:#E4EEF6; border-radius:18px;">
                      <tr>
                        <td style="padding:24px 26px 4px;">
                          <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:17px; font-weight:bold; color:#2C5C77; mso-line-height-rule:exactly; line-height:22px;">`)
	b.WriteString(htmlEscape(reportDateHeading(report)))
	b.WriteString(`</p>
                        </td>
                      </tr>
                      <tr>
                        <td style="padding:12px 26px 24px;">
                          <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
                            <tr>`)

	columnWidth := 100 / len(report.Card)
	for i, metric := range report.Card {
		color := cardMetricColors[i%len(cardMetricColors)]
		padding := "padding-left:10px; padding-right:10px;"
		if i == 0 {
			padding = "padding-right:10px;"
		} else if i == len(report.Card)-1 {
			padding = "padding-left:10px;"
		}
		b.WriteString(fmt.Sprintf(`
                              <td width="%d%%" style="%s">
                                <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:30px; font-weight:bold; color:#3A332C; mso-line-height-rule:exactly; line-height:32px;">%d</p>
                                <p style="margin:6px 0 0; font-family:Arial, Helvetica, sans-serif; font-size:11px; font-weight:bold; letter-spacing:0.05em; text-transform:uppercase; color:%s;">%s</p>`,
			columnWidth, padding, metric.Count, color, htmlEscape(metric.Label)))
		for _, detail := range strings.Split(metric.Detail, " · ") {
			if detail == "" {
				continue
			}
			b.WriteString(`
                                <p style="margin:2px 0 0; font-family:Arial, Helvetica, sans-serif; font-size:12px; color:#6B7280;">`)
			b.WriteString(htmlEscape(detail))
			b.WriteString(`</p>`)
		}
		b.WriteString(`
                              </td>`)
		if i < len(report.Card)-1 {
			b.WriteString(`
                              <td width="1" style="background-color:#C7D3D9; font-size:1px; line-height:1px;">&nbsp;</td>`)
		}
	}

	b.WriteString(`
                            </tr>
                          </table>
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>`)
}

// reportDateHeading formats the report's start date the way the KPI card
// mockup does ("Sunday, July 20"), falling back to the raw string if it
// cannot be parsed (StartDate is always a plain YYYY-MM-DD outside tests).
func reportDateHeading(report Report) string {
	parsed, err := time.Parse("2006-01-02", report.StartDate)
	if err != nil {
		return report.StartDate
	}
	return parsed.Format("Monday, January 2")
}

func writeHTMLList(b *strings.Builder, heading string, items []string) {
	nonEmpty := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			nonEmpty = append(nonEmpty, item)
		}
	}
	if len(nonEmpty) == 0 {
		return
	}

	b.WriteString(`
                <tr>
                  <td style="padding-bottom:10px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:16px; font-weight:bold; color:#3D6D91; mso-line-height-rule:exactly; line-height:22px;">`)
	b.WriteString(htmlEscape(heading))
	b.WriteString(`</p>
                  </td>
                </tr>
                <tr>
                  <td style="padding-bottom:24px;">
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">`)
	for i, item := range nonEmpty {
		paddingBottom := "6px"
		if i == len(nonEmpty)-1 {
			paddingBottom = "0"
		}
		b.WriteString(`
                      <tr>
                        <td width="16" valign="top" style="font-family:Arial, Helvetica, sans-serif; font-size:15px; color:#3A332C; line-height:23px;">&bull;</td>
                        <td style="font-family:Arial, Helvetica, sans-serif; font-size:15px; color:#3A332C; mso-line-height-rule:exactly; line-height:23px; padding-bottom:`)
		b.WriteString(paddingBottom)
		b.WriteString(`;">`)
		b.WriteString(htmlEscape(item))
		b.WriteString(`</td>
                      </tr>`)
	}
	b.WriteString(`
                    </table>
                  </td>
                </tr>`)
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
