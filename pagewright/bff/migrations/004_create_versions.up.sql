-- Create versions table (keep recent versions for quick access)
CREATE TABLE IF NOT EXISTS versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    build_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_site_build UNIQUE (site_id, build_id)
);

CREATE INDEX idx_versions_site_id ON versions(site_id);
CREATE INDEX idx_versions_build_id ON versions(build_id);
CREATE INDEX idx_versions_created_at ON versions(created_at DESC);
