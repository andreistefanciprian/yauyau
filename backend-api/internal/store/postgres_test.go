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
	if err := Migrate(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

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
	if membership.FamilyID == nil || *membership.FamilyID != familyID {
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
	if membership.FamilyID == nil || *membership.FamilyID != firstFamilyID {
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

	// Simulate an invite (PR11 will do this via a real invite endpoint):
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

func TestCreateInvite_CreatesUserAndMembership(t *testing.T) {
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

	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	invitee, err := s.UpsertUserByEmail(ctx, inviteeEmail)
	if err != nil {
		t.Fatalf("resolve invitee: %v", err)
	}

	membership, err := s.GetFamilyMembership(ctx, invitee.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if !membership.Found || membership.Status != MembershipStatusInvited {
		t.Fatalf("expected an invited membership, got %+v", membership)
	}
	if membership.FamilyID == nil || *membership.FamilyID != familyID {
		t.Fatalf("expected family id %v, got %v", familyID, membership.FamilyID)
	}
}

// TestCreateInvite_IdempotentOnRepeat guards the ON CONFLICT DO NOTHING added
// to CreateInvite: a retried or double-sent invite for the same
// (family_id, email) pair must succeed as a no-op rather than fail on
// family_members' primary key.
func TestCreateInvite_IdempotentOnRepeat(t *testing.T) {
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

	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("first invite: %v", err)
	}
	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("expected a repeat invite for the same email/family to be a no-op, got: %v", err)
	}
}

// TestGetFamilyMembership_PrefersActiveOverInvited guards the ORDER BY added
// to GetFamilyMembership: a user who is active in one family and separately
// invited to another must resolve to the active membership, not whichever
// row Postgres happens to return first.
func TestGetFamilyMembership_PrefersActiveOverInvited(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	user, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	activeFamilyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "active family")
	if err != nil {
		t.Fatalf("create active family: %v", err)
	}

	otherOwner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert other owner: %v", err)
	}
	invitedFamilyID, err := s.CreateFamilyWithOwner(ctx, otherOwner.ID, "invited family")
	if err != nil {
		t.Fatalf("create invited family: %v", err)
	}

	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM families WHERE id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{user.ID, otherOwner.ID})
	})

	// user is already active in activeFamilyID; inviting them into a second
	// family gives them two family_members rows.
	if err := s.CreateInvite(ctx, invitedFamilyID, user.Email); err != nil {
		t.Fatalf("invite into second family: %v", err)
	}

	membership, err := s.GetFamilyMembership(ctx, user.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.Status != MembershipStatusActive {
		t.Fatalf("expected the active membership to be preferred, got status %q", membership.Status)
	}
	if membership.FamilyID == nil || *membership.FamilyID != activeFamilyID {
		t.Fatalf("expected family id %v (the active one), got %v", activeFamilyID, membership.FamilyID)
	}
}

