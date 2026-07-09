package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

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

func (s *PostgresStore) GetCurrentBaby(ctx context.Context) (Baby, error) {
	const query = `SELECT id, family_id, name, timezone FROM babies WHERE id = $1`

	var baby Baby
	err := s.pool.QueryRow(ctx, query, BabyID).
		Scan(&baby.ID, &baby.FamilyID, &baby.Name, &baby.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Baby{}, ErrNotFound
	}
	if err != nil {
		return Baby{}, fmt.Errorf("get current baby: %w", err)
	}

	return baby, nil
}

func (s *PostgresStore) CreateEvent(ctx context.Context, eventType string, attributes map[string]any, occurredAt time.Time) (Event, error) {
	id := uuid.New()

	const query = `
		INSERT INTO events (id, family_id, baby_id, event_type, occurred_at, attributes, source)
		VALUES ($1, $2, $3, $4, $5, $6, 'web')
		RETURNING created_at
	`

	var createdAt time.Time
	err := s.pool.QueryRow(ctx, query, id, FamilyID, BabyID, eventType, occurredAt, attributes).Scan(&createdAt)
	if err != nil {
		return Event{}, fmt.Errorf("inserting %s event: %w", eventType, err)
	}

	return Event{
		ID:         id,
		BabyID:     BabyID,
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
func (s *PostgresStore) DeleteEvent(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM events WHERE id = $1 AND baby_id = $2`

	tag, err := s.pool.Exec(ctx, query, id, BabyID)
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
func (s *PostgresStore) ListAllEvents(ctx context.Context, limit int) ([]Event, error) {
	const query = `
		SELECT id, baby_id, event_type, attributes, occurred_at, created_at
		FROM events
		WHERE baby_id = $1
		ORDER BY occurred_at DESC
		LIMIT $2
	`

	rows, err := s.pool.Query(ctx, query, BabyID, limit)
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
