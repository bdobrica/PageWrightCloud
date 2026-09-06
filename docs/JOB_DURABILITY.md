# Job durability, restart recovery and retention (M2.9)

M2.9 adds gateway recovery and PostgreSQL lifecycle history, a Redis startup
durability gate, legacy-TTL protection, bounded metadata retention and a read-only
audit command. Worker execution/isolation is unchanged: the selected worker stays
`pagewright-worker:m2.8`. See [dispatch](QUEUE_DISPATCH.md),
[fenced commits](FENCED_COMMITS.md) and [result recovery](RESULT_RECOVERY.md).

## Persistence requirements

Production manager startup checks Redis configuration and AOF write health before
serving admission or dispatching jobs. Required settings:

| Setting | Required value |
| --- | --- |
| `appendonly` | `yes` |
| `appendfsync` | `always` |
| `no-appendfsync-on-rewrite` | `no` |
| `aof-load-truncated` | `no` |
| `maxmemory-policy` | `noeviction` |
| `aof_last_write_status` | `ok` |

Root and standalone-manager Compose set these explicitly and retain `/data` in
`redis_data`. External Redis must provide equivalent persistent storage and permit
the manager's read-only `CONFIG GET` and `INFO persistence` checks. The manager
does not mutate server configuration. There is no production environment switch
to bypass this gate; only compile-time integration spawners use ephemeral Redis.

