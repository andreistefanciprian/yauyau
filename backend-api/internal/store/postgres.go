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

// GetBaby returns the baby with id, regardless of which family it belongs to.
// Callers use the returned FamilyID for authorization checks before exposing
// or mutating anything through user-facing routes.
func (s *PostgresStore) GetBaby(ctx context.Context, id uuid.UUID) (Baby, error) {
	const query = `
		SELECT id, family_id, name, timezone
		FROM babies
		WHERE id = $1
	`

	var baby Baby
	err := s.pool.QueryRow(ctx, query, id).
		Scan(&baby.ID, &baby.FamilyID, &baby.Name, &baby.Timezone)
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
		SELECT id, family_id, name, timezone
		FROM babies
		WHERE family_id = $1
		ORDER BY created_at ASC
		LIMIT 1
	`

	var baby Baby
	err := s.pool.QueryRow(ctx, query, familyID).
		Scan(&baby.ID, &baby.FamilyID, &baby.Name, &baby.Timezone)
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
		RETURNING id, family_id, name, timezone
	`

	baby := Baby{ID: uuid.New()}
	err := s.pool.QueryRow(ctx, query, baby.ID, familyID, name, timezone).
		Scan(&baby.ID, &baby.FamilyID, &baby.Name, &baby.Timezone)
	if err != nil {
		return Baby{}, fmt.Errorf("creating baby: %w", err)
	}

	return baby, nil
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
func (s *PostgresStore) ListAllEvents(ctx context.Context, familyID, babyID uuid.UUID, limit int) ([]Event, error) {
	const query = `
		SELECT id, baby_id, event_type, attributes, occurred_at, created_at
		FROM events
		WHERE family_id = $1 AND baby_id = $2
		ORDER BY occurred_at DESC
		LIMIT $3
	`

	rows, err := s.pool.Query(ctx, query, familyID, babyID, limit)
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

// UpsertUserByEmail returns the existing user with this email, creating one
// if none exists yet. Email is the only identity a user has.
func (s *PostgresStore) UpsertUserByEmail(ctx context.Context, email string) (User, error) {
	const query = `
		INSERT INTO users (id, email)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
		RETURNING id, email, created_at
	`

	var u User
	err := s.pool.QueryRow(ctx, query, uuid.New(), email).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		return User{}, fmt.Errorf("upserting user by email: %w", err)
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
		SELECT family_id, role, status
		FROM family_members
		WHERE user_id = $1
		ORDER BY (status = 'active') DESC, created_at ASC
		LIMIT 1
	`

	var m FamilyMembership
	var familyID uuid.UUID
	var role, status string
	err := s.pool.QueryRow(ctx, query, userID).Scan(&familyID, &role, &status)
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
		SELECT role, status
		FROM family_members
		WHERE user_id = $1 AND family_id = $2
	`

	var role, status string
	err := s.pool.QueryRow(ctx, query, userID, familyID).Scan(&role, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return FamilyMembership{Found: false}, nil
	}
	if err != nil {
		return FamilyMembership{}, fmt.Errorf("getting family membership for family: %w", err)
	}

	return FamilyMembership{
		Found:    true,
		FamilyID: &familyID,
		Role:     MembershipRole(role),
		Status:   MembershipStatus(status),
	}, nil
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
		INSERT INTO family_members (family_id, user_id, role, status)
		SELECT id, $3, $4, $5 FROM new_family
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
