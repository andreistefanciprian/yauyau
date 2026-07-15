CREATE TABLE IF NOT EXISTS baby_latest_growth (
  family_id UUID NOT NULL REFERENCES families(id),
  baby_id UUID NOT NULL REFERENCES babies(id),
  weight_grams INTEGER,
  weight_measured_at TIMESTAMPTZ,
  length_cm DOUBLE PRECISION,
  length_measured_at TIMESTAMPTZ,
  head_circumference_cm DOUBLE PRECISION,
  head_circumference_measured_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (family_id, baby_id),
  CHECK ((weight_grams IS NULL) = (weight_measured_at IS NULL)),
  CHECK ((length_cm IS NULL) = (length_measured_at IS NULL)),
  CHECK ((head_circumference_cm IS NULL) = (head_circumference_measured_at IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_baby_latest_growth_baby
  ON baby_latest_growth (baby_id);

WITH latest_weight AS (
  SELECT DISTINCT ON (family_id, baby_id)
    family_id,
    baby_id,
    (attributes->>'weight_grams')::integer AS weight_grams,
    occurred_at
  FROM events
  WHERE event_type = 'growth_measurement'
    AND attributes ? 'weight_grams'
  ORDER BY family_id, baby_id, occurred_at DESC, id DESC
),
latest_length AS (
  SELECT DISTINCT ON (family_id, baby_id)
    family_id,
    baby_id,
    (attributes->>'length_cm')::double precision AS length_cm,
    occurred_at
  FROM events
  WHERE event_type = 'growth_measurement'
    AND attributes ? 'length_cm'
  ORDER BY family_id, baby_id, occurred_at DESC, id DESC
),
latest_head AS (
  SELECT DISTINCT ON (family_id, baby_id)
    family_id,
    baby_id,
    (attributes->>'head_circumference_cm')::double precision AS head_circumference_cm,
    occurred_at
  FROM events
  WHERE event_type = 'growth_measurement'
    AND attributes ? 'head_circumference_cm'
  ORDER BY family_id, baby_id, occurred_at DESC, id DESC
),
babies_with_growth AS (
  SELECT family_id, baby_id FROM latest_weight
  UNION
  SELECT family_id, baby_id FROM latest_length
  UNION
  SELECT family_id, baby_id FROM latest_head
)
INSERT INTO baby_latest_growth (
  family_id,
  baby_id,
  weight_grams,
  weight_measured_at,
  length_cm,
  length_measured_at,
  head_circumference_cm,
  head_circumference_measured_at,
  updated_at
)
SELECT
  babies_with_growth.family_id,
  babies_with_growth.baby_id,
  latest_weight.weight_grams,
  latest_weight.occurred_at,
  latest_length.length_cm,
  latest_length.occurred_at,
  latest_head.head_circumference_cm,
  latest_head.occurred_at,
  now()
FROM babies_with_growth
LEFT JOIN latest_weight
  ON latest_weight.family_id = babies_with_growth.family_id
  AND latest_weight.baby_id = babies_with_growth.baby_id
LEFT JOIN latest_length
  ON latest_length.family_id = babies_with_growth.family_id
  AND latest_length.baby_id = babies_with_growth.baby_id
LEFT JOIN latest_head
  ON latest_head.family_id = babies_with_growth.family_id
  AND latest_head.baby_id = babies_with_growth.baby_id
ON CONFLICT (family_id, baby_id)
DO UPDATE SET
  weight_grams = EXCLUDED.weight_grams,
  weight_measured_at = EXCLUDED.weight_measured_at,
  length_cm = EXCLUDED.length_cm,
  length_measured_at = EXCLUDED.length_measured_at,
  head_circumference_cm = EXCLUDED.head_circumference_cm,
  head_circumference_measured_at = EXCLUDED.head_circumference_measured_at,
  updated_at = EXCLUDED.updated_at;
