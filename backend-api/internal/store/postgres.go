package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is Postgres's SQLSTATE code for a unique-constraint
// violation (23505), used to recognize idx_family_members_one_active_per_user
// rejecting a second active membership without depending on its error text.
const uniqueViolation = "23505"

// Connect opens a connection pool to PostgreSQL using the given DATABASE_URL.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}

// Migrate applies the .sql files in migrationsDir in lexical order. Each
// migration is idempotent (CREATE TABLE IF NOT EXISTS / INSERT ... ON
// CONFLICT DO NOTHING), so it is safe to run on every startup.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)

	for _, name := range files {
		contents, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		if _, err := pool.Exec(ctx, string(contents)); err != nil {
			return fmt.Errorf("applying migration %s: %w", name, err)
		}
	}

	return nil
}

// PostgresStore is the Postgres-backed implementation of the persistence
// methods that internal/handlers.Store expects.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

const babyColumns = `
	id,
	family_id,
	name,
	timezone,
	COALESCE(birth_date::text, ''),
	COALESCE(birth_weight_kg::text, ''),
	COALESCE(birth_length_cm::text, ''),
	COALESCE(sex, '')
`

const (
	dailyReportEmailReportType = "daily"
	dailyReportEmailSendHour   = 9
)

func scanBaby(row pgx.Row) (Baby, error) {
	var baby Baby
	err := row.Scan(
		&baby.ID,
		&baby.FamilyID,
		&baby.Name,
		&baby.Timezone,
		&baby.BirthDate,
		&baby.BirthWeightKg,
		&baby.BirthLengthCm,
		&baby.Sex,
	)
	return baby, err
}

type dailyReportEmailCandidate struct {
	FamilyID        uuid.UUID
	BabyID          uuid.UUID
	BabyName        string
	BabyTimezone    string
	RecipientUserID uuid.UUID
	RecipientEmail  string
}

// GetBaby returns the baby with id, regardless of which family it belongs to.
// Callers use the returned FamilyID for authorization checks before exposing
// or mutating anything through user-facing routes.
func (s *PostgresStore) GetBaby(ctx context.Context, id uuid.UUID) (Baby, error) {
	const query = `
		SELECT ` + babyColumns + `
		FROM babies
		WHERE id = $1 AND archived_at IS NULL
	`

	baby, err := scanBaby(s.pool.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Baby{}, ErrNotFound
	}
	if err != nil {
		return Baby{}, fmt.Errorf("get baby: %w", err)
	}

	return baby, nil
}

