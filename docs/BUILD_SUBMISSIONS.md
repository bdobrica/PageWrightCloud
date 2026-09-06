# Durable build submissions — M1.2

The gateway commits a job-to-artifact mapping before calling the manager. The
pending version uses `target_version`, never the execution's `job_id`.
See [JOB_CONTRACT.md](JOB_CONTRACT.md) for wire shapes.
M2.5 selects [the latest completed draft, then live, then bootstrap](BUILD_SOURCE.md)
for each new submission and exposes the persisted base in chat.
M2.9 adds [durable lifecycle history and background recovery](JOB_DURABILITY.md).

## Retry identity

`POST /sites/{fqdn}/build` requires an `Idempotency-Key` header containing a nonzero
UUID. Its scope is authenticated owner + site. Reusing a committed key with a
different message/clarification returns 409. The committed prompt, source, job ID
and target version are reused, even after gateway restart.

The UI retains the key after network errors or uncertain responses, blocks
simultaneous double sends, and rotates it for changed input. Success or an explicit
rejection clears it so a later intentional send can create a new attempt. This
state is page-local: navigation/refresh recovery is M3. API callers must retain
their keys themselves. CORS permits the header.

Provider instruction generation precedes reservation. Concurrent first requests
can both call the provider, but only one prompt/mapping is committed and dispatched.
Provider failures create no job/version. Clarification questions are not queued
jobs; their pre-submission conversation cache remains in-memory. Once a clarified
submission is committed, retries use its database record rather than that cache.

## Persistence and dispatch

Migration 007 adds `build_submissions`: owner/site, request key/hash, independent
execution/artifact IDs, source, prompt, dispatch state, status snapshot and saved
submission response/error. Reservation checks site ownership under a transaction
lock and inserts the pending version and mapping atomically. Failed writes roll
both back; unique constraints and a transaction-scoped request lock serialize
duplicates. Legacy versions are preserved without guessing their execution IDs.

| Dispatch state | Meaning | Retry action |
| --- | --- | --- |
| `ready` | Mapping/version committed; dispatch unclaimed | Atomically claim; only winner sends POST |
| `dispatching` | Manager may have received the request | GET the same ID; no second POST |
| `accepted` | Manager outcome and version status saved | Return saved response; M2.9 recovers subsequent lifecycle status |
| `rejected` | Definite failure and failed version saved | Return saved error |

The gateway sends the exact committed identities and prompt. Outcome writes
update submission and version together, use a bounded context independent of
browser disconnect, and reject conflicting late decisions. A failed outcome
write leaves the mapping available for reconciliation.

M2.9 periodically reconciles exact manager outcomes into PostgreSQL submission,
version and lifecycle history in one transaction. Replays can still lag while
the manager is unavailable. M3.1 adds owner-checked history retrieval and UI wiring;
worker callbacks do not write directly to PostgreSQL.

## Failure and uncertainty

Saved rejection responses contain `error`, `message`, `submission_state: "rejected"`,
`job_id` and `target_version`. Contention/identity conflicts use 409, spawn/request
rejection uses 502, and failure during connection dial uses 503 (no request was
delivered). Same-key retries repeat the rejection; a new intentional attempt uses
a new key.

Timeouts, lost responses, malformed manager responses and failed outcome writes
are not proof that execution failed. GET reconciliation checks the exact
job/owner/site/source/target/prompt and recognizes saved manager rejection records.
If no verified outcome can be saved, the gateway returns HTTP 503 with
`error: "submission_uncertain"`, `submission_state: "dispatching"`, a diagnostic,
and the persisted job/target IDs. Retry with the same key.

A crash between claiming and sending, or loss of manager evidence, can leave a
submission uncertain even when GET returns 404. The gateway deliberately does
not automatically redispatch or invent a new version. M2.9 records a durable
operator-required diagnostic and supplies a [recovery runbook](JOB_DURABILITY.md).
Do not delete a
submission or switch keys to bypass uncertainty without checking whether work ran.

## Manager boundary

An explicit manager `job_id` must be a canonical nonzero UUID with a distinct,
nonblank `target_version`. A Redis script reserves the snapshot and appends its
queue ID atomically before lock/spawn. Matching repeats return the existing job;
changed associations/prompt/source/target return 409. Remembered `job_busy` and
`spawn_failed` errors retain their 409/502 statuses. No existing reservation is
spawned again by the request handler, including pending ones after a crash.

New reservation keys have no TTL so deduplication cannot simply age out during
execution. M2.9 startup removes surviving legacy reservation TTLs; already-missing
records are not recreated. An
atomic worker-ID merge preserves a terminal callback that finishes during Spawn.

Nonexpiring keys alone are not restart durability. Root/standalone Redis use
persistent AOF storage and M2.9 checks the production durability settings.
Canonical identities/receipts remain permanent; only old terminal dispatch
bookkeeping is trimmed. PostgreSQL history survives independently, and missing
manager evidence is never automatically redispatched. Callback fencing is M2.7;
internal API authentication remains M4.

The real Docker spawner distinguishes definite rejection from ambiguous
create/start outcomes; M2.8 handles result/exit/timeout reconciliation without
relaunching intent. Manager POST redirects are not
followed, so a later redirect dial failure cannot masquerade as an undelivered request.

## Verification and upgrade

Database tests cover rollback, ownership, duplicate/mismatched keys, concurrent
reservations/claims, atomic outcomes, reconnect and 6→7 upgrade. Gateway tests
inspect committed rows while manager POST is received, then exercise handler
recreation, duplicate requests, rejection, lost responses, failed outcome writes,
dial failures and missing manager evidence. Manager/Redis tests cover actual atomic
reservation, concurrent HTTP repeats, error stages, TTLs and nonresurrection. UI
tests cover key reuse/rotation, double-send prevention and failed result parsing.

Run `make test-all`, `make test-integration` and UI contract tests/lint/build from
[TESTING.md](TESTING.md). Providers/spawning are faked; these checks do not invoke
paid AI or prove publishing works.

Back up before upgrading and rebuild gateway/manager/UI together. Submission
records reference version rows: generic version deletion/retention cannot silently
remove mappings. M2.9 retains these identities; destructive retention remains
disabled. Migration does not delete old
versions or rewrite live/preview pointers.
