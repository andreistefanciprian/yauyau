// Package backendclient holds the backend-api response types and the HTTP
// client that fetches them. See internal/handlers for the interface that
// consumes this package.
package backendclient

import (
	"errors"
	"time"
)

var ErrForbidden = errors.New("forbidden")
var ErrNotFound = errors.New("not found")

// APIError carries backend-api's own {"error": "..."} message through to the
// caller, so a 400 validation failure (e.g. "occurred_at cannot be in the
// future") can be shown to the user instead of a generic failure message.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
}

type Baby struct {
	ID               string `json:"id"`
	FamilyID         string `json:"family_id"`
	Name             string `json:"name"`
	Timezone         string `json:"timezone"`
	BirthDate        string `json:"birth_date,omitempty"`
	BirthWeightKg    string `json:"birth_weight_kg,omitempty"`
	BirthLengthCm    string `json:"birth_length_cm,omitempty"`
	Sex              string `json:"sex,omitempty"`
	CanInvite        bool   `json:"can_invite"`
	HasPendingInvite bool   `json:"has_pending_invite"`
}

type User struct {
	ID                        string `json:"id"`
	Email                     string `json:"email"`
	DisplayName               string `json:"display_name,omitempty"`
	CanManageDailyReportEmail bool   `json:"can_manage_daily_report_email"`
	DailyReportEmailEnabled   bool   `json:"daily_report_email_enabled"`
}

type TimelineMember struct {
	UserID                  string `json:"user_id"`
	Email                   string `json:"email"`
	Role                    string `json:"role"`
	Status                  string `json:"status"`
	Relationship            string `json:"relationship,omitempty"`
	DailyReportEmailEnabled bool   `json:"daily_report_email_enabled"`
}

type TimelineMembersResult struct {
	Members []TimelineMember `json:"members"`
}

// Event is a generic event exactly as backend-api's combined /events
// endpoint returns it: event_type plus its type-specific attributes, not a
// typed per-event shape. Interpreting Attributes is internal/handlers' job,
// same division of responsibility as backend-api's own store.Event.
type Event struct {
	ID         string         `json:"id"`
	BabyID     string         `json:"baby_id"`
	EventType  string         `json:"event_type"`
	Attributes map[string]any `json:"attributes"`
	OccurredAt time.Time      `json:"occurred_at"`
	CreatedAt  time.Time      `json:"created_at"`
}

type DailyReport struct {
	Title        string           `json:"title"`
	Summary      string           `json:"summary"`
	Highlights   []string         `json:"highlights"`
	Card         *DailyReportCard `json:"card,omitempty"`
	GeneratedAt  time.Time        `json:"generated_at"`
	RangeStart   time.Time        `json:"range_start"`
	RangeEnd     time.Time        `json:"range_end"`
	SelectedDate string           `json:"-"`
	LoadAI       bool             `json:"-"`
}

type DailyReportCard struct {
	Intro          string                     `json:"intro"`
	PrimaryMetrics []DailyReportPrimaryMetric `json:"primary_metrics"`
	Story          string                     `json:"story,omitempty"`
	Observation    string                     `json:"observation,omitempty"`
	Encouragement  string                     `json:"encouragement,omitempty"`
}

type DailyReportPrimaryMetric struct {
	Count     string `json:"count"`
	Total     string `json:"total,omitempty"`
	Qualifier string `json:"qualifier,omitempty"`
}

type AIDailyCard struct {
	SchemaVersion string `json:"schema_version"`
	Opening       string `json:"opening"`
	Story         string `json:"story"`
	Observation   string `json:"observation"`
	Encouragement string `json:"encouragement"`
}
