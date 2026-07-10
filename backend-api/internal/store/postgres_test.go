package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
)

// These are integration tests, not pure unit tests — the store package is a
// thin SQL wrapper, so there's no meaningful logic to test without a real
// database. They connect to the local Postgres started by
// `docker compose up postgres` (or `task up`) and skip, rather than fail,
// if it isn't reachable, so `go test ./...` still works in environments
// without Docker running.
func testStore(t *testing.T) *PostgresStore {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Same default docker-compose.yml/.env.example produce for local dev,
		// kept here (rather than only reading the env var) so `go test ./...`
		// works out of the box against `docker compose up postgres` without
		// requiring DATABASE_URL to be exported manually first.
		dbURL = "postgres://postgres:postgres@localhost:5432/yauli?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping: could not connect to postgres at %s (is `docker compose up postgres` running?): %v", dbURL, err)
	}
	t.Cleanup(pool.Close)

	return NewPostgresStore(pool)
}

// testEmail returns a unique email per call so tests can run repeatedly
// (and in parallel) without colliding on the `users.email` unique
// constraint.
func testEmail(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-%s@example.com", uuid.NewString())
}

// execCleanup runs a teardown statement and reports (rather than silently
// swallows) any error, so a failed cleanup surfaces at its source instead of
// causing a confusing failure in some later, unrelated test run.
func execCleanup(t *testing.T, s *PostgresStore, query string, args ...any) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), query, args...); err != nil {
		t.Errorf("cleanup %q: %v", query, err)
	}
}

func TestUpsertUserByEmail_Idempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)
	t.Cleanup(func() { execCleanup(t, s, `DELETE FROM users WHERE email = $1`, email) })

	first, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if first.Email != email {
		t.Fatalf("expected email %q, got %q", email, first.Email)
	}

	second, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the same user id on a repeat upsert, got %v vs %v", second.ID, first.ID)
	}
}

func TestGetFamilyMembership_NotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)
	t.Cleanup(func() { execCleanup(t, s, `DELETE FROM users WHERE email = $1`, email) })

	user, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	membership, err := s.GetFamilyMembership(ctx, user.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.Found {
		t.Fatalf("expected no membership for a fresh user, got %+v", membership)
	}
}

func TestCreateFamilyWithOwner(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := testEmail(t)

	user, err := s.UpsertUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	familyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, user.ID)
	})

	membership, err := s.GetFamilyMembership(ctx, user.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if !membership.Found {
		t.Fatalf("expected a membership to exist after CreateFamilyWithOwner")
	}
	if membership.FamilyID != familyID {
		t.Fatalf("expected family id %v, got %v", familyID, membership.FamilyID)
	}
	if membership.Role != MembershipRoleOwner {
		t.Fatalf("expected role %q, got %q", MembershipRoleOwner, membership.Role)
	}
	if membership.Status != MembershipStatusActive {
		t.Fatalf("expected status %q, got %q", MembershipStatusActive, membership.Status)
	}
}

// TestCreateFamilyWithOwner_RejectsSecondActiveMembership guards the
// idx_family_members_one_active_per_user constraint: a user who already has
// an active membership must not be able to end up with a second one (e.g. a
// retried "create family" request), rather than silently ending up owner of
// two families with GetFamilyMembership returning an arbitrary one of them.
func TestCreateFamilyWithOwner_RejectsSecondActiveMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	user, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	firstFamilyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "first family")
	if err != nil {
		t.Fatalf("create first family: %v", err)
	}
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, firstFamilyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, firstFamilyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, user.ID)
	})

	if _, err := s.CreateFamilyWithOwner(ctx, user.ID, "second family"); err == nil {
		t.Fatalf("expected creating a second family for an already-active user to fail, got no error")
	}

	// The rejected second attempt must not have left anything behind: the
	// user should still resolve to exactly the first family.
	membership, err := s.GetFamilyMembership(ctx, user.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.FamilyID != firstFamilyID {
		t.Fatalf("expected membership to still point at the first family %v, got %v", firstFamilyID, membership.FamilyID)
	}
}

func TestActivateInvitedMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	owner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	familyID, err := s.CreateFamilyWithOwner(ctx, owner.ID, "test family")
	if err != nil {
		t.Fatalf("create family: %v", err)
	}

	invitee, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert invitee: %v", err)
	}

	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{owner.ID, invitee.ID})
	})

	// Simulate an invite (PR13 will do this via a real invite endpoint):
	// a pending family_members row for a user who hasn't logged in yet.
	if _, err := s.pool.Exec(ctx, `INSERT INTO family_members (family_id, user_id, role, status) VALUES ($1, $2, $3, $4)`,
		familyID, invitee.ID, MembershipRoleMember, MembershipStatusInvited); err != nil {
		t.Fatalf("insert invited row: %v", err)
	}

	if err := s.ActivateInvitedMembership(ctx, invitee.ID, familyID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	membership, err := s.GetFamilyMembership(ctx, invitee.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.Status != MembershipStatusActive {
		t.Fatalf("expected status %q after activation, got %q", MembershipStatusActive, membership.Status)
	}
	if membership.Role != MembershipRoleMember {
		t.Fatalf("expected role %q, got %q", MembershipRoleMember, membership.Role)
	}
}

func TestActivateInvitedMembership_NotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// No invited row exists for these arbitrary, never-inserted ids.
	err := s.ActivateInvitedMembership(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