The fsync policy favors acknowledgement durability over throughput. Rejecting
truncated AOF loading avoids silently accepting a shortened log; no-eviction avoids
discarding reservation keys under memory pressure. These are Redis configuration
guarantees, not protection against disk loss, rolled-back backups or unsupported
HA/failover. See [Redis persistence](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/)
and the [Redis 7.4 configuration reference](https://raw.githubusercontent.com/redis/redis/7.4/redis.conf).

Startup removes inherited TTLs from **surviving** job records, digest receipts,
site fence counters and dispatch indexes before opening admission. It never
persists a site lease, invents a missing key or reacquires an expired attempt.
The migration is idempotent, bounded to 100,000 scanned matching records and the
60-second startup context; oversized migrations fail startup for offline review.
Use dedicated Redis: these prefixes and site locks are global service state.

An expired legacy record is already missing evidence, not an empty slot safe to
reuse. Stop old writers and back up before migration. Existing writable-layer
Redis data is **not** copied into a newly attached volume automatically.

## Gateway recovery and durable history

Migration 009 adds a recovery timestamp/error to `build_submissions` and the
`job_history` table. Reservation, submission outcome and observed status changes
write lifecycle history in the same transaction as their version/submission
changes. Existing submissions are backfilled with their known current state;
legacy versions without a job mapping are not assigned guessed execution IDs.

Gateway startup launches a non-overlapping five-second recovery loop. A database
statement claims at most 32 due rows with `FOR UPDATE SKIP LOCKED`, ordered by
oldest observation, with a 30-second cooldown and submission-age grace period.
Listing is bounded to five seconds; each job observation to ten seconds. The
loop survives process restart via database state, not local ownership. Duplicate
observations across replicas are harmless; only the existing atomic dispatch
claim can authorize POST. Shutdown cancels recovery and waits for it before
closing the database.

| Persisted state / evidence | Recovery action |
| --- | --- |
| `ready`, never claimed | Claim atomically; winner sends the exact persisted request once |
| `dispatching`, including crash before send | GET the same job ID; never POST it again |
| `accepted` and pending/running | GET and verify manager identity/outcome |
| Exact owner/site/job/source/target/prompt match | Atomically update submission, version and lifecycle history |
| Running observation followed by pending | Reject the regression |
| Stored completed/failed outcome | Never reopen or revise it, even if materialization finishes late |
| Manager 404 | Keep uncertainty; record `manager_evidence_missing_operator_required` |
| Conflicting manager identity | Keep uncertainty; record `manager_evidence_conflict_operator_required` |
| Transport failure / malformed response | Keep uncertainty; retry a future bounded observation |

Manager responses are context-bound, redirect-free and limited to 1 MiB. Recovery
never regenerates a prompt, calls a provider, changes a target version, or uses
artifact existence alone as proof of a manager outcome. A failed history insert
rolls back both submission and version updates. The journal records observed
states, not every transient event that may have occurred during an outage.

Same-key submission responses can now reflect a recovered later status. Owner-
checked job/history endpoints, UI refresh and live updates remain M3; the journal
is not exposed as an unauthenticated endpoint.

## Retention policy

- Canonical Redis jobs, receipt reservations, fences, PostgreSQL submission/request
  mappings and version identity are retained without TTL. They are both history
  and duplicate-execution protection, not a disposable cache.
- `job_history` has at most one event per status per job (four possible states),
  enforced by its primary key. Polls and duplicate terminal observations do not
  append unbounded logs. Only a single current recovery diagnostic is stored.
- After 30 days, terminal Redis dispatch claim/token/state metadata can be removed
  by an incremental one-minute scan (COUNT hint 128, five-second operation limit).
  An atomic script compares the observed job and requires no active/site reservation.
  It never deletes the canonical job, receipt or fence. Retried admission still
  finds the original outcome after this cleanup.
- There is **no bounded total-storage guarantee** for permanent identities. Monitor
  Redis/PostgreSQL capacity and back them up; do not enable eviction or expire
  deduplication to save space. See [M2.10 container/temp/private-log retention](WORKER_RETENTION.md)
  for the current `pagewright-worker:m2.10` upgrade and cleanup policy.
  Future canonical-history archival needs a separate, coordinated tombstone policy.

## Operator recovery: missing or corrupt evidence

1. Pause public build admission and stop **all** gateways/managers that could
   dispatch or mutate recovery state. Keep the deployment private. Do not switch
   keys, reset submissions to `ready`, delete reservations, or lower fence counters.
2. Inventory running workers on the correct Docker daemon. A name alone is not
   ownership evidence; verify labels and full attempt identity as in M2.8. Stop
   only verified workers when necessary. Drain/stop storage writers before taking
   a consistent backup or restore; approved materializations can otherwise finish
   after observation. Never use broad container deletion or privileged workers.
3. Preserve PostgreSQL, the entire Redis data directory (including multipart AOF
   manifest/files) and storage as one recovery set. Keep the damaged/current set
   separately; restore into a new isolated volume/project and verify it before
   replacing anything. Do not run `down --volumes` on application state.
4. Run the read-only audit with writers stopped. The production manager image
   includes `/app/recovery-audit`; using the existing configured Compose project:

   ```sh
   docker compose run --rm --no-deps --entrypoint /app/recovery-audit manager
   ```

   It prints bounded JSON counts/issues without prompts, lease tokens or secrets.
   Exit 0 means no detected index/TTL/legacy-attempt issue; exit 2 means issues;
   exit 1 means the audit could not finish. It checks up to 100,000 job records,
   10,000 queued/site reservations and 128 active jobs. It detects missing indexed
   jobs and unindexed pending/running jobs, but is **not** proof that a missing
   record never executed and does not independently verify filesystem receipts.
5. Review PostgreSQL evidence separately, through an authorized database session:

   ```sql
   SELECT job_id, site_id, target_version, dispatch_state, status, recovery_error
   FROM build_submissions
   WHERE recovery_error <> '' OR dispatch_state = 'dispatching'
   ORDER BY recovery_checked_at, job_id
   LIMIT 100;
   ```

6. Prefer restoring the exact verified evidence. Normal pre-intent queue claims
   resume through M2.2; current fenced running/uncertain attempts resolve through
   M2.8 without relaunch. Gateway recovery then persists verified terminal history.
   If a backup predates possible dispatch, even a restored `ready` row is not
   proof of non-execution: quarantine it as uncertain under an explicitly reviewed
   database repair before enabling the gateway. Never blindly replay restored work.
7. If evidence cannot be recovered, keep affected submissions/slots quarantined
   and admission paused for affected work. Record the incident and seek an
   explicitly reviewed recovery/migration decision. A missing receipt, Docker 404,
   old deadline or empty Redis database does not authorize synthesizing a failed
   outcome, reopening a job, releasing unrelated locks or redispatching it.

This conservative operator path deliberately trades availability for preventing
duplicate execution. Arbitrary datastore loss/rollback cannot be repaired by
assuming absence means non-execution. No destructive repair or live migration is
performed by the audit or this milestone.

## Acceptance

`make test-integration` covers real PostgreSQL journal atomicity, concurrent batch
claims, gateway handler recreation, claimed-before-send uncertainty, late verified
completion, terminal non-regression, legacy TTL protection, retention deduplication
and read-only audits, alongside existing Redis attempt-recovery tests.

`make smoke-stack` uses a generated disposable project and missing worker image
(no AI). It verifies root startup, bootstrap persistence, kills only that project's
Redis/gateway/manager with SIGKILL, recreates containers with retained volumes,
then checks job deduplication, durable terminal history and a claimed-but-never-
received submission that remains uncertain. It also runs the packaged audit.
These checks do not establish disk-loss, backup-restore or multi-host HA safety.
