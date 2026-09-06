# Bounded result delivery and recovery (M2.8)

Selected worker: `pagewright-worker:m2.8`. This extends the
[M2.7 fenced commit contract](FENCED_COMMITS.md); it does not weaken worker
commit checks, reacquire expired leases, restart workers, or overwrite artifacts.

## Callback delivery

The runner sends at most four identical result POSTs, with exponential delays
of 250/500/1000 milliseconds. Each HTTP request has a three-second timeout;
the entire delivery has a 25-second deadline, inside the existing 16-minute
whole-runner watchdog. Redirects are disabled and response bodies are bounded
to 64 KiB. A successful response must contain the matching attempt and outcome.

After an ambiguous response, including a malformed/truncated 200 or terminal
409, the runner looks up the job. Only the same identity, status, manifest path,
result and error message confirm delivery. Another attempt/outcome is not
success. Permanent 4xx responses (except 408/429) stop POST retries after lookup.
The manager still rejects every terminal callback duplicate; admission remains
idempotent. Lookup resolves ambiguity without making terminal writes replayable.

An uncertain manifest-upload or completion acknowledgement does **not** trigger
a contradictory failure callback. The runner exits and leaves reconciliation to
the manager. Earlier execution/artifact/log failures still report failure with
the same bounded delivery policy. Storage uploads themselves are not automatically
replayed; identical active-attempt retries remain allowed by M2.7.

## Manager reconciliation

The Docker manager scans durable active attempts on a five-second ticker without
overlapping scans. At most 128 candidates and four concurrent observations
are allowed, with a five-second listing deadline and 25-second per-candidate
deadline. One unavailable candidate does not prevent processing other returned
candidates. All replicas may reconcile; Redis decides the single terminal winner.

Eligible jobs have a running fenced attempt and a Redis dispatch timestamp.
`started`/`uncertain` attempts are checked for worker exit. `launching` intent is
left alone until its Redis-clock lifetime expires (default 30 minutes,
`PAGEWRIGHT_WORKER_TIMEOUT`). Missing containers, created-but-not-started workers,
and daemon failures are not proof that an in-flight launch cannot run.
They wait for the deadline, without replaying create/start.

Docker lookup uses the deterministic job name even when a launch acknowledgement
was lost. It verifies the name, worker/job/site/network labels, actual network,
and full launch attempt from the container configuration before trusting exit
state or killing anything. Timeout kills target only a verified immutable
container ID. No broad delete, privileged container, profile change, or
namespace/security relaxation is introduced.

For an exited worker or expired lifetime, the manager reads the exact immutable
receipt and checks artifact, private-log and manifest bytes against its SHA-256
and size reservations. Reads are bounded to 64 MiB/4 MiB/4 MiB, five seconds per
request, with no redirect or transparent decompression. A complete matching set
can recover `completed` even if the callback and lease have expired: this records
already-committed bytes, not new worker authority.

| Observation | Terminal outcome |
| --- | --- |
| All three same-attempt reservations and exact files | `completed`, canonical manifest path |
| Confirmed exit, partial reservations or missing/mismatched bytes | `failed / artifact_incomplete` |
| Confirmed exit without reservations | `failed / worker_exit` (or `worker_oom`) |
| Lifetime exceeded without complete output | `failed / worker_timeout` |
| Definite create/launch rejection | Existing `failed / spawn_failed` dispatch path |
| Unavailable storage, invalid receipt, superseded attempt | No speculative outcome; retain for reconciliation |

The manager-only Redis recovery operation compares the attempt, current fence,
site/active reservations and **exact observed receipt** atomically. A new digest
reservation or a winning callback invalidates the observation. Terminal outcome,
matching lease deletion and site/capacity release occur together. Another lease
is never deleted; terminal history never changes. No HTTP recovery bypass exists.
Definite spawn failure now also releases the matching lease inside the dispatch
outcome script, rather than depending on a second lock-release request.

## Reserved but missing bytes

A receipt is never deleted or altered to resolve uncertainty. Missing or changed
files after exit/deadline produce a conservative failed outcome and preserve the
receipt. The expired/terminal worker cannot reserve or retry publication again.
Recovery neither reconstructs missing bytes nor authorizes a new writer.

A storage operation already holding a reservation may still finish materializing
those exact bytes after observation, as defined by M2.7. This does not reopen a
failed job or change its terminal result. Completed materializations remain
immutable/readable; receipt and orphan/version retention needs M2.9/M2.10 policy.
Transient storage failure is not a definitive 404: it retains capacity until
evidence becomes available. Renewal still ends at the configured lifetime.

If Docker is unavailable at timeout, terminal fencing can proceed when storage
evidence is sufficient even though termination cannot be confirmed. The container
may remain; resource ceilings/watchdog are unchanged. Orphan-container cleanup
is M2.10, not permission to delete unrelated containers.

## Operations and remaining scope

Drain/reconcile before a coordinated manager/storage/worker upgrade. Do not reset
fences, remove receipts, or hot-mix old active jobs lacking required metadata.
Legacy/corrupt/missing Redis records remain fail-closed and need the M2.9 restart
and durability policy. Kubernetes remains a stub without this Docker reconciler.
All replicas must share configuration, Redis, storage and the same Docker daemon;
multi-host scheduling is not supported.

Internal APIs/Redis/Docker remain trusted infrastructure; fencing is not service
authentication. M4 owns scoped credentials. M2.9 covers broader restart/durability
and gateway ambiguity; M2.10 covers cleanup/retention; M2.12 covers an explicitly
cost-bounded provider run. This work does not claim production or paid-provider
acceptance.
