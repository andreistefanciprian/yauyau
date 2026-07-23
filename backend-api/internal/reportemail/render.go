package reportemail

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const reportEncouragement = "You've got this."

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
	writeTextTrend(&b, report.Trend)

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

func writeTextTrend(b *strings.Builder, days []TrendDay) {
	if len(days) == 0 {
		return
	}

	b.WriteString("\nLast 7 days:\n")
	for _, day := range days {
		fmt.Fprintf(
			b,
			"%s: Sleep %.1fh · Feeds %d (%s, %d mL bottle) · Pump %d mL (%s) · Nappies %d\n",
			day.Label,
			day.SleepHours,
			day.FeedCount,
			formatTrendDurationMinutes(day.FeedDurationMinutes),
			day.FeedBottleMl,
			day.PumpMl,
			formatTrendDurationMinutes(day.PumpDurationMinutes),
			day.NappyCount,
		)
	}
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
                  <td style="padding-bottom:4px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:19px; font-weight:bold; color:#2C5C77; mso-line-height-rule:exactly; line-height:26px;">`)
	b.WriteString(htmlEscape(reportDateHeading(report)))
	b.WriteString(`</p>
                  </td>
                </tr>
                <tr>
                  <td style="padding-bottom:20px;">
                    <p style="margin:0; font-family:Arial, Helvetica, sans-serif; font-size:13px; color:#9C9184; mso-line-height-rule:exactly; line-height:18px;">`)
	b.WriteString(htmlEscape(reportSubtitle(report)))
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

	writeHTMLTrend(&b, report)

	b.WriteString(`
                <tr>
                  <td align="center" style="padding-top:8px;">
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
                        <td style="padding:22px 24px;">
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

// newestFirst returns a copy of days in reverse order, for chart rendering
// that wants the most recent day first without mutating the caller's slice
// or affecting the plain-text trend log, which reads more naturally
// oldest-first as a chronological list.
func newestFirst(days []TrendDay) []TrendDay {
	reversed := make([]TrendDay, len(days))
	for i, d := range days {
		reversed[len(days)-1-i] = d
	}
	return reversed
}

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

// trendBarLabelWidth, trendBarValueWidth, and trendBarTrackWidth are the
// column widths shared by every "Last 7 days" chart, matching the Reports
// page design's row layout (30px day label + flexible bar + 44px value).
const (
	trendBarLabelWidth = 30
	trendBarValueWidth = 44
	trendBarTrackWidth = 434
	trendBarTrackColor = "#EDE2D6"
)

// trendLegendEntry is one color-swatch legend chip shown beside a chart's
// heading, e.g. the amber square labeled "Count" on the Feeds chart.
type trendLegendEntry struct {
	Color string
	Label string
}

// trendSeriesSpec is one bar series within a "Last 7 days" chart. Single-bar
// charts (Sleep, Nappies) have exactly one series; stacked charts (Feeds,
// Pump) have several, each on its own row per day.
type trendSeriesSpec struct {
	Color  string
	Value  func(TrendDay) float64
	Format func(float64) string
}

// trendChartSpec describes one of the "Last 7 days" charts: a heading, its
// legend chips, and one or more stacked bar series.
type trendChartSpec struct {
	Heading       string
	HeadingColor  string
	Legend        []trendLegendEntry
	BarHeight     int
	ValueFontSize string
	Series        []trendSeriesSpec
}

// writeHTMLTrend renders the "Last 7 days" section: Sleep, Feeds
// (Count/Duration/Bottle mL stacked with a legend), Nappies, then Pump
// (mL/Duration stacked with a legend) — matching the Reports page design.
// Each series scales independently against its own weekly max. Omitted
// entirely when report.Trend is empty.
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
		Heading:       "Sleep",
		HeadingColor:  "#6E4E96",
		Legend:        []trendLegendEntry{{Color: "#B99BD1", Label: "Hours"}},
		BarHeight:     12,
		ValueFontSize: "11.5px",
		Series: []trendSeriesSpec{
			{
				Color:  "#B99BD1",
				Value:  func(d TrendDay) float64 { return d.SleepHours },
				Format: func(v float64) string { return fmt.Sprintf("%.1fh", v) },
			},
		},
	})
	writeHTMLTrendChart(b, report.Trend, trendChartSpec{
		Heading:      "Feeds",
		HeadingColor: "#8F5A2B",
		Legend: []trendLegendEntry{
			{Color: "#E8A87C", Label: "Count"},
			{Color: "#F0C7A3", Label: "Duration"},
			{Color: "#B5652F", Label: "Bottle mL"},
		},
		BarHeight:     9,
		ValueFontSize: "11px",
		Series: []trendSeriesSpec{
			{
				Color:  "#E8A87C",
				Value:  func(d TrendDay) float64 { return float64(d.FeedCount) },
				Format: func(v float64) string { return fmt.Sprintf("%d", int(v)) },
			},
			{
				Color:  "#F0C7A3",
				Value:  func(d TrendDay) float64 { return float64(d.FeedDurationMinutes) },
				Format: func(v float64) string { return formatTrendDurationMinutes(int(v)) },
			},
			{
				Color:  "#B5652F",
				Value:  func(d TrendDay) float64 { return float64(d.FeedBottleMl) },
				Format: func(v float64) string { return fmt.Sprintf("%d mL", int(v)) },
			},
		},
	})
	writeHTMLTrendChart(b, report.Trend, trendChartSpec{
		Heading:       "Nappies",
		HeadingColor:  "#9C7A4E",
		Legend:        []trendLegendEntry{{Color: "#9C7A4E", Label: "Changed"}},
		BarHeight:     12,
		ValueFontSize: "11.5px",
		Series: []trendSeriesSpec{
			{
				Color:  "#9C7A4E",
				Value:  func(d TrendDay) float64 { return float64(d.NappyCount) },
				Format: func(v float64) string { return fmt.Sprintf("%d", int(v)) },
			},
		},
	})
	writeHTMLTrendChart(b, report.Trend, trendChartSpec{
		Heading:      "Pump",
		HeadingColor: "#B5652F",
		Legend: []trendLegendEntry{
			{Color: "#D6A339", Label: "mL"},
			{Color: "#E8C978", Label: "Duration"},
		},
		BarHeight:     9,
		ValueFontSize: "11px",
		Series: []trendSeriesSpec{
			{
				Color:  "#D6A339",
				Value:  func(d TrendDay) float64 { return float64(d.PumpMl) },
				Format: func(v float64) string { return fmt.Sprintf("%d mL", int(v)) },
			},
			{
				Color:  "#E8C978",
				Value:  func(d TrendDay) float64 { return float64(d.PumpDurationMinutes) },
				Format: func(v float64) string { return formatTrendDurationMinutes(int(v)) },
			},
		},
	})
}

