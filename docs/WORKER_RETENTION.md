# Worker and staging retention (M2.10)

Selected worker: `pagewright-worker:m2.12`. Isolation, the pinned CLI/compiler,
capabilities and resource ceilings are unchanged. No privileged containers are
required. This policy complements [durable job recovery](JOB_DURABILITY.md) and
[immutable result receipts](RESULT_RECOVERY.md); it does not garbage-collect versions.

## Containers and operational diagnostics

The manager scans canonical jobs at startup and once per minute (Redis SCAN count
hint 128, 20-second operation context). Only completed/failed jobs at least one
hour past their terminal update, with a surviving fenced launch identity, qualify.
Docker inspection must verify the deterministic name, role/job/site/network
labels, actual network and complete launch tuple against that job. Missing,
corrupt, legacy or conflicting evidence is quarantined, never inferred from names
or labels alone. Unknown orphan containers require the read-only audit and
operator procedure in the durability runbook; they are not automatically deleted.

Before any destructive action, Redis atomically rechecks the unchanged terminal
job and absence of its active/site reservation, then saves an allowlisted JSON
diagnostic of at most 4 KiB. Fields are job, site, target version, container ID,
terminal status, exit code, OOM/running flags and observation time. Prompts,
environment, credentials, lease tokens and raw Docker output are not collected.
The key is `<queue-key>:diagnostics:<job-id>`, with a seven-day TTL from its first
write. Retries preserve that TTL and require the same container ID. If it expires
while cleanup is still pending, a later observation can create a new record.
Diagnostic storage failure prevents cleanup.

A lingering running terminal worker is killed by its verified immutable ID; only
a fresh later inspection permits removal. Exited/dead and never-started created
containers are removed by immutable ID without force or volume deletion. Missing
containers are harmless. Canonical jobs, fences, receipts and reservation state
are never deleted or rewritten by this loop. Existing Docker log rotation remains
two 10 MiB files per worker; those local logs disappear with its container. Worker
scratch data uses bounded tmpfs and does not require host-directory deletion.

New immutable private worker logs contain only job/site/version identifiers,
executor-output byte count and an explicit redaction notice. Arbitrary executor
text is wholly withheld, not filtered by a secret-detection regex. The diagnostic
is at most 4 KiB before the existing JSON content envelope; oversized identities
produce a bounded notice. Runner failure callbacks and completion/error console
messages also withhold raw execution diagnostics. This trades detailed debugging
output for conservative redaction.

Immutable private logs remain with their version, without an independent TTL:
completion verification needs their digest receipts and bytes. Historical logs
are not retroactively scrubbed or rewritten. Source/artifacts, manifest summaries,
prompts and existing API responses are not operational-log redaction targets.
This is not a claim that all stored user content or every system log is sanitized;
internal APIs still require a trusted network until M4 authentication lands.

## Abandoned storage uploads

Storage checks at startup and hourly for regular `.upload-*` files at least seven
days old. It never follows symlinks, removes directories, or deletes final
archives/manifests/logs or receipts. Each pass permits at most 128 deletions and
100,000 visited entries, with a five-second context. Each deletion syncs its
parent directory. An oversized tree defers further cleanup and requires offline
maintenance; filesystem calls themselves are not forcibly interrupted.

All immutable writers hold a shared `flock` on the stable root `.uploads.lock`
from before staging creation through publication and cleanup. The cleaner takes
an exclusive, nonblocking lock; any active writer postpones cleanup. This also
coordinates multiple storage processes sharing the supported Linux local
filesystem/Docker named volume. Network-filesystem locking semantics are not
validated. Never unlink or replace `.uploads.lock` while services are operating.

The seven-day cutoff may discard reserved but never materialized staging bytes.
It does not erase the reservation, reopen the attempt, or promise recovery of
unpublished output. Preserve incident evidence before that cutoff if needed:
stop/drain writers, back up the affected volume and durable receipts, and follow
the recovery runbook rather than editing canonical state or forcing publication.

## Upgrade and operational limits

Drain/reconcile manager and worker attempts and stop all storage writers before
the coordinated upgrade: older writers do not participate in the upload lock.
Back up durable databases and storage first, then deploy the selected worker and
updated manager/storage together. Do not run mixed old/new storage writers.

Retention is best effort while services and evidence are available. Outages,
quarantined containers, continuously busy uploads or very large trees can delay
cleanup. There is no hard total-disk bound: published versions, identity records
and immutable logs remain retained. Monitor disk capacity and use reviewed,
offline maintenance for ambiguous or oversized collections; never bulk-prune
containers/volumes or remove receipts to resolve uncertainty.

Acceptance uses isolated Redis/storage fixtures, real Docker identity/removal
tests and root-stack restart smoke tests. It covers receipt preservation, active
upload exclusion, symlink/final-file preservation, diagnostic failure/TTL guards,
and secret-sentinel omission. No production-host cleanup or paid provider call is
part of this acceptance. Broader runner fault acceptance is documented in
[M2.11 runner acceptance](RUNNER_ACCEPTANCE.md).
