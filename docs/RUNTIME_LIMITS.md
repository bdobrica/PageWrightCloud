# Runtime limits and readiness

M4.10 adds request-scoped cancellation without changing worker isolation or the
durable build/deployment protocols. These defaults apply after rebuilding and
recreating the affected services; changing this repository does not upgrade the
remote pilot.

## HTTP budgets

| Listener | Handler context budget | Read headers / request | Write / idle |
| --- | --- | --- | --- |
| Gateway public API | 55s | 5s / 15s | 65s / 60s |
| Manager | 15s | 5s / 30s | 30s / 60s |
| Storage | 45s | 5s / 30s | 60s / 60s |
| Serving API | 45s | 5s / 30s | 60s / 60s |

The UI defaults to 30s, retains 10s read/poll limits, and allows 60s for build
submission. Gateway clarification and instruction calls each have a 25s HTTP
limit inside the shared 55s request budget. Internal gateway clients and serving
artifact downloads have 30s total HTTP limits. Their transports cap dialing and
TLS negotiation at 5s, response headers at 30s, idle connections at 60s, and
connections per host at 16 (8 idle; 32 idle overall). Redirects remain disabled
and internal credentials remain restricted to their configured origin.

The separate worker-provider listener keeps its existing 120s upstream limit
and 150s write timeout; worker execution, delivery and recovery budgets are not
replaced by a browser request deadline. There are no added provider retries or
paid readiness calls. Cancelling local I/O does not establish that a remote
provider stopped processing or that a reservation can be refunded.

Gateway foreground database and downstream HTTP operations inherit the incoming
request cancellation. Request-local database/client views do not mutate shared
objects. Stream cancellation remains attached until the body is closed, while
the HTTP client's own timeout is preserved. A cancelled serving download removes
its temporary file without replacing the destination.

A timeout is **not a rollback receipt**. A manager request may have been accepted
before its response was lost. Continue to reconcile the same persisted job ID;
do not generate a fresh idempotency key merely because a request timed out.
Existing bounded detached outcome persistence is retained. Serving preserves
pending/activating intent when cancellation interrupts preparation, so durable
recovery can retry the same deployment identity. Local filesystem operations and
already-started atomic activation are not forcibly interrupted by HTTP contexts.

## Connection pools

Each gateway process opens at most 10 PostgreSQL connections, keeps at most 5
idle, recycles connections after 30 minutes, and retires idle ones after 5 minutes.
Startup ping has a 5s deadline and closes the pool on failure. Pool waits and SQL
from HTTP handlers are cancellable; operator CLI calls retain their own lifecycle.
Account for all replicas and operator tools when sizing the database server.

Each manager Redis client has a 16-connection pool, 3s pool/read/write limits,
5s dialing, context deadline enforcement, and no automatic command retries.
Queue and lock clients remain separate. Durable application-level reconciliation
is unchanged; an uncertain Redis write is not assumed to have rolled back.

## Liveness and readiness

Exact anonymous `GET /health` remains liveness. Exact `GET /ready` returns
`200 ready` or `503 not ready`, with `Cache-Control: no-store` and no dependency
names, credentials or raw errors. The complete readiness check has a 2s budget.
Internal requests with query strings or other methods still require authorization.

| Service | Readiness dependencies |
| --- | --- |
| Gateway | PostgreSQL ping; manager, storage and serving readiness |
| Manager | Redis ping; Docker daemon ping for the Docker spawner |
| Storage | Configured artifact volume exists and can be opened/listed |
| Serving | Artifact/config directories; storage readiness; no pending Nginx recovery journal and a local Nginx HTTP response |

This graph has no cycles. Storage intentionally does not depend on manager
readiness: its read path remains usable during manager recovery, while fenced
writes continue to require manager authorization. Readiness does not certify disk
capacity/writability, image availability, successful worker sandbox startup,
provider billing/availability, external DNS/TLS or end-to-end publication.

Compose health checks use readiness. Docker marks unhealthy containers but does
not automatically restart them just for being unhealthy; inspect the dependency
and existing service logs before taking recovery action. Liveness being healthy
is not sufficient to admit new work.

## Verification

Run `make test-all`, `make test-integration`, and
`node scripts/check-service-auth.mjs`. Integration uses a uniquely named,
disposable Compose project, no application `.env`, no published ports and no paid
provider. In addition to PostgreSQL pool-wait/query cancellation, it checks live
readiness, stops Redis then storage, verifies dependent readiness becomes 503
while liveness remains 200, and requires readiness to recover after restart.
Its cleanup removes only that test project's containers and volumes.

Focused gateway race tests cover both provider calls, bootstrap writes, version
listing, manager submission, serving toggles, streaming cancellation and retained
client deadlines. Serving tests verify interrupted downloads preserve existing
bytes. The shared runtime/auth conformance checks guard independent Go modules
against drift. These checks do not close the remaining public M4.9 release gates.
