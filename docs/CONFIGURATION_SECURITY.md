# Required configuration and routine log safety (M4.2)

Root Compose no longer supplies working database or JWT secrets. A fresh checkout
must explicitly set independent `PAGEWRIGHT_POSTGRES_PASSWORD` (16+ bytes) and
`PAGEWRIGHT_JWT_SECRET` (32+ bytes). Generate each separately, for example using
`openssl rand -hex 32`, and place them in a protected, ignored `.env` or your
deployment secret injection mechanism. Do not commit or paste real values into
issues, test output, shell arguments or shared logs. Minimum length is not proof
of randomness. Known legacy JWT placeholders are rejected, including in development.
The legacy gateway-only Compose file also requires explicit secrets, but remains
an unsupported service-only topology rather than an alternative MVP deployment.

Compose rejects missing/empty secrets before starting services. Gateway startup
validates JWT, database, required storage/manager/serving endpoints, port and JWT
lifetime before connecting or migrating. Invalid values produce key/category-only
errors, not parser errors containing private values. Explicit invalid port/lifetime
strings cannot silently fall back; JWT lifetime must be positive and at most 24h.
AI stays disabled by default; enabling it still requires the separate provider
credential and allowance described in [pilot limits](PILOT_LIMITS.md). Disabled
OAuth does not require OAuth secrets. M4.3 adds required service/Redis credentials,
private ports and scoped worker access; see [internal authentication](INTERNAL_AUTH.md).

## One PostgreSQL credential source

The supported root stack uses its bundled PostgreSQL service, database/user
`pagewright`, and one `PAGEWRIGHT_POSTGRES_PASSWORD` value supplied to both
PostgreSQL and gateway. Gateway constructs the URL with `url.UserPassword`, so
reserved URL punctuation is password data. No shell-built connection string is
used. Database secrets are not passed to manager or worker.

**Root Compose no longer uses `PAGEWRIGHT_DATABASE_URL`.** This deliberately
prevents a stale URL from disagreeing with the bundled database password.
Deployments that intentionally use external PostgreSQL need a reviewed deployment
override, not a silent change to the root topology. Direct gateway/operator-CLI
processes can still set an explicit PostgreSQL URL with host, database, username,
16+ byte password and explicit `sslmode`. Only `sslmode` query options are accepted
(`disable`, `require`, `verify-ca`, `verify-full`); query-level credential/host
overrides are rejected. Advanced driver options need a reviewed extension.
Root single-host container traffic retains `sslmode=disable`; this is not TLS
protection for a remote database.

## Existing installations: coordinate rotation

Changing `POSTGRES_PASSWORD` does **not** change a role password in an initialized
PostgreSQL volume. Do not delete volumes or run a fresh initialization to rotate it.
Back up and verify recovery first; schedule downtime and stop gateway/dispatch
traffic. Through an authorized interactive database-admin session, use psql's
`\password pagewright` prompt to change the existing role password without putting
it in SQL arguments/history. Set the same new value in deployment configuration,
then recreate the coordinated services and verify login/readiness. Keep rollback
credentials protected until the rotation is confirmed. These steps require an
operator; no existing credentials, databases or remote hosts were changed here.

Set a new independent JWT secret in the same rollout. Rotation invalidates all
existing signed sessions; users must sign in again. Tab-local draft recovery
remains available, but is not a backup. Old images/configurations must not continue
accepting tokens signed by retired defaults. `.env` values and container environments
are accessible to the deployment administrator: never publish expanded
`docker compose config`, `docker inspect`, environment dumps or private traces.

## Routine logs versus private execution records

Reset tokens and reset-account addresses are no longer logged. Password-reset
email delivery and single-use consumption are implemented; see [reset operations](PASSWORD_RESET.md).
Logs are not a delivery channel. Gateway/operator failures withhold raw driver errors;
manager startup/reconciliation withhold private dependency diagnostics. Production
and legacy worker parse failures withhold job payloads; the legacy worker and
Kubernetes stub no longer print prompts or job environments/callback URLs.
The stub remains a stub, not a supported production execution path.

This does not erase historical logs or change private artifact manifests/execution
logs, which intentionally retain job content for diagnosis. Existing logs may
contain compromised credentials/tokens: rotate affected credentials, restrict
access, and handle retained logs under the operator's retention policy. Never
rely on string-based secret replacement to sanitize arbitrary model output.
Internal access restrictions are implemented. See [operations](OPERATIONS.md) for
bounded diagnostics and [release evidence](RELEASE_ACCEPTANCE.md) for public gates.

## Verification

- `node scripts/config-acceptance.mjs`: read-only Compose validation with an isolated
  environment and dummy secrets; missing-secret rejection and consistent credential
  wiring without printing expanded configuration or loading `.env`.
- Gateway race tests: actual entrypoint subprocess rejects invalid configuration
  before database I/O; unit tests cover placeholder/missing settings, malformed
  values, private parser input, URL punctuation and query override rejection.
- `make test-integration`: real reset-token creation is verified while capturing
  routine logs and checking neither token/account/hash leaks into logs or response.
- `make smoke-stack`: explicit synthetic secrets (including database punctuation),
  fresh initialization, login and crash/recreation persistence.
- [Browser acceptance](BROWSER_ACCEPTANCE.md): full isolated journey and operator
  provisioning against the new root credential wiring, with no paid provider calls.
