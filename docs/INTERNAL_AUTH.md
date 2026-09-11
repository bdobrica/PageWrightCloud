# Internal access and worker attempt credentials (M4.3)

Root Compose publishes only gateway, UI and the public hosting edge. PostgreSQL,
Redis, manager, storage, serving API, theme registry and worker status ports are
not published. Changing old internal `*_PORT` variables does not reopen them.
`docker-compose.smoke.yaml` is a disposable acceptance overlay that temporarily
publishes random loopback ports; never use it with application data or a pilot.

## Credentials and trust boundary

Set an independent random `PAGEWRIGHT_SERVICE_TOKEN` of at least 32 bytes for the
gateway, manager, storage and serving services. Missing/short keys fail startup.
Never reuse the JWT secret or worker-known provider credential. These four
services form one trusted control plane: this MVP uses a shared service credential,
not per-service ACLs or mTLS. It is not supplied to PostgreSQL, Redis, UI, workers
or generated sites. Clients pin credential attachment to the configured destination
origin, and existing redirect refusal remains in place.

Every internal manager/storage/serving API read and write requires a service or
permitted worker Bearer credential. Only exact `GET /health` is unauthenticated.
The public gateway continues using user JWTs and ownership checks. A user JWT,
cookie, query token or pilot-provider token is not internal service authority.
Worker status HTTP binds to `127.0.0.1` inside the worker container; orchestration
uses Docker lifecycle inspection and the existing callback/reconciliation flow.

Root Redis now requires `PAGEWRIGHT_REDIS_PASSWORD`, independently generated and
at least 16 bytes. It is supplied only to Redis and manager, not workers. Redis
health checks use `REDISCLI_AUTH`; anonymous container-network commands return
NOAUTH. PostgreSQL retains its independently configured role password from M4.2.
The separate isolated integration fixture uses disposable Redis without auth for
queue tests; root startup/browser acceptance exercises production Redis auth.

## Worker scope and existing fencing

At Docker launch, manager signs a `worker-v1` capability using HMAC-SHA256. It binds
the job, site, source version, target version, lock token, fencing number and expiry.
The worker receives only that signed capability in `PAGEWRIGHT_WORKER_TOKEN`,
alongside its existing private launch snapshot and limited provider credential.
The master signing/service key is never passed to the worker. The CLI environment
allowlist and sandbox/privilege/resource policies are unchanged.

Capabilities expire 17 minutes after issuance, covering the runner's existing
16-minute whole-container ceiling with a one-minute delivery margin. Keep host
clocks synchronized. No token-refresh or automatic relaunch path is introduced.

| Destination | Worker capability permits |
| --- | --- |
| Manager | Read its exact job; POST its exact job's status/result |
| Storage | GET its source archive; PUT its target archive; POST target logs/manifest |
| Serving | Nothing |
| Redis/PostgreSQL | Nothing; separate credentials required |

Worker bootstrap writes, version enumeration, private metadata reads, other
job/version operations and manager write-commit authority are denied. Cross-job
URLs fail before callback decoding or queue writes. Verified signed attempt fields
must also match callback bodies and storage attempt headers. Spoofed verification
headers are cleared at the authentication boundary. Job readback checks the saved
attempt, preventing a stale capability from learning a replacement lock token.

Authorization does not replace fencing: manager still checks full stored identity,
lease and terminal state; storage still requests authenticated write-commit approval
and uses its atomic publication guard. A previously signed capability can read its
source until expiry, but a stale/terminal attempt cannot bypass write fencing.
Callback readback/retry remains job-scoped after terminal completion until expiry.

The independently built Go modules carry identical small stdlib-only serviceauth
packages and conformance tests. `node scripts/check-service-auth.mjs` and the
integration runner reject drift; update all copies together. This avoids changing
module/build-context topology during the security gate.

## Upgrade and recovery

Back up first and drain dispatch/active workers. Configure independent service and
Redis credentials, rebuild the coordinated services **and worker image**, then
restart the stack and verify readiness/login. Old worker images cannot send scoped
credentials and must not remain configured as the root manager's worker image.
The default tag is now `pagewright-worker:m4.5`; update any older explicit image
pin in deployment configuration. Build it with `docker compose build worker`
before starting dispatch. Do not reuse an old local image under the new tag.

Preserve PostgreSQL/Redis/artifact volumes. Redis loads its existing AOF with the
new startup authentication setting; this does not reset queue state. PostgreSQL
password rotation still follows [M4.2 guidance](CONFIGURATION_SECURITY.md).
Service-key rotation revokes old worker capabilities and requires all four services
to restart together. Do not rotate during active attempts without accepting failed
callbacks and operator reconciliation. Missing/expired credentials fail closed,
not through an unauthenticated fallback. Never delete durable job reservations to
recover a credential failure.

These changes protect the supported root topology; legacy service-only deployment
files are not a supported pilot configuration. Do not add public API port mappings
to work around authentication errors. Use authorized container exec/admin access
without printing real environment values, signed capabilities or private job data.
Docker administrators/control-plane processes remain trusted; HTTP within the host
network is not encrypted or protected against host compromise. Local origin,
identifier/archive and cross-user acceptance passed; public failure-recovery/
full-browser gates remain. See [release evidence](RELEASE_ACCEPTANCE.md).

## Verification

- `node scripts/config-acceptance.mjs`: required secrets, unpublished internal ports
  and consistent configuration; does not load `.env` or print expanded secrets.
- Shared conformance/race tests: absent/forged/expired/rotated credentials, worker
  route/method restrictions and destination pinning. Handler tests check cross-job
  and stale-attempt rejection without backend/queue mutation.
- `make test-integration`: authenticated production internal services, explicit
  test-only client credentials and existing lifecycle/fencing/transport recovery.
- `make smoke-stack`: authenticated root startup, Redis/gateway/manager crash and
  recreation with durable history and unchanged application volumes.
- [Browser acceptance](BROWSER_ACCEPTANCE.md): actual sandboxed workers complete two
  edits and the expected failed build through scoped credentials. A fixture
  container without a master token then probes anonymous API/Redis access and
  cross-job/cross-version misuse using the test workers' real signed capabilities.
  Operator provisioning/login and disabled-AI behavior remain covered. All provider
  responses are local dummy fixtures; no paid calls or remote deployment occur.
