package reportemail

import (
	"fmt"
	"math"
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
                </tr>`)

	writeHTMLTrend(&b, report)

	b.WriteString(`
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
                              <td width="%d%%" valign="top" style="%s vertical-align:top;">
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

// trendMultiBarTrackWidth is the pixel width of a fully-scaled bar in the
// stacked Feeds/Pump charts, matching the design mockup's bar tracks. It is
// narrower than the single-series charts to leave room for wider value text
// (durations, mL amounts) in trendMultiValueWidth.
const trendMultiBarTrackWidth = 190

// formatTrendDurationMinutes renders a duration the way the trend charts'
// design mockup does: "1h 44m" once there's at least an hour, "16 min"
// otherwise. This is deliberately separate from the web app / KPI card's own
// duration formatting (report.go's formatCompactDurationMinutes) — the two
// are allowed to diverge since they serve different, independently designed
// surfaces.
func formatTrendDurationMinutes(minutes int) string {
	hours := minutes / 60
	remaining := minutes % 60
	if hours == 0 {
		return fmt.Sprintf("%d min", remaining)
	}
	if remaining == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, remaining)
}

// trendChartSpec describes one of the "Last 7 days" single-bar charts (Sleep,
// Nappies): how to read its value off a TrendDay, how to format that value,
// and the bar's visual proportions.
type trendChartSpec struct {
	Heading       string
	LabelColor    string
	BarColor      string
	TrackWidth    int
	BarHeight     int
	ValueWidth    int
	ValueFontSize string
	Nowrap        bool
	Value         func(TrendDay) float64
	Format        func(float64) string
}

// trendSeriesSpec is one stacked bar series within a multi-series trend
// chart (e.g. Feeds' Count/Duration/Bottle mL, or Pump's mL/Duration).
type trendSeriesSpec struct {
	Legend string
	Color  string
	Value  func(TrendDay) float64
	Format func(float64) string
}

// trendMultiChartSpec describes one of the "Last 7 days" stacked-bar charts
// (Feeds, Pump): several series sharing one day label and one legend.
type trendMultiChartSpec struct {
	Heading      string
	HeadingColor string
	ValueWidth   int
	Series       []trendSeriesSpec
}

// writeHTMLTrend renders the "Last 7 days" section: single bar charts for
// sleep and nappies, and stacked multi-series bar charts for feeds and pump
// (count/duration/bottle mL, and mL/duration respectively). Each series is
// scaled independently against its own weekly max. Omitted entirely when
// report.Trend is empty.
func writeHTMLTrend(b *strings.Builder, report Report) {
	if len(report.Trend) == 0 {
		return
	}

	b.WriteString(`
                <tr>
                  <td style="padding-top:28px; padding-bottom:14px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:16px; font-weight:bold; color:#3D6D91; mso-line-height-rule:exactly; line-height:22px;">Last 7 days</p>
                  </td>
                </tr>`)

	writeHTMLTrendChart(b, report.Trend, trendChartSpec{
		Heading:       "Sleep (hours)",
		LabelColor:    "#6E4E96",
		BarColor:      "#B99BD1",
		TrackWidth:    292,
		BarHeight:     14,
		ValueWidth:    40,
		ValueFontSize: "12px",
		Value:         func(d TrendDay) float64 { return d.SleepHours },
		Format:        func(v float64) string { return fmt.Sprintf("%.1fh", v) },
	})
	writeHTMLTrendMultiChart(b, report.Trend, trendMultiChartSpec{
		Heading:      "Feeds",
		HeadingColor: "#8F5A2B",
		ValueWidth:   60,
		Series: []trendSeriesSpec{
			{
				Legend: "Count",
				Color:  "#E8A87C",
				Value:  func(d TrendDay) float64 { return float64(d.FeedCount) },
				Format: func(v float64) string { return fmt.Sprintf("%d", int(v)) },
			},
			{
				Legend: "Duration",
				Color:  "#F0C7A3",
				Value:  func(d TrendDay) float64 { return float64(d.FeedDurationMinutes) },
				Format: func(v float64) string { return formatTrendDurationMinutes(int(v)) },
			},
			{
				Legend: "Bottle mL",
				Color:  "#B5652F",
				Value:  func(d TrendDay) float64 { return float64(d.FeedBottleMl) },
				Format: func(v float64) string { return fmt.Sprintf("%d mL", int(v)) },
			},
		},
	})
	writeHTMLTrendChart(b, report.Trend, trendChartSpec{
		Heading:       "Nappies changed",
		LabelColor:    "#9C7A4E",
		BarColor:      "#9C7A4E",
		TrackWidth:    170,
		BarHeight:     12,
		ValueWidth:    30,
		ValueFontSize: "11.5px",
		Nowrap:        true,
		Value:         func(d TrendDay) float64 { return float64(d.NappyCount) },
		Format:        func(v float64) string { return fmt.Sprintf("%d", int(v)) },
	})
	writeHTMLTrendMultiChart(b, report.Trend, trendMultiChartSpec{
		Heading:      "Pump",
		HeadingColor: "#B5652F",
		ValueWidth:   46,
		Series: []trendSeriesSpec{
			{
				Legend: "mL",
				Color:  "#D6A339",
				Value:  func(d TrendDay) float64 { return float64(d.PumpMl) },
				Format: func(v float64) string { return fmt.Sprintf("%d mL", int(v)) },
			},
			{
				Legend: "Duration",
				Color:  "#E8C978",
				Value:  func(d TrendDay) float64 { return float64(d.PumpDurationMinutes) },
				Format: func(v float64) string { return formatTrendDurationMinutes(int(v)) },
			},
		},
	})
}

