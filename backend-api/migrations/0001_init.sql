CREATE TABLE IF NOT EXISTS families (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS babies (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id),
  name TEXT NOT NULL,
  timezone TEXT NOT NULL DEFAULT 'Australia/Adelaide',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS events (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id),
  baby_id UUID NOT NULL REFERENCES babies(id),
  event_type TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  attributes JSONB NOT NULL DEFAULT '{}',
  source TEXT NOT NULL DEFAULT 'web',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_events_baby_type_occurred
  ON events (baby_id, event_type, occurred_at DESC);

INSERT INTO families (id, name)
VALUES ('11111111-1111-1111-1111-111111111111', 'Cip & Jenny')
ON CONFLICT DO NOTHING;

INSERT INTO babies (id, family_id, name, timezone)
VALUES (
  '22222222-2222-2222-2222-222222222222',
  '11111111-1111-1111-1111-111111111111',
  'YauYau',
  'Australia/Adelaide'
)
ON CONFLICT DO NOTHING;
