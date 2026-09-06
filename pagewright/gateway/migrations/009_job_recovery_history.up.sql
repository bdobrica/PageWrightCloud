ALTER TABLE build_submissions
 ADD COLUMN recovery_checked_at TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01 UTC',
 ADD COLUMN recovery_error TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_build_recovery ON build_submissions(recovery_checked_at,job_id)
 WHERE dispatch_state IN ('ready','dispatching') OR (dispatch_state='accepted' AND status IN ('pending','running'));

-- Bounded to one event per lifecycle state, never an unbounded polling log.
CREATE TABLE job_history (
 job_id UUID NOT NULL REFERENCES build_submissions(job_id) ON DELETE CASCADE,
 status VARCHAR(20) NOT NULL CHECK (status IN ('pending','running','completed','failed')),
 error_code TEXT NOT NULL DEFAULT '',
 error_message TEXT NOT NULL DEFAULT '',
 result TEXT NOT NULL DEFAULT '',
 manifest_path TEXT NOT NULL DEFAULT '',
 observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(job_id,status)
);
INSERT INTO job_history(job_id,status,error_code,error_message,observed_at)
 SELECT job_id,status,error_code,error_message,updated_at FROM build_submissions;
