# Manager Service

Port 8081. Redis-backed durable job admission and bounded background Docker
dispatch. See the [dispatch contract](../../docs/QUEUE_DISPATCH.md) for recovery,
persistence, configuration and upgrade requirements, and the
[Docker launch contract](../../docs/DOCKER_SPAWNER.md) for image/network setup and
socket authority. [Result recovery](../../docs/RESULT_RECOVERY.md) covers bounded
callbacks, verified worker exits, artifact receipts and terminal timeouts.
Paid-provider acceptance remains a later milestone.
Production startup now requires the Redis settings in the
[M2.9 durability and recovery runbook](../../docs/JOB_DURABILITY.md). The image
also includes `/app/recovery-audit`, a read-only consistency report for operators.
Terminal worker cleanup and bounded diagnostics follow the
[retention policy](../../docs/WORKER_RETENTION.md); ambiguous orphans stay quarantined.

## API

| Method | Route | Behavior |
| --- | --- | --- |
| POST | `/jobs` | Atomically admit/enqueue; first acceptance is 201 pending |
| GET | `/jobs/{job_id}` | Canonical current snapshot |
| POST | `/jobs/{job_id}/status` | Worker outcome update |
| POST | `/jobs/{job_id}/result` | Worker completion update |
| GET | `/health` | HTTP health |

Example admission body (the gateway normally supplies canonical identity):

```json
{
  "job_id": "692982f4-3a86-45f6-a84a-c749081c0b22",
  "owner_id": "user-id",
  "site_id": "site-id",
  "source_version": "initial",
  "target_version": "build-v1",
  "prompt": "Add a contact form"
}
```

Jobs progress from `pending` to `running` to `completed`/`failed`. Submission
does not wait for Docker. Retry the same identity to reconcile an unknown
submission result; do not submit a new identity to bypass uncertainty. Another
pending/running job for the same site receives `job_busy`. Pending callbacks and
terminal-to-running transitions are rejected. Callbacks are not yet authenticated
or fully fenced; restrict the manager to trusted internal/local clients.

## Configuration

All variables use the `PAGEWRIGHT_` prefix.

| Variable | Default / requirement |
| --- | --- |
| `PORT` | `8081` |
| `QUEUE_BACKEND` | `redis`; must support durable dispatch |
| `REDIS_ADDR` | `localhost:6379` |
| `REDIS_PASSWORD`, `REDIS_DB` | Empty, `0` |
| `DISPATCH_CONCURRENCY` | `4`, range 1–128, shared active-job cap |
| `DISPATCH_CLAIM_TTL` | `30s`, range 1s–1m, pre-launch claim lease |
| `LOCK_TTL` | `5m`; renewal remains unimplemented |
| `WORKER_SPAWNER` | `docker`; Kubernetes is a historical logging stub |
| `WORKER_IMAGE` | `pagewright-worker:m2.10`; [CLI contract and sandbox prerequisites](../../docs/WORKER_CLI.md) |
| `WORKER_APPARMOR_PROFILE` | Empty or reviewed `pagewright-worker`; requires explicit host profile installation |
| `WORKER_NETWORK` | Required dedicated Docker network |
| `WORKER_STORAGE_URL` | `http://storage:8080`, reachable by workers |
| `MANAGER_URL` | Worker-reachable callback URL |
| `WORKER_LLM_KEY` | Separate worker-only provider key |

Redis stores JSON job snapshots and a list plus token/lease/site/active dispatch
metadata under the configured queue namespace. Admission guards and new job
records do not expire. Launch intent is never automatically replayed. Read the
dispatch contract before upgrading Redis storage or changing concurrency.

## Running and tests

Use root Compose for the complete dependency topology; the standalone Compose
file only provides manager/Redis and requires worker-reachable storage separately.
`make run` and `make test` operate on this module. From the repository root,
`make test-integration`, `make test-docker-spawner` and `make smoke-stack` cover
the real Redis protocol, isolated Docker launch and persistence respectively.
