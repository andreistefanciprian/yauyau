package reportemail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/andreistefanciprian/yauli/backend-api/internal/aireport"
)

// Report is the renderer-facing email payload. It contains the validated AI
// report JSON plus scheduling context, but no auth/session data.
type Report struct {
	RecipientEmail string
	BabyName       string
	ReportType     string
	StartDate      string
	EndDate        string
	Output         aireport.Output
	// Card holds the deterministic KPI counts (feeds, sleep, pump, nappies)
	// shown at the top of the email, the same numbers the web app's daily
	// report card shows. Optional: nil/empty when unavailable, in which case
	// the email simply omits that section.
	Card []CardMetric
	// Trend holds the last 7 calendar days of daily totals (oldest first,
	// ending with the report day) for the email's "Last 7 days" bar charts.
	// Optional: nil/empty when unavailable, in which case the email simply
	// omits that section.
	Trend []TrendDay
}

// CardMetric is one KPI column in the report email's summary card, e.g.
// "9 Feeds, 660 ml".
type CardMetric struct {
	Label  string
	Count  int
	Detail string
}

// TrendDay is one calendar day's daily totals in the email's "Last 7 days"
// trend charts.
type TrendDay struct {
	Label               string
	SleepHours          float64
	FeedCount           int
	FeedDurationMinutes int
	FeedBottleMl        int
	NappyCount          int
	PumpMl              int
	PumpDurationMinutes int
}

// Sender delivers rendered AI report emails and returns the provider message
// id when the provider gives us one.
type Sender interface {
	SendReportEmail(ctx context.Context, report Report) (string, error)
}

// Stdout keeps local development usable before Mailgun credentials are
// configured. The scheduler can exercise the same path without sending mail.
type Stdout struct{}

func (Stdout) SendReportEmail(_ context.Context, report Report) (string, error) {
	slog.Info("development report email",
		"report_type", report.ReportType,
	)
	return "", nil
}

// Disabled keeps backend-api bootable when report email delivery has not been
// configured yet. Scheduler code should treat this as a failed delivery
// attempt, not as a successful no-op.
type Disabled struct{}

func (Disabled) SendReportEmail(context.Context, Report) (string, error) {
	return "", errors.New("report email delivery is not configured")
}

func subject(report Report) string {
	reportType := strings.TrimSpace(report.ReportType)
	if reportType == "" {
		reportType = "baby"
	}
	return fmt.Sprintf("%s's %s report", report.BabyName, reportType)
}
