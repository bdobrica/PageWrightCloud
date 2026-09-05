-- Preserve legacy sites without claiming their source artifacts exist.
ALTER TABLE sites ADD COLUMN initialization_status TEXT NOT NULL DEFAULT 'legacy'
    CHECK (initialization_status IN ('legacy','pending','ready'));

CREATE TABLE site_bootstraps (
    site_id UUID PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    version_id TEXT NOT NULL CHECK (version_id = 'initial'),
    archive BYTEA NOT NULL CHECK (octet_length(archive) > 0),
    manifest BYTEA NOT NULL CHECK (octet_length(manifest) > 0),
    execution_log BYTEA NOT NULL CHECK (octet_length(execution_log) > 0)
);
