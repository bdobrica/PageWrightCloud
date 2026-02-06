-- Create site_aliases table
CREATE TABLE IF NOT EXISTS site_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    alias VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_alias UNIQUE (alias)
);

CREATE INDEX idx_site_aliases_site_id ON site_aliases(site_id);
CREATE INDEX idx_site_aliases_alias ON site_aliases(alias);