// GetCurrentBaby returns familyID's "current" baby: the first one created,
// since a family with more than one baby has no other ordering signal yet.
func (s *PostgresStore) GetCurrentBaby(ctx context.Context, familyID uuid.UUID) (Baby, error) {
	const query = `
		SELECT ` + babyColumns + `
		FROM babies
		WHERE family_id = $1 AND archived_at IS NULL
		ORDER BY created_at ASC
		LIMIT 1
	`

	baby, err := scanBaby(s.pool.QueryRow(ctx, query, familyID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Baby{}, ErrNotFound
	}
	if err != nil {
		return Baby{}, fmt.Errorf("get current baby: %w", err)
	}

	return baby, nil
}

// CreateBaby adds a new baby to familyID, which must already exist — the
// caller (handlers.CreateBaby) is responsible for creating the family first
// via CreateFamilyWithOwner if this is the family's first baby.
func (s *PostgresStore) CreateBaby(ctx context.Context, familyID uuid.UUID, name, timezone string) (Baby, error) {
	const query = `
		INSERT INTO babies (id, family_id, name, timezone)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + babyColumns + `
	`

	baby := Baby{ID: uuid.New()}
	baby, err := scanBaby(s.pool.QueryRow(ctx, query, baby.ID, familyID, name, timezone))
	if err != nil {
		return Baby{}, fmt.Errorf("creating baby: %w", err)
	}

	return baby, nil
}

// UpdateBaby updates editable profile fields for an active baby belonging to
// familyID and returns the updated row.
func (s *PostgresStore) UpdateBaby(ctx context.Context, familyID, babyID uuid.UUID, baby Baby) (Baby, error) {
	const query = `
		UPDATE babies
		SET
			name = $1,
			timezone = $2,
			birth_date = NULLIF($3, '')::date,
			birth_weight_kg = NULLIF($4, '')::numeric,
			birth_length_cm = NULLIF($5, '')::numeric,
			sex = NULLIF($6, '')
		WHERE id = $7 AND family_id = $8 AND archived_at IS NULL
		RETURNING ` + babyColumns + `
	`

	updated, err := scanBaby(s.pool.QueryRow(
		ctx,
		query,
		baby.Name,
		baby.Timezone,
		baby.BirthDate,
		baby.BirthWeightKg,
		baby.BirthLengthCm,
		baby.Sex,
		babyID,
		familyID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Baby{}, ErrNotFound
	}
	if err != nil {
		return Baby{}, fmt.Errorf("updating baby: %w", err)
	}

	return updated, nil
}

// ArchiveBaby soft-deletes an active baby timeline. Events remain stored for
// future recovery/audit, but active reads no longer return the baby.
func (s *PostgresStore) ArchiveBaby(ctx context.Context, familyID, babyID uuid.UUID) error {
	const query = `
		UPDATE babies
		SET archived_at = NOW()
		WHERE id = $1 AND family_id = $2 AND archived_at IS NULL
	`

	tag, err := s.pool.Exec(ctx, query, babyID, familyID)
	if err != nil {
		return fmt.Errorf("archiving baby: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *PostgresStore) CreateEvent(ctx context.Context, familyID, babyID uuid.UUID, eventType string, attributes map[string]any, occurredAt time.Time) (Event, error) {
	id := uuid.New()

	const query = `
		INSERT INTO events (id, family_id, baby_id, event_type, occurred_at, attributes, source)
		VALUES ($1, $2, $3, $4, $5, $6, 'web')
		RETURNING created_at
	`

	var createdAt time.Time
	err := s.pool.QueryRow(ctx, query, id, familyID, babyID, eventType, occurredAt, attributes).Scan(&createdAt)
	if err != nil {
		return Event{}, fmt.Errorf("inserting %s event: %w", eventType, err)
	}

	return Event{
		ID:         id,
		BabyID:     babyID,
		EventType:  eventType,
		OccurredAt: occurredAt,
		CreatedAt:  createdAt,
		Attributes: attributes,
	}, nil
}

// GetEvent returns one event for the current baby. Handlers use this when a
// generic event route needs the event type before deciding follow-up work.
func (s *PostgresStore) GetEvent(ctx context.Context, familyID, babyID, id uuid.UUID) (Event, error) {
	const query = `
		SELECT id, baby_id, event_type, attributes, occurred_at, created_at
		FROM events
		WHERE id = $1 AND family_id = $2 AND baby_id = $3
	`

	var ev Event
	err := s.pool.QueryRow(ctx, query, id, familyID, babyID).Scan(
		&ev.ID,
		&ev.BabyID,
		&ev.EventType,
		&ev.Attributes,
		&ev.OccurredAt,
		&ev.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, fmt.Errorf("getting event: %w", err)
	}

	return ev, nil
}

// UpdateEvent replaces the editable fields of an existing event that belongs
// to the current baby. eventType is part of the WHERE clause so callers
// cannot accidentally transform a nappy into a feed by posting mismatched
// form data.
func (s *PostgresStore) UpdateEvent(ctx context.Context, familyID, babyID, id uuid.UUID, eventType string, attributes map[string]any, occurredAt time.Time) (Event, error) {
	const query = `
		UPDATE events
		SET attributes = $1, occurred_at = $2
		WHERE id = $3 AND family_id = $4 AND baby_id = $5 AND event_type = $6
		RETURNING created_at
	`

	var createdAt time.Time
	err := s.pool.QueryRow(ctx, query, attributes, occurredAt, id, familyID, babyID, eventType).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, fmt.Errorf("updating event: %w", err)
	}

	return Event{
		ID:         id,
		BabyID:     babyID,
		EventType:  eventType,
		OccurredAt: occurredAt,
		CreatedAt:  createdAt,
		Attributes: attributes,
	}, nil
}

// DeleteEvent removes a single event belonging to the current baby.
// ErrNotFound is returned if no matching row exists (already deleted, wrong
// id, or belongs to a different baby), so callers can tell that apart from a
// real database error.
func (s *PostgresStore) DeleteEvent(ctx context.Context, familyID, babyID, id uuid.UUID) error {
	const query = `DELETE FROM events WHERE id = $1 AND family_id = $2 AND baby_id = $3`

	tag, err := s.pool.Exec(ctx, query, id, familyID, babyID)
	if err != nil {
		return fmt.Errorf("deleting event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ListAllEvents returns the most recent events across every event type,
// ordered newest-first, for consumers (the frontend timeline) that need a
// single merged view instead of one list per type.
func (s *PostgresStore) ListAllEvents(ctx context.Context, familyID, babyID uuid.UUID, from, to time.Time, limit int) ([]Event, error) {
	const query = `
		SELECT id, baby_id, event_type, attributes, occurred_at, created_at
		FROM events
		WHERE family_id = $1
			AND baby_id = $2
			AND occurred_at >= $3
			AND occurred_at < $4
		ORDER BY occurred_at DESC
		LIMIT $5
	`

	rows, err := s.pool.Query(ctx, query, familyID, babyID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var results []Event
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.BabyID, &ev.EventType, &ev.Attributes, &ev.OccurredAt, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		results = append(results, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating events: %w", err)
	}

	return results, nil
}

// GetBabyLatestGrowth returns the current projection of the latest known
// growth measurements for babyID. ErrNotFound means no growth measurements
// have been recorded yet.
func (s *PostgresStore) GetBabyLatestGrowth(ctx context.Context, familyID, babyID uuid.UUID) (BabyLatestGrowth, error) {
	const query = `
		SELECT
			family_id,
			baby_id,
			weight_grams,
			weight_measured_at,
			length_cm,
			length_measured_at,
			head_circumference_cm,
			head_circumference_measured_at,
			updated_at
		FROM baby_latest_growth
		WHERE family_id = $1 AND baby_id = $2
	`

	growth, err := scanBabyLatestGrowth(s.pool.QueryRow(ctx, query, familyID, babyID))
	if errors.Is(err, pgx.ErrNoRows) {
		return BabyLatestGrowth{}, ErrNotFound
	}
	if err != nil {
		return BabyLatestGrowth{}, fmt.Errorf("getting baby latest growth: %w", err)
	}

	return growth, nil
}

// RefreshBabyLatestGrowth rebuilds the latest-growth projection from
// growth_measurement events after one of those events is created, edited, or
// deleted. It keeps each measurement type independent because families often
// record weight, length, and head circumference at different times.
func (s *PostgresStore) RefreshBabyLatestGrowth(ctx context.Context, familyID, babyID uuid.UUID) (BabyLatestGrowth, error) {
	const query = `
		WITH latest_weight AS (
			SELECT (attributes->>'weight_grams')::integer AS weight_grams, occurred_at
			FROM events
			WHERE family_id = $1
				AND baby_id = $2
				AND event_type = 'growth_measurement'
				AND attributes ? 'weight_grams'
			ORDER BY occurred_at DESC, id DESC
			LIMIT 1
		),
		latest_length AS (
			SELECT (attributes->>'length_cm')::double precision AS length_cm, occurred_at
			FROM events
			WHERE family_id = $1
				AND baby_id = $2
				AND event_type = 'growth_measurement'
				AND attributes ? 'length_cm'
			ORDER BY occurred_at DESC, id DESC
			LIMIT 1
		),
		latest_head AS (
			SELECT (attributes->>'head_circumference_cm')::double precision AS head_circumference_cm, occurred_at
			FROM events
			WHERE family_id = $1
				AND baby_id = $2
				AND event_type = 'growth_measurement'
				AND attributes ? 'head_circumference_cm'
			ORDER BY occurred_at DESC, id DESC
			LIMIT 1
		)
		INSERT INTO baby_latest_growth (
			family_id,
			baby_id,
			weight_grams,
			weight_measured_at,
			length_cm,
			length_measured_at,
			head_circumference_cm,
			head_circumference_measured_at,
			updated_at
		)
		SELECT
			$1,
			$2,
			latest_weight.weight_grams,
			latest_weight.occurred_at,
			latest_length.length_cm,
			latest_length.occurred_at,
			latest_head.head_circumference_cm,
			latest_head.occurred_at,
			now()
		FROM (SELECT 1) seed
		LEFT JOIN latest_weight ON true
		LEFT JOIN latest_length ON true
		LEFT JOIN latest_head ON true
		WHERE latest_weight.weight_grams IS NOT NULL
			OR latest_length.length_cm IS NOT NULL
			OR latest_head.head_circumference_cm IS NOT NULL
		ON CONFLICT (family_id, baby_id)
		DO UPDATE SET
			weight_grams = EXCLUDED.weight_grams,
			weight_measured_at = EXCLUDED.weight_measured_at,
			length_cm = EXCLUDED.length_cm,
			length_measured_at = EXCLUDED.length_measured_at,
			head_circumference_cm = EXCLUDED.head_circumference_cm,
			head_circumference_measured_at = EXCLUDED.head_circumference_measured_at,
			updated_at = EXCLUDED.updated_at
		RETURNING
			family_id,
			baby_id,
			weight_grams,
			weight_measured_at,
			length_cm,
			length_measured_at,
			head_circumference_cm,
			head_circumference_measured_at,
			updated_at
	`

	growth, err := scanBabyLatestGrowth(s.pool.QueryRow(ctx, query, familyID, babyID))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, deleteErr := s.pool.Exec(ctx, `DELETE FROM baby_latest_growth WHERE family_id = $1 AND baby_id = $2`, familyID, babyID); deleteErr != nil {
			return BabyLatestGrowth{}, fmt.Errorf("deleting empty baby latest growth projection: %w", deleteErr)
		}
		return BabyLatestGrowth{}, ErrNotFound
	}
	if err != nil {
		return BabyLatestGrowth{}, fmt.Errorf("refreshing baby latest growth: %w", err)
	}

	return growth, nil
}

func scanBabyLatestGrowth(row pgx.Row) (BabyLatestGrowth, error) {
	var growth BabyLatestGrowth
	err := row.Scan(
		&growth.FamilyID,
		&growth.BabyID,
		&growth.WeightGrams,
		&growth.WeightMeasuredAt,
		&growth.LengthCM,
		&growth.LengthMeasuredAt,
		&growth.HeadCircumferenceCM,
		&growth.HeadCircumferenceMeasuredAt,
		&growth.UpdatedAt,
	)
	return growth, err
}

// GetAIReportCache returns a previously generated AI report for this exact
// semantic input hash. ErrNotFound means the caller should generate a fresh
// report if generation is configured.
func (s *PostgresStore) GetAIReportCache(ctx context.Context, familyID, babyID uuid.UUID, reportType string, rangeStart, rangeEnd time.Time, inputHash string) (AIReportCache, error) {
	const query = `
		SELECT
			id,
			family_id,
			baby_id,
			report_type,
			range_start,
			range_end,
			input_hash,
			prompt_version,
			input_schema_version,
			output_schema_version,
			model,
			content_json,
			created_at
		FROM ai_report_cache
		WHERE family_id = $1
			AND baby_id = $2
			AND report_type = $3
			AND range_start = $4
			AND range_end = $5
			AND input_hash = $6
	`

	report, err := scanAIReportCache(s.pool.QueryRow(ctx, query, familyID, babyID, reportType, rangeStart, rangeEnd, inputHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return AIReportCache{}, ErrNotFound
	}
	if err != nil {
		return AIReportCache{}, fmt.Errorf("getting AI report cache: %w", err)
	}

	return report, nil
}

// CreateAIReportCache stores generated report JSON as a regenerable cache
// entry. The unique key makes retries idempotent for the same report input.
func (s *PostgresStore) CreateAIReportCache(ctx context.Context, report AIReportCache) (AIReportCache, error) {
	if report.ID == uuid.Nil {
		report.ID = uuid.New()
	}

	const query = `
		INSERT INTO ai_report_cache (
			id,
			family_id,
			baby_id,
			report_type,
			range_start,
			range_end,
			input_hash,
			prompt_version,
			input_schema_version,
			output_schema_version,
			model,
			content_json
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (family_id, baby_id, report_type, range_start, range_end, input_hash)
		DO UPDATE SET
			prompt_version = EXCLUDED.prompt_version,
			input_schema_version = EXCLUDED.input_schema_version,
			output_schema_version = EXCLUDED.output_schema_version,
			model = EXCLUDED.model,
			content_json = EXCLUDED.content_json
		RETURNING
			id,
			family_id,
			baby_id,
			report_type,
			range_start,
			range_end,
			input_hash,
			prompt_version,
			input_schema_version,
			output_schema_version,
			model,
			content_json,
			created_at
	`

	saved, err := scanAIReportCache(s.pool.QueryRow(
		ctx,
		query,
		report.ID,
		report.FamilyID,
		report.BabyID,
		report.ReportType,
		report.RangeStart,
		report.RangeEnd,
		report.InputHash,
		report.PromptVersion,
		report.InputSchemaVersion,
		report.OutputSchemaVersion,
		report.Model,
		report.ContentJSON,
	))
	if err != nil {
		return AIReportCache{}, fmt.Errorf("creating AI report cache: %w", err)
	}

	return saved, nil
}

// ListDueDailyReportEmailJobs returns owner-recipient jobs whose baby's local
// 9 AM send time has arrived for today. It deliberately does not de-duplicate
// already-sent work yet; the future delivery-attempt table will own that.
func (s *PostgresStore) ListDueDailyReportEmailJobs(ctx context.Context, now time.Time) ([]DailyReportEmailJob, error) {
	const query = `
		SELECT
			fm.family_id,
			b.id,
			b.name,
			b.timezone,
			u.id,
			u.email
		FROM family_members fm
		JOIN users u ON u.id = fm.user_id
		JOIN LATERAL (
			SELECT id, name, timezone
			FROM babies
			WHERE family_id = fm.family_id
				AND archived_at IS NULL
			ORDER BY created_at ASC
			LIMIT 1
		) b ON true
		WHERE fm.role = $1
			AND fm.status = $2
			AND fm.daily_report_email_enabled = true
		ORDER BY fm.family_id, b.id, fm.created_at, u.email
	`

	rows, err := s.pool.Query(ctx, query, MembershipRoleOwner, MembershipStatusActive)
	if err != nil {
		return nil, fmt.Errorf("querying due daily report email jobs: %w", err)
	}
	defer rows.Close()

	var jobs []DailyReportEmailJob
	for rows.Next() {
		var candidate dailyReportEmailCandidate
		if err := rows.Scan(
			&candidate.FamilyID,
			&candidate.BabyID,
			&candidate.BabyName,
			&candidate.BabyTimezone,
			&candidate.RecipientUserID,
			&candidate.RecipientEmail,
		); err != nil {
			return nil, fmt.Errorf("scanning daily report email candidate: %w", err)
		}

		job, due, err := dailyReportEmailJobFor(candidate, now)
		if err != nil {
			return nil, err
		}
		if due {
			jobs = append(jobs, job)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating daily report email candidates: %w", err)
	}

	return jobs, nil
}

func dailyReportEmailJobFor(candidate dailyReportEmailCandidate, now time.Time) (DailyReportEmailJob, bool, error) {
	loc, err := time.LoadLocation(candidate.BabyTimezone)
	if err != nil {
		return DailyReportEmailJob{}, false, fmt.Errorf("load baby timezone %q for daily report email job: %w", candidate.BabyTimezone, err)
	}

	localNow := now.In(loc)
	scheduledFor := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), dailyReportEmailSendHour, 0, 0, 0, loc)
	if localNow.Before(scheduledFor) {
		return DailyReportEmailJob{}, false, nil
	}

	reportStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	reportEnd := reportStart.AddDate(0, 0, 1)

	return DailyReportEmailJob{
		FamilyID:        candidate.FamilyID,
		BabyID:          candidate.BabyID,
		BabyName:        candidate.BabyName,
		BabyTimezone:    candidate.BabyTimezone,
		RecipientUserID: candidate.RecipientUserID,
		RecipientEmail:  candidate.RecipientEmail,
		ReportType:      dailyReportEmailReportType,
		StartDate:       reportStart.Format(time.DateOnly),
		EndDate:         reportStart.Format(time.DateOnly),
		RangeStart:      reportStart,
		RangeEnd:        reportEnd,
		ScheduledFor:    scheduledFor,
	}, true, nil
}

// scanAIReportCache centralizes row mapping so create and get keep the same
// column order and response shape.
func scanAIReportCache(row pgx.Row) (AIReportCache, error) {
	var report AIReportCache
	err := row.Scan(
		&report.ID,
		&report.FamilyID,
		&report.BabyID,
		&report.ReportType,
		&report.RangeStart,
		&report.RangeEnd,
		&report.InputHash,
		&report.PromptVersion,
		&report.InputSchemaVersion,
		&report.OutputSchemaVersion,
		&report.Model,
		&report.ContentJSON,
		&report.CreatedAt,
	)
	return report, err
}

// UpsertUserByEmail returns the existing user with this email, creating one
// if none exists yet. Email is the stable login identity.
func (s *PostgresStore) UpsertUserByEmail(ctx context.Context, email string) (User, error) {
	const query = `
		INSERT INTO users (id, email)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
		RETURNING id, email, display_name, created_at
	`

	var u User
	err := s.pool.QueryRow(ctx, query, uuid.New(), email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return User{}, fmt.Errorf("upserting user by email: %w", err)
	}

	return u, nil
}

// GetUser returns a login-capable user by id.
func (s *PostgresStore) GetUser(ctx context.Context, id uuid.UUID) (User, error) {
	const query = `
		SELECT id, email, display_name, created_at
		FROM users
		WHERE id = $1
	`

	var u User
	err := s.pool.QueryRow(ctx, query, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("getting user: %w", err)
	}

	return u, nil
}

// UpdateUserDisplayName stores an optional account display name for user id.
func (s *PostgresStore) UpdateUserDisplayName(ctx context.Context, id uuid.UUID, displayName string) (User, error) {
	const query = `
		UPDATE users
		SET display_name = $1
		WHERE id = $2
		RETURNING id, email, display_name, created_at
	`

	var u User
	err := s.pool.QueryRow(ctx, query, displayName, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("updating user display name: %w", err)
	}

	return u, nil
}

// GetFamilyMembership returns userID's family membership, if any. A user can
// hold multiple rows (one active membership plus separate pending invites in
// other families, per idx_family_members_one_active_per_user's comment), so
// the active row - being the unique, authoritative one - is always preferred
// over an arbitrary pending invite.
func (s *PostgresStore) GetFamilyMembership(ctx context.Context, userID uuid.UUID) (FamilyMembership, error) {
	const query = `
		SELECT family_id, role, status, daily_report_email_enabled
		FROM family_members
		WHERE user_id = $1
		ORDER BY (status = 'active') DESC, created_at ASC
		LIMIT 1
	`

	var m FamilyMembership
	var familyID uuid.UUID
	var role, status string
	err := s.pool.QueryRow(ctx, query, userID).Scan(&familyID, &role, &status, &m.DailyReportEmailEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return FamilyMembership{Found: false}, nil
	}
	if err != nil {
		return FamilyMembership{}, fmt.Errorf("getting family membership: %w", err)
	}

	m.Found = true
	m.FamilyID = &familyID
	m.Role = MembershipRole(role)
	m.Status = MembershipStatus(status)
	return m, nil
}

// GetFamilyMembershipForFamily returns userID's membership in familyID, if
// any. Unlike GetFamilyMembership, this does not prefer a different active
// membership over a pending invite: callers already know the family they are
// authorizing against.
func (s *PostgresStore) GetFamilyMembershipForFamily(ctx context.Context, userID, familyID uuid.UUID) (FamilyMembership, error) {
	const query = `
		SELECT role, status, daily_report_email_enabled
		FROM family_members
		WHERE user_id = $1 AND family_id = $2
	`

	var role, status string
	var dailyReportEmailEnabled bool
	err := s.pool.QueryRow(ctx, query, userID, familyID).Scan(&role, &status, &dailyReportEmailEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return FamilyMembership{Found: false}, nil
	}
	if err != nil {
		return FamilyMembership{}, fmt.Errorf("getting family membership for family: %w", err)
	}

	return FamilyMembership{
		Found:                   true,
		FamilyID:                &familyID,
		Role:                    MembershipRole(role),
		Status:                  MembershipStatus(status),
		DailyReportEmailEnabled: dailyReportEmailEnabled,
	}, nil
}

// HasPendingInviteOutsideFamily reports whether userID has an outstanding
// invite to any family other than excludeFamilyID. Used only for product
// guidance while multi-timeline switching is intentionally deferred.
func (s *PostgresStore) HasPendingInviteOutsideFamily(ctx context.Context, userID, excludeFamilyID uuid.UUID) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM family_members
			WHERE user_id = $1
				AND family_id <> $2
				AND status = $3
		)
	`

	var found bool
	if err := s.pool.QueryRow(ctx, query, userID, excludeFamilyID, MembershipStatusInvited).Scan(&found); err != nil {
		return false, fmt.Errorf("checking pending invite: %w", err)
	}
	return found, nil
}

// CreateFamilyWithOwner creates a new family and makes userID its active
// owner, atomically. familyName is never shown to users — the family is a
// backend-only grouping, not a product concept. A single data-modifying CTE
// keeps both inserts in one round trip and one implicit statement-level
// transaction, rather than an explicit Begin/Exec/Exec/Commit. If userID
// already has an active membership elsewhere (including the losing side of
// two concurrent calls for the same brand-new user),
// idx_family_members_one_active_per_user rejects the insert and this returns
// ErrActiveMembershipExists rather than silently creating a second active
// membership.
func (s *PostgresStore) CreateFamilyWithOwner(ctx context.Context, userID uuid.UUID, familyName string) (uuid.UUID, error) {
	const query = `
		WITH new_family AS (
			INSERT INTO families (id, name) VALUES ($1, $2)
			RETURNING id
		)
		INSERT INTO family_members (family_id, user_id, role, status, daily_report_email_enabled)
		SELECT id, $3, $4, $5, true FROM new_family
	`

	familyID := uuid.New()
	if _, err := s.pool.Exec(ctx, query, familyID, familyName, userID, MembershipRoleOwner, MembershipStatusActive); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return uuid.Nil, ErrActiveMembershipExists
		}
		return uuid.Nil, fmt.Errorf("creating family with owner: %w", err)
	}

	return familyID, nil
}

