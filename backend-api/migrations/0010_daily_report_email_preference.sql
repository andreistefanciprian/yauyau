ALTER TABLE family_members
  ADD COLUMN IF NOT EXISTS daily_report_email_enabled BOOLEAN;

UPDATE family_members
SET daily_report_email_enabled = (role = 'owner' AND status = 'active')
WHERE daily_report_email_enabled IS NULL;

ALTER TABLE family_members
  ALTER COLUMN daily_report_email_enabled SET DEFAULT false;

ALTER TABLE family_members
  ALTER COLUMN daily_report_email_enabled SET NOT NULL;