// writeHTMLTrendChart renders one labeled single-bar chart row group. Bars
// are scaled against the max value across the given days for that metric
// alone, so a quiet week doesn't read as flat against a busy one.
func writeHTMLTrendChart(b *strings.Builder, days []TrendDay, spec trendChartSpec) {
	maxValue := 0.0
	for _, d := range days {
		if v := spec.Value(d); v > maxValue {
			maxValue = v
		}
	}

	nowrap := ""
	if spec.Nowrap {
		nowrap = " white-space:nowrap;"
	}

	b.WriteString(`
                <tr>
                  <td style="padding-bottom:6px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:12px; font-weight:bold; letter-spacing:0.04em; color:`)
	b.WriteString(spec.LabelColor)
	b.WriteString(`; text-transform:uppercase; mso-line-height-rule:exactly; line-height:18px;">`)
	b.WriteString(htmlEscape(spec.Heading))
	b.WriteString(`</p>
                  </td>
                </tr>
                <tr>
                  <td style="padding-bottom:20px;">
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">`)

	for i, d := range days {
		value := spec.Value(d)
		barWidth := 0
		if maxValue > 0 {
			barWidth = int(math.Round(float64(spec.TrackWidth) * value / maxValue))
			if barWidth > spec.TrackWidth {
				barWidth = spec.TrackWidth
			}
		}
		spacerWidth := spec.TrackWidth - barWidth

		rowPadding := ""
		if i > 0 {
			rowPadding = " padding-top:6px;"
		}

		b.WriteString(fmt.Sprintf(`
                      <tr><td width="34" style="font-family:Arial, Helvetica, sans-serif; font-size:12px; color:#9C9184;%s">%s</td><td style="%s"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr><td width="%d" height="%d" bgcolor="%s" style="border-radius:4px; font-size:1px; line-height:1px;">&nbsp;</td>`,
			rowPadding, htmlEscape(d.Label), strings.TrimPrefix(rowPadding, " "), barWidth, spec.BarHeight, spec.BarColor))
		if spacerWidth > 0 {
			b.WriteString(fmt.Sprintf(`<td width="%d" style="font-size:1px; line-height:1px;">&nbsp;</td>`, spacerWidth))
		}
		b.WriteString(fmt.Sprintf(`</tr></table></td><td width="%d" align="right" style="font-family:Arial, Helvetica, sans-serif; font-size:%s; color:#3A332C;%s%s">%s</td></tr>`,
			spec.ValueWidth, spec.ValueFontSize, nowrap, rowPadding, htmlEscape(spec.Format(value))))
	}

	b.WriteString(`
                    </table>
                  </td>
                </tr>`)
}

