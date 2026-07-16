CREATE TABLE IF NOT EXISTS ai_report_email_deliveries (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id),
  baby_id UUID NOT NULL REFERENCES babies(id),
  recipient_user_id UUID NOT NULL REFERENCES users(id),
  recipient_email TEXT NOT NULL,
  report_type TEXT NOT NULL,
  range_start TIMESTAMPTZ NOT NULL,
  range_end TIMESTAMPTZ NOT NULL,
  scheduled_for TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL,
  ai_report_cache_id UUID REFERENCES ai_report_cache(id) ON DELETE SET NULL,
  provider_message_id TEXT,
  error_message TEXT,
  attempted_at TIMESTAMPTZ,
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (
    family_id,
    baby_id,
    recipient_user_id,
    report_type,
    range_start,
    range_end,
    scheduled_for
  )
);

CREATE INDEX IF NOT EXISTS idx_ai_report_email_deliveries_status
  ON ai_report_email_deliveries (status, scheduled_for);

CREATE INDEX IF NOT EXISTS idx_ai_report_email_deliveries_created_at
  ON ai_report_email_deliveries (created_at);
