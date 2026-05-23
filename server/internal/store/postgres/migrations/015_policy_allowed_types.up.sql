ALTER TABLE policies
  ADD COLUMN IF NOT EXISTS allowed_types text[] NOT NULL DEFAULT ARRAY['*']::text[];

UPDATE policies
SET allowed_types = ARRAY['*']::text[]
WHERE allowed_types IS NULL OR array_length(allowed_types, 1) IS NULL;
