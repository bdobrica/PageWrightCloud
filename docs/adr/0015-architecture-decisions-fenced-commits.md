# Site leases and fenced commits (M2.7)

ADR 0015 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


The manager owns site leases and commit authority. A worker attempt is the tuple
`job_id`, `site_id`, `owner_id`, source/target versions, `lock_token` and
`fencing_token`. The random lock token is the unique attempt identity; the fence
is the monotonically increasing per-site generation. These internal fields are
not added to the gateway's public response DTO.

## Lease lifecycle

Acquisition allocates the fence and expiring lock in one Redis script. A paused
acquirer cannot increment the fence after a separately acquired lease expires.
Fence allocation fails closed before exceeding Lua/JSON's exact integer range.
Dispatch intent checks the actual lock and fence atomically and records a
Redis-clock dispatch start timestamp.

Each manager replica renews active jobs from authoritative Redis state, including
launching/uncertain jobs, without depending on process-local goroutines surviving
a restart. Renewal verifies the running attempt, active/site reservation, lock
token and current fence. It never reacquires an expired lock. Missing, expired,
superseded or terminal attempts cannot commit. Their unreconciled job slots remain
reserved; lease loss does not implicitly start a replacement worker.

Defaults: lock TTL 5 minutes, renewal every minute, maximum renewal lifetime
30 minutes from dispatch intent (`PAGEWRIGHT_WORKER_TIMEOUT`). Startup requires
TTL >= 1 second, a positive renewal interval <= TTL/3 and lifetime >= TTL.
Renewals are clipped at that lifetime. The worker's M2.6 execution/watchdog limits
remain independent and shorter. All replicas must use the same settings and Redis
database. Redis Cluster/failover is not a supported deployment topology.

## Storage publication

Normal workers send `X-Pagewright-Attempt` (JSON of the attempt tuple) on artifact,
private-log and manifest writes. Storage uses its configured
`PAGEWRIGHT_MANAGER_URL` (default `http://manager:8081`) as the only commit
authority; callers cannot supply a callback host. Non-bootstrap writes without
matching identity are rejected. `initial` is reserved for the existing bootstrap
path and cannot be a newly submitted worker target. Existing immutable versions
remain readable without consulting the manager.

The write sequence is:

`stage bytes + fsync → atomic Redis digest reservation → no-replace filesystem link`

After reading and syncing the entire request, storage computes SHA-256 and byte
count and calls `POST /jobs/{job_id}/write-commit`. The manager checks the current
lease, fence, attempt and site reservation in the same Redis operation that
reserves those exact bytes for that version/part. Reservations bind a target
version to one attempt and do not expire. Different bytes or another attempt
cannot replace them. Manifest approval additionally requires artifact/log
reservations, while storage requires both files to exist and syncs their
directories. The manifest's fencing token must match the attempt header.

The Redis reservation is the **logical write commit point**, not an authorization
check to be repeated later. The following hard link materializes that already
approved immutable write. A lease expiring before reservation rejects the write;
expiry after reservation does not revoke the exact bytes already committed.
This distinction avoids claiming an atomic transaction spans Redis and the
filesystem. No filesystem overwrite or mutable "latest" pointer is permitted.
Redis provides [atomic script execution](https://redis.io/docs/latest/develop/programmability/eval-intro/).

If storage crashes or the acknowledgement is lost after reservation, bytes may
be staged/reserved but not visible. An identical object retry is allowed only
while that attempt remains active and leased. M2.8 adds conservative
[receipt and result reconciliation](0016-architecture-decisions-result-recovery.md); do not delete a receipt
to retry different bytes. [Restart durability](../JOB_DURABILITY.md) is covered by M2.9 and receipt
receipts remain nonexpiring under the [M2.10 retention policy](../WORKER_RETENTION.md).

## Results and retries

Both callback routes require the complete attempt identity. The queue checks it
again atomically, including the live lease and current fence, before modifying
outcome fields. Completion requires a manifest reservation for that attempt and
the canonical manifest path. Terminal outcome, capacity/site release and
token-checked lock removal occur in one script.

Expired, superseded, duplicate terminal, and terminal-to-running callbacks return
409 without modifying the snapshot or releasing another attempt's lock. This
includes identical terminal retries. Admission retries (`POST /jobs`) remain
idempotent and return the stored snapshot. A worker receiving an ambiguous result
response now reconciles via bounded job lookup (M2.8), not assume 409 means
its previous result was never accepted.

## Upgrade and scope

Selected image/build defaults are `pagewright-worker:m2.12`.
Drain/reconcile pending, running and uncertain jobs before upgrading manager,
storage and worker together. Old workers do not send the new identity header;
old active jobs lack the dispatch timestamp/commit receipts needed for this
contract. Do not hot-mix worker generations or reset fence counters/receipts.
Back up Redis and storage together; Redis persistence and uncertain-job recovery
follow the [M2.9 durability gate and operator policy](../JOB_DURABILITY.md). No live
application-data restore is performed; surviving legacy reservation TTLs are protected.

Fencing is not authentication: the internal API is still trusted-network-only.
An actor with internal API/Redis access can obtain or forge identities. Service
authentication, per-job credentials and external exposure remain M4. The AI tool
process remains isolated from runner credentials by M2.6. The bootstrap exception
is not a general unfenced worker-write mode.

Acceptance includes actual Redis concurrent jobs outliving two original TTLs,
expired-lease non-resurrection, monotonic replacement fences, changed-byte and
attempt rejection, missing-manifest completion rejection, terminal duplicate
rejection, and a streamed upload expiring before publication. Service fixtures
use real manager attempts; the compiled worker round-trip exercises the complete
artifact/log/manifest/callback path without a paid provider.
