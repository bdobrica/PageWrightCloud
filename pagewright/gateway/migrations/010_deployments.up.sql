CREATE SEQUENCE deployment_sequence;
CREATE TABLE deployments (
 site_id UUID PRIMARY KEY REFERENCES sites(id) ON DELETE RESTRICT,
 sequence BIGINT NOT NULL UNIQUE DEFAULT nextval('deployment_sequence'),
 fqdn TEXT NOT NULL,
 version TEXT NOT NULL,
 target TEXT NOT NULL CHECK (target IN ('live','preview')),
 status TEXT NOT NULL CHECK (status IN ('pending','completed','failed')),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX deployments_pending ON deployments(updated_at) WHERE status='pending';