// writeHTMLTrendMultiChart renders one stacked multi-series chart (Feeds,
// Pump): a heading with a color-swatch legend, then one row group per day
// with each series' bar on its own line under a rowspan'd day label. Each
// series is scaled against its own weekly max independently of the others,
// since e.g. feed counts and bottle mL totals live on very different scales.
func writeHTMLTrendMultiChart(b *strings.Builder, days []TrendDay, spec trendMultiChartSpec) {
	b.WriteString(`
                <tr>
                  <td style="padding-bottom:8px;">
                    <p style="margin:0 0 8px; font-family:Arial, Helvetica, sans-serif; font-size:12px; font-weight:bold; letter-spacing:0.04em; color:`)
	b.WriteString(spec.HeadingColor)
	b.WriteString(`; text-transform:uppercase; mso-line-height-rule:exactly; line-height:18px;">`)
	b.WriteString(htmlEscape(spec.Heading))
	b.WriteString(`</p>
                    <table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>`)
	for _, series := range spec.Series {
		b.WriteString(fmt.Sprintf(`<td style="padding-right:14px;"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr><td width="9" height="9" bgcolor="%s" style="border-radius:2px; font-size:1px; line-height:9px;">&nbsp;</td><td style="font-family:Arial, Helvetica, sans-serif; font-size:11px; color:#6B7280; padding-left:5px;">%s</td></tr></table></td>`,
			series.Color, htmlEscape(series.Legend)))
	}
	b.WriteString(`</tr></table>
                  </td>
                </tr>
                <tr>
                  <td style="padding-bottom:20px; padding-top:10px;">
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">`)

	maxValues := make([]float64, len(spec.Series))
	for si, series := range spec.Series {
		for _, d := range days {
			if v := series.Value(d); v > maxValues[si] {
				maxValues[si] = v
			}
		}
	}

	for di, d := range days {
		dayPadding := ""
		if di > 0 {
			dayPadding = "padding-top:10px;"
		}
		for si, series := range spec.Series {
			seriesPadding := dayPadding
			if si > 0 {
				seriesPadding = "padding-top:3px;"
			}

			value := series.Value(d)
			barWidth := 0
			if maxValues[si] > 0 {
				barWidth = int(math.Round(trendMultiBarTrackWidth * value / maxValues[si]))
				if barWidth > trendMultiBarTrackWidth {
					barWidth = trendMultiBarTrackWidth
				}
			}
			spacerWidth := trendMultiBarTrackWidth - barWidth

			b.WriteString(`
                      <tr>`)
			if si == 0 {
				b.WriteString(fmt.Sprintf(`<td width="34" rowspan="%d" valign="top" style="font-family:Arial, Helvetica, sans-serif; font-size:12px; font-weight:bold; color:#3A332C; %s">%s</td>`,
					len(spec.Series), dayPadding, htmlEscape(d.Label)))
			}
			b.WriteString(fmt.Sprintf(`<td style="%s"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr><td width="%d" height="12" bgcolor="%s" style="border-radius:4px; font-size:1px; line-height:1px;">&nbsp;</td>`,
				seriesPadding, barWidth, series.Color))
			if spacerWidth > 0 {
				b.WriteString(fmt.Sprintf(`<td width="%d" style="font-size:1px; line-height:1px;">&nbsp;</td>`, spacerWidth))
			}
			b.WriteString(fmt.Sprintf(`</tr></table></td><td width="%d" align="right" style="font-family:Arial, Helvetica, sans-serif; font-size:11.5px; color:#3A332C; white-space:nowrap; %s">%s</td></tr>`,
				spec.ValueWidth, seriesPadding, htmlEscape(series.Format(value))))
		}
	}

	b.WriteString(`
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
