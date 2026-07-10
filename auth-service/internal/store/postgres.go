package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

// CreateMagicLink inserts a magic_links row for userID. Only tokenHash (a
// SHA-256 hex digest) is stored — the raw token exists only in the emailed
// link, never at rest. expires_at is computed by Postgres's own clock
// (NOW() + INTERVAL), not Go's time.Now(), matching ConsumeMagicLink's
// expiry check against the same clock.
func (s *PostgresStore) CreateMagicLink(ctx context.Context, userID uuid.UUID, tokenHash string) error {
	const query = `
		INSERT INTO magic_links (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, NOW() + INTERVAL '15 minutes')
	`

	if _, err := s.pool.Exec(ctx, query, uuid.New(), userID, tokenHash); err != nil {
		return fmt.Errorf("creating magic link: %w", err)
	}

	return nil
}

// ConsumeMagicLink atomically marks the magic link matching tokenHash as
// used and returns the user_id it belonged to — a single UPDATE ...
// RETURNING rather than a SELECT then UPDATE, so a real click and a
// prefetch/double-tap racing for the same token can never both succeed.
// ErrNotFound covers "no such token," "already used," and "expired" alike;
// none of those cases should distinguish themselves to the caller.
func (s *PostgresStore) ConsumeMagicLink(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	const query = `
		UPDATE magic_links
		SET used_at = NOW()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		RETURNING user_id
	`

	var userID uuid.UUID
	err := s.pool.QueryRow(ctx, query, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("consuming magic link: %w", err)
	}

	return userID, nil
}

// CreateSession inserts a new session for userID. familyID is nil for a
// brand-new user with no family membership yet (resolved later via
// AttachFamily, once onboarding creates one) — sessions.family_id is
// nullable specifically to represent that state.
func (s *PostgresStore) CreateSession(ctx context.Context, userID uuid.UUID, familyID *uuid.UUID) (uuid.UUID, error) {
	const query = `
		INSERT INTO sessions (id, user_id, family_id, expires_at)
		VALUES ($1, $2, $3, NOW() + INTERVAL '30 days')
	`

	sessionID := uuid.New()
	if _, err := s.pool.Exec(ctx, query, sessionID, userID, familyID); err != nil {
		return uuid.Nil, fmt.Errorf("creating session: %w", err)
	}

	return sessionID, nil
}

// WriteAuditLog records a login/logout event. sessionID is nil for events
// that don't yet have one (there is none until CreateSession succeeds).
func (s *PostgresStore) WriteAuditLog(ctx context.Context, userID uuid.UUID, sessionID *uuid.UUID, eventType string) error {
	const query = `
		INSERT INTO audit_logs (id, user_id, session_id, event_type)
		VALUES ($1, $2, $3, $4)
	`

	if _, err := s.pool.Exec(ctx, query, uuid.New(), userID, sessionID, eventType); err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}

	return nil
}
