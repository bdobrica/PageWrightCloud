# Durable bounded dispatch (M2.2)

`POST /jobs` atomically stores the canonical job, reserves its site and appends
its identity to Redis. A first successful response is **201 pending**; it does
not acquire a lock or call Docker. Poll `GET /jobs/{job_id}` for execution and
failure. Same-identity retries return the stored snapshot (including remembered
rejections), never enqueue another execution. A different job for an already
pending/running site is rejected with `job_busy`.

## Dispatch boundary

1. Atomically claim a pending identity with a random owner token and Redis-clock
   lease. Claims count against the shared active-job limit across all replicas.
2. Acquire the existing site lock and atomically record running status, lock,
   fence and irreversible launch intent, checking claim ownership and expiry.
3. Call Docker once. Merge container metadata without overwriting a fast terminal
   callback. A definite no-start failure records `failed/spawn_failed` and frees
   capacity. Success or an ambiguous outcome retains capacity until terminal.

Expired **pre-intent** claims return to the queue; stale claim owners cannot
launch. Restarting a manager preserves this recovery and the shared limit.
**Intent is never automatically replayed**, including a crash immediately before
Docker is called, a lost intent acknowledgement or an ambiguous launch. These
jobs conservatively remain running and occupy capacity; automatic/operator
reconciliation is M2.8/M2.9. This is duplicate-safe dispatch, not guaranteed
eventual completion or exactly-once execution under arbitrary storage loss.
Do not delete dispatch markers or expire reservations to force retries.

Terminal callbacks atomically free active capacity and the site's admission
guard. Outcome updates preserve dispatch metadata and cannot reopen terminal
jobs. Pending jobs cannot report execution outcomes. M2.7 adds
[lease renewal and fenced storage/result commits](FENCED_COMMITS.md), including
atomic lock removal and rejection of terminal callback retries. Authentication
remains M4; internal APIs must still
be restricted to trusted local operation.

## Configuration and shutdown

| Variable | Default | Meaning |
| --- | --- | --- |
| `PAGEWRIGHT_DISPATCH_CONCURRENCY` | `4` | Shared active-job cap, 1–128 |
| `PAGEWRIGHT_DISPATCH_CLAIM_TTL` | `30s` | Pre-intent lease, 1s–1m |

All replicas sharing the queue must use the same cap. Drain active jobs and stop
all replicas before changing it; mismatched active configurations fail closed.
The cap counts complete job lifetimes, not just concurrent Docker API calls.
It does not bound backlog size or individual worker CPU/memory.

Shutdown stops new claims, waits up to 45 seconds for bounded dispatch operations
while keeping callbacks available, then allows 15 seconds for HTTP shutdown.
Compose grants 65 seconds. This does not cancel or drain long-running workers
or recover lost callbacks. M2.7 renews active leases while a manager is running;
shutdown stops this replica's renewer, while another replica can continue it.

## Persistence and upgrades

Root and standalone-manager Compose now mount `redis_data` at `/data` and enable
AOF with `appendfsync always`. This minimum prerequisite was brought forward
from M2.9. Redis documents that this policy fsyncs writes before acknowledging
them, with a throughput/latency cost: [Redis persistence](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/)
and [fsync latency](https://redis.io/docs/latest/operate/oss_and_stack/management/optimization/latency/).
External Redis must provide equivalent durable storage/acknowledgement settings;
the manager does not configure an external server. Disk loss, replication/HA,
backups and restore procedures are not solved by a local volume. The isolated
protocol-test Redis remains intentionally ephemeral.

**Before upgrading an existing stack**, stop old managers and preserve/export
existing Redis data. Earlier Compose stored it in the container writable layer;
attaching a new volume does not migrate those records. Arrange and verify the
Redis backup/restore into the new persistent storage before replacing that
container. This change does not migrate application data automatically, and
`docker compose down --volumes` still destroys named-volume data.

On first startup the dispatcher imports at most 10,000 legacy list entries:
pending jobs remain queued; running jobs reserve capacity/site admission and are
never relaunched; terminal history remains stored. Missing/malformed records,
unknown states or multiple active legacy jobs for one site stop initialization
for reconciliation. Stop **all** old request-spawning managers before upgrading;
mixed old/new dispatch implementations are unsupported. Existing legacy record
TTLs are not rewritten; history retention and broader restart reconciliation,
including gateway claims not received by the manager, remain M2.9.

## Verification

- `make test-integration`: race-enabled real-Redis claim expiry/token fencing,
  intent-boundary recovery, independent dispatcher clients and restarts, global
  capacity, legacy import, malformed metadata, terminal merges and fast callbacks;
  existing gateway/worker contracts now wait for asynchronous dispatch.
- `make test-docker-spawner`: isolated real daemon launch regression, no AI.
- `make smoke-stack`: root stack acknowledges a job, records missing-image failure
  without launching a worker, recreates containers retaining volumes, then checks
  the exact identity/outcome and retry deduplication survive. Test resources are
  isolated and removed, not application state.
