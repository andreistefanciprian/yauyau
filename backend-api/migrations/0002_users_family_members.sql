CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS family_members (
  family_id UUID NOT NULL REFERENCES families(id),
  user_id UUID NOT NULL REFERENCES users(id),
  role TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (family_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_family_members_user ON family_members (user_id);

-- A user can hold multiple pending invites at once (harmless — nothing is
-- decided yet), but at most one *active* membership: this is what
-- GetFamilyMembership assumes when it queries by user_id alone, and what
-- CreateFamilyWithOwner/ActivateInvitedMembership rely on the database to
-- reject rather than silently allow.
CREATE UNIQUE INDEX IF NOT EXISTS idx_family_members_one_active_per_user
  ON family_members (user_id)
  WHERE status = 'active';
