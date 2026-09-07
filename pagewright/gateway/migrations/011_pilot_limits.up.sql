-- Durable admission and spending reservations. No automatic budget refunds.
CREATE TABLE pilot_attempts (
 owner_id text NOT NULL, site_id text NOT NULL, request_key text NOT NULL,
 request_hash text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 busy boolean NOT NULL DEFAULT true,
 PRIMARY KEY(owner_id,site_id,request_key)
);
CREATE INDEX pilot_attempt_owner_date ON pilot_attempts(owner_id,created_at);
CREATE INDEX pilot_attempt_site_date ON pilot_attempts(site_id,created_at);
CREATE TABLE pilot_rates (
 key text PRIMARY KEY, window_start timestamptz NOT NULL, count integer NOT NULL
);
CREATE TABLE pilot_provider_reservations (
 id uuid PRIMARY KEY, reserved_cents integer NOT NULL CHECK(reserved_cents > 0),
 active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now()
);
