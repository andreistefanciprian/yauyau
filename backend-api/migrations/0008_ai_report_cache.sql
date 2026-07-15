CREATE TABLE IF NOT EXISTS ai_report_cache (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id),
  baby_id UUID NOT NULL REFERENCES babies(id),
  report_type TEXT NOT NULL,
  range_start TIMESTAMPTZ NOT NULL,
  range_end TIMESTAMPTZ NOT NULL,
  input_hash TEXT NOT NULL,
  prompt_version TEXT NOT NULL,
  input_schema_version TEXT NOT NULL,
  output_schema_version TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  content_json JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (family_id, baby_id, report_type, range_start, range_end, input_hash)
);

CREATE INDEX IF NOT EXISTS idx_ai_report_cache_created_at
  ON ai_report_cache (created_at);