// writeHTMLTrendChart renders one chart: a heading row with its legend chips
// on the right, then one row per day per series. Each series is scaled
// against the max value across the given days for that series alone, so a
// quiet pump week doesn't read as flat against a busy feed week.
func writeHTMLTrendChart(b *strings.Builder, days []TrendDay, spec trendChartSpec) {
	maxValues := make([]float64, len(spec.Series))
	for si, series := range spec.Series {
		for _, d := range days {
			if v := series.Value(d); v > maxValues[si] {
				maxValues[si] = v
			}
		}
	}

	// days arrives oldest-first (see TrendDay's doc comment); render
	// newest-first instead so the report day sits at the top of each chart,
	// closest to the rest of the report, rather than scrolled to the bottom.
	days = newestFirst(days)

	b.WriteString(`
                <tr>
                  <td style="padding-bottom:8px;">
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
                      <tr>
                        <td align="left" style="font-family:Arial, Helvetica, sans-serif; font-size:12px; font-weight:bold; letter-spacing:0.04em; text-transform:uppercase; color:`)
	b.WriteString(spec.HeadingColor)
	b.WriteString(`;">`)
	b.WriteString(htmlEscape(spec.Heading))
	b.WriteString(`</td>
                        <td align="right"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>`)
	for i, entry := range spec.Legend {
		padding := ""
		if i > 0 {
			padding = "padding-left:12px;"
		}
		b.WriteString(fmt.Sprintf(`<td style="%s"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr><td width="8" height="8" bgcolor="%s" style="border-radius:2px; font-size:1px; line-height:8px;">&nbsp;</td><td style="font-family:Arial, Helvetica, sans-serif; font-size:11px; color:#6B7280; padding-left:5px; white-space:nowrap;">%s</td></tr></table></td>`,
			padding, entry.Color, htmlEscape(entry.Label)))
	}
	b.WriteString(`</tr></table></td>
                      </tr>
                    </table>
                  </td>
                </tr>
                <tr>
                  <td style="padding-bottom:18px;">
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">`)

	multiSeries := len(spec.Series) > 1
	for di, d := range days {
		for si, series := range spec.Series {
			rowPadding := ""
			switch {
			case di == 0 && si == 0:
				rowPadding = ""
			case si > 0:
				rowPadding = "padding-top:2px;"
			case multiSeries:
				rowPadding = "padding-top:8px;"
			default:
				rowPadding = "padding-top:5px;"
			}

			label := ""
			if si == 0 {
				label = htmlEscape(d.Label)
			}

			value := series.Value(d)
			barWidth := 0
			if maxValues[si] > 0 {
				barWidth = int(math.Round(trendBarTrackWidth * value / maxValues[si]))
				if barWidth > trendBarTrackWidth {
					barWidth = trendBarTrackWidth
				}
			}
			spacerWidth := trendBarTrackWidth - barWidth

			b.WriteString(fmt.Sprintf(`
                      <tr><td width="%d" style="font-family:Arial, Helvetica, sans-serif; font-size:11.5px; color:#9C9184; padding-right:10px; %s">%s</td><td style="padding-right:10px; %s"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>`,
				trendBarLabelWidth, rowPadding, label, rowPadding))
			// Bar and track render as one seamless pill: whichever segment is
			// present alone gets fully rounded corners, and when both are
			// present the fill only rounds its left edge while the track
			// only rounds its right edge, so there's no visible seam between
			// the two colors.
			switch {
			case barWidth == 0:
				b.WriteString(fmt.Sprintf(`<td width="%d" height="%d" bgcolor="%s" style="border-radius:4px; font-size:1px; line-height:1px;">&nbsp;</td>`,
					trendBarTrackWidth, spec.BarHeight, trendBarTrackColor))
			case spacerWidth == 0:
				b.WriteString(fmt.Sprintf(`<td width="%d" height="%d" bgcolor="%s" style="border-radius:4px; font-size:1px; line-height:1px;">&nbsp;</td>`,
					barWidth, spec.BarHeight, series.Color))
			default:
				b.WriteString(fmt.Sprintf(`<td width="%d" height="%d" bgcolor="%s" style="border-radius:4px 0 0 4px; font-size:1px; line-height:1px;">&nbsp;</td><td width="%d" height="%d" bgcolor="%s" style="border-radius:0 4px 4px 0; font-size:1px; line-height:1px;">&nbsp;</td>`,
					barWidth, spec.BarHeight, series.Color, spacerWidth, spec.BarHeight, trendBarTrackColor))
			}
			b.WriteString(fmt.Sprintf(`</tr></table></td><td width="%d" align="right" style="font-family:Arial, Helvetica, sans-serif; font-size:%s; color:#3A332C; white-space:nowrap; %s">%s</td></tr>`,
				trendBarValueWidth, spec.ValueFontSize, rowPadding, htmlEscape(series.Format(value))))
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

// reportSubtitle renders the small muted line under the date heading, the
// same "{baby}'s day, summarised" pattern the Reports page uses, adapted to
// whatever period the report covers (daily, weekly, ...).
func reportSubtitle(report Report) string {
	period := reportPeriodNoun(report.ReportType)
	name := strings.TrimSpace(report.BabyName)
	if name == "" {
		return fmt.Sprintf("%s, summarised", period)
	}
	return fmt.Sprintf("%s's %s, summarised", name, period)
}

func reportPeriodNoun(reportType string) string {
	switch strings.ToLower(strings.TrimSpace(reportType)) {
	case "daily":
		return "day"
	case "weekly":
		return "week"
	case "":
		return "report"
	default:
		return reportType
	}
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
