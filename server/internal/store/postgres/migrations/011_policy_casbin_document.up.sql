ALTER TABLE policies
    ADD COLUMN IF NOT EXISTS casbin_document TEXT;