// UpdateDailyReportEmailPreference stores the scheduled daily email opt-in for
// the current owner. The WHERE clause deliberately enforces the current
// product rule: only active owners can opt in or out.
func (s *PostgresStore) UpdateDailyReportEmailPreference(ctx context.Context, familyID, userID uuid.UUID, enabled bool) (FamilyMembership, error) {
	const query = `
		UPDATE family_members
		SET daily_report_email_enabled = $1
		WHERE family_id = $2
			AND user_id = $3
			AND role = $4
			AND status = $5
		RETURNING role, status, daily_report_email_enabled
	`

	var role, status string
	var dailyReportEmailEnabled bool
	err := s.pool.QueryRow(ctx, query, enabled, familyID, userID, MembershipRoleOwner, MembershipStatusActive).Scan(&role, &status, &dailyReportEmailEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return FamilyMembership{}, ErrNotFound
	}
	if err != nil {
		return FamilyMembership{}, fmt.Errorf("updating daily report email preference: %w", err)
	}

	return FamilyMembership{
		Found:                   true,
		FamilyID:                &familyID,
		Role:                    MembershipRole(role),
		Status:                  MembershipStatus(status),
		DailyReportEmailEnabled: dailyReportEmailEnabled,
	}, nil
}

// CreateInvite resolves (creating if necessary) the invitee's user record by
// email and grants them a pending (status=invited) membership in familyID,
// as a single atomic statement — a failure partway through (e.g. a bad
// familyID failing the FK check) can't leave behind a user row with no
// invite the way two separate calls could. Re-inviting an already
// invited/active (family_id, user_id) pair is a no-op (ON CONFLICT DO
// NOTHING), so retries and double-sends are safe rather than erroring on
// family_members' primary key. Multiple pending invites for the same user
// across different families are still allowed (see
// idx_family_members_one_active_per_user's comment) — this never creates an
// active row, so it can't violate that constraint.
func (s *PostgresStore) CreateInvite(ctx context.Context, familyID uuid.UUID, email string) error {
	const query = `
		WITH upserted_user AS (
			INSERT INTO users (id, email)
			VALUES ($1, $2)
			ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
			RETURNING id
		)
		INSERT INTO family_members (family_id, user_id, role, status)
		SELECT $3, id, $4, $5 FROM upserted_user
		ON CONFLICT (family_id, user_id) DO NOTHING
	`

	if _, err := s.pool.Exec(ctx, query, uuid.New(), email, familyID, MembershipRoleMember, MembershipStatusInvited); err != nil {
		return fmt.Errorf("creating invite: %w", err)
	}

	return nil
}

