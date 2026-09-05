-- Gateway binaries originally embedded a narrower schema than migrations 001-005.
-- Preserve existing records and reconcile that schema without replaying history.
ALTER TABLE sites ALTER COLUMN live_version_id TYPE VARCHAR(255);
ALTER TABLE sites ALTER COLUMN preview_version_id TYPE VARCHAR(255);
ALTER TABLE versions ALTER COLUMN build_id TYPE VARCHAR(255);
ALTER TABLE versions ALTER COLUMN status SET DEFAULT 'pending';

CREATE INDEX IF NOT EXISTS idx_site_aliases_alias ON site_aliases(alias);
CREATE INDEX IF NOT EXISTS idx_versions_build_id ON versions(build_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'users'::regclass AND conname = 'email_or_oauth'
    ) THEN
        ALTER TABLE users ADD CONSTRAINT email_or_oauth CHECK (
            password_hash IS NOT NULL OR
            (oauth_provider IS NOT NULL AND oauth_id IS NOT NULL)
        ) NOT VALID;
    END IF;
END
$$;

-- Invalid legacy rows fail the upgrade visibly; never delete or rewrite users.
ALTER TABLE users VALIDATE CONSTRAINT email_or_oauth;
