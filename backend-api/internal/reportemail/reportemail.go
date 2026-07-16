package reportemail

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	log.Printf("AI %s report email for %s to %s: %s", report.ReportType, report.BabyName, report.RecipientEmail, report.Output.Title)
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
