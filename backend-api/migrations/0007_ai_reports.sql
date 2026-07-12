CREATE TABLE IF NOT EXISTS ai_reports (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id),
  baby_id UUID NOT NULL REFERENCES babies(id),
  report_type TEXT NOT NULL,
  range_start TIMESTAMPTZ NOT NULL,
  range_end TIMESTAMPTZ NOT NULL,
  input_hash TEXT NOT NULL,
  model TEXT NOT NULL,
  content JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_reports_unique_input
  ON ai_reports (family_id, baby_id, report_type, range_start, range_end, input_hash);