// ListTimelineMembers returns every active or invited user with access to
// familyID, ordered by active owners first, then active members, then invites.
func (s *PostgresStore) ListTimelineMembers(ctx context.Context, familyID uuid.UUID) ([]TimelineMember, error) {
	const query = `
		SELECT u.id, u.email, fm.role, fm.status, COALESCE(fm.relationship, '')
		FROM family_members fm
		JOIN users u ON u.id = fm.user_id
		WHERE fm.family_id = $1
		ORDER BY
			(fm.status = 'active') DESC,
			(fm.role = 'owner') DESC,
			fm.created_at ASC,
			u.email ASC
	`

	rows, err := s.pool.Query(ctx, query, familyID)
	if err != nil {
		return nil, fmt.Errorf("listing timeline members: %w", err)
	}
	defer rows.Close()

	var members []TimelineMember
	for rows.Next() {
		var m TimelineMember
		var role, status string
		if err := rows.Scan(&m.UserID, &m.Email, &role, &status, &m.Relationship); err != nil {
			return nil, fmt.Errorf("scanning timeline member: %w", err)
		}
		m.Role = MembershipRole(role)
		m.Status = MembershipStatus(status)
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating timeline members: %w", err)
	}

	return members, nil
}

// UpdateTimelineMemberRelationship stores a user's relationship label for a
// family timeline. Empty or all-whitespace input clears the label.
func (s *PostgresStore) UpdateTimelineMemberRelationship(ctx context.Context, familyID, userID uuid.UUID, relationship string) error {
	const query = `
		UPDATE family_members
		SET relationship = NULLIF($1, '')
		WHERE family_id = $2 AND user_id = $3
	`

	tag, err := s.pool.Exec(ctx, query, strings.TrimSpace(relationship), familyID, userID)
	if err != nil {
		return fmt.Errorf("updating timeline member relationship: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// RemoveTimelineMember removes a family_members row. Callers are responsible
// for applying access-management policy before deletion, including revoking
// auth-service sessions before removing an active member.
func (s *PostgresStore) RemoveTimelineMember(ctx context.Context, familyID, userID uuid.UUID) error {
	const query = `DELETE FROM family_members WHERE family_id = $1 AND user_id = $2`

	tag, err := s.pool.Exec(ctx, query, familyID, userID)
	if err != nil {
		return fmt.Errorf("removing timeline member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ActivateInvitedMembership flips a pending invite to active — called when
// an invited user completes their first login. ErrNotFound is returned if
// no matching invited row exists.
func (s *PostgresStore) ActivateInvitedMembership(ctx context.Context, userID, familyID uuid.UUID) error {
	const query = `
		UPDATE family_members
		SET status = $1
		WHERE user_id = $2 AND family_id = $3 AND status = $4
	`

	tag, err := s.pool.Exec(ctx, query, MembershipStatusActive, userID, familyID, MembershipStatusInvited)
	if err != nil {
		return fmt.Errorf("activating membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