func TestGetFamilyMembershipForFamily_ReturnsSpecificMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	user, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	activeFamilyID, err := s.CreateFamilyWithOwner(ctx, user.ID, "active family")
	if err != nil {
		t.Fatalf("create active family: %v", err)
	}

	otherOwner, err := s.UpsertUserByEmail(ctx, testEmail(t))
	if err != nil {
		t.Fatalf("upsert other owner: %v", err)
	}
	invitedFamilyID, err := s.CreateFamilyWithOwner(ctx, otherOwner.ID, "invited family")
	if err != nil {
		t.Fatalf("create invited family: %v", err)
	}

	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM families WHERE id = ANY($1)`, []uuid.UUID{activeFamilyID, invitedFamilyID})
		execCleanup(t, s, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{user.ID, otherOwner.ID})
	})

	if err := s.CreateInvite(ctx, invitedFamilyID, user.Email); err != nil {
		t.Fatalf("invite into second family: %v", err)
	}

	membership, err := s.GetFamilyMembershipForFamily(ctx, user.ID, invitedFamilyID)
	if err != nil {
		t.Fatalf("get membership for invited family: %v", err)
	}
	if !membership.Found {
		t.Fatal("expected invited membership to be found")
	}
	if membership.FamilyID == nil || *membership.FamilyID != invitedFamilyID {
		t.Fatalf("expected family id %v, got %v", invitedFamilyID, membership.FamilyID)
	}
	if membership.Status != MembershipStatusInvited {
		t.Fatalf("expected status %q, got %q", MembershipStatusInvited, membership.Status)
	}
}

func TestListTimelineMembers(t *testing.T) {
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

	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	members, err := s.ListTimelineMembers(ctx, familyID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d: %+v", len(members), members)
	}
	if members[0].UserID != owner.ID || members[0].Email != owner.Email || members[0].Role != MembershipRoleOwner || members[0].Status != MembershipStatusActive {
		t.Fatalf("expected owner first, got %+v", members[0])
	}
	if members[1].Email != inviteeEmail || members[1].Role != MembershipRoleMember || members[1].Status != MembershipStatusInvited {
		t.Fatalf("expected invited member second, got %+v", members[1])
	}
}

func TestUpdateTimelineMemberRelationship(t *testing.T) {
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
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	if err := s.UpdateTimelineMemberRelationship(ctx, familyID, owner.ID, "  Mum  "); err != nil {
		t.Fatalf("update relationship: %v", err)
	}

	members, err := s.ListTimelineMembers(ctx, familyID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || members[0].Relationship != "Mum" {
		t.Fatalf("expected relationship to be trimmed and stored, got %+v", members)
	}

	if err := s.UpdateTimelineMemberRelationship(ctx, familyID, owner.ID, " "); err != nil {
		t.Fatalf("clear relationship: %v", err)
	}
	members, err = s.ListTimelineMembers(ctx, familyID)
	if err != nil {
		t.Fatalf("list members after clear: %v", err)
	}
	if len(members) != 1 || members[0].Relationship != "" {
		t.Fatalf("expected relationship to be cleared, got %+v", members)
	}
}

func TestRemoveTimelineMember(t *testing.T) {
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

	inviteeEmail := testEmail(t)
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE email = ANY($1)`, []string{owner.Email, inviteeEmail})
	})

	if err := s.CreateInvite(ctx, familyID, inviteeEmail); err != nil {
		t.Fatalf("create invite: %v", err)
	}
	invitee, err := s.UpsertUserByEmail(ctx, inviteeEmail)
	if err != nil {
		t.Fatalf("resolve invitee: %v", err)
	}

	if err := s.RemoveTimelineMember(ctx, familyID, invitee.ID); err != nil {
		t.Fatalf("remove member: %v", err)
	}

	members, err := s.ListTimelineMembers(ctx, familyID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || members[0].UserID != owner.ID {
		t.Fatalf("expected only owner to remain, got %+v", members)
	}
	if err := s.RemoveTimelineMember(ctx, familyID, invitee.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on repeat remove, got %v", err)
	}
}

func TestRemoveTimelineMemberDeletesActiveMember(t *testing.T) {
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
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	if err := s.RemoveTimelineMember(ctx, familyID, owner.ID); err != nil {
		t.Fatalf("remove active member: %v", err)
	}

	membership, err := s.GetFamilyMembershipForFamily(ctx, owner.ID, familyID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if membership.Found {
		t.Fatalf("expected active membership to be removed, got %+v", membership)
	}
}

func TestCreateBaby(t *testing.T) {
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
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	baby, err := s.CreateBaby(ctx, familyID, "YauYau", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}
	if baby.FamilyID != familyID {
		t.Fatalf("expected family id %v, got %v", familyID, baby.FamilyID)
	}
	if baby.Name != "YauYau" {
		t.Fatalf("expected name %q, got %q", "YauYau", baby.Name)
	}
	if baby.Timezone != "Australia/Adelaide" {
		t.Fatalf("expected timezone %q, got %q", "Australia/Adelaide", baby.Timezone)
	}
}

func TestGetBaby(t *testing.T) {
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
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	created, err := s.CreateBaby(ctx, familyID, "YauYau", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create baby: %v", err)
	}

	got, err := s.GetBaby(ctx, created.ID)
	if err != nil {
		t.Fatalf("get baby: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected baby id %v, got %v", created.ID, got.ID)
	}
	if got.FamilyID != familyID {
		t.Fatalf("expected family id %v, got %v", familyID, got.FamilyID)
	}
}

// TestGetCurrentBaby_ReturnsFirstCreated guards the "current baby" ordering
// for a family with more than one baby (e.g. twins added one after another) —
// it must consistently return the first one created, not an arbitrary row.
func TestGetCurrentBaby_ReturnsFirstCreated(t *testing.T) {
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
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM babies WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	first, err := s.CreateBaby(ctx, familyID, "First", "Australia/Adelaide")
	if err != nil {
		t.Fatalf("create first baby: %v", err)
	}
	if _, err := s.CreateBaby(ctx, familyID, "Second", "Australia/Adelaide"); err != nil {
		t.Fatalf("create second baby: %v", err)
	}

	current, err := s.GetCurrentBaby(ctx, familyID)
	if err != nil {
		t.Fatalf("get current baby: %v", err)
	}
	if current.ID != first.ID {
		t.Fatalf("expected the first-created baby %v, got %v", first.ID, current.ID)
	}
}

func TestGetCurrentBaby_NotFound(t *testing.T) {
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
	t.Cleanup(func() {
		execCleanup(t, s, `DELETE FROM family_members WHERE family_id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM families WHERE id = $1`, familyID)
		execCleanup(t, s, `DELETE FROM users WHERE id = $1`, owner.ID)
	})

	if _, err := s.GetCurrentBaby(ctx, familyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a family with no babies, got %v", err)
	}
}
