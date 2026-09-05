-- Legacy versions remain untouched: their execution IDs cannot be inferred.
CREATE TABLE build_submissions (
    job_id UUID PRIMARY KEY CHECK (job_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_version VARCHAR(255) NOT NULL CHECK (btrim(source_version) <> ''),
    target_version VARCHAR(255) NOT NULL CHECK (btrim(target_version) <> ''),
    prompt TEXT NOT NULL CHECK (btrim(prompt) <> ''),
    request_key UUID NOT NULL,
    request_hash VARCHAR(64) NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    dispatch_state VARCHAR(20) NOT NULL DEFAULT 'ready' CHECK (dispatch_state IN ('ready','dispatching','accepted','rejected')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed')),
    error_message TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    response_status INTEGER NOT NULL DEFAULT 0 CHECK (response_status = 0 OR response_status BETWEEN 100 AND 599),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, site_id, request_key),
    UNIQUE (site_id, target_version),
    FOREIGN KEY (site_id, target_version) REFERENCES versions(site_id, build_id),
    CHECK (target_version <> job_id::text),
    CHECK ((status = 'failed' AND btrim(error_message) <> '') OR (status <> 'failed' AND error_message = '')),
    CHECK (dispatch_state <> 'rejected' OR status = 'failed')
);

CREATE INDEX idx_build_submissions_site_created ON build_submissions(site_id, created_at DESC);
