# Docker worker launch contract (M2.1)

The supported worker is `pagewright/worker`, not the historical
`pagewright/manager/cmd/worker` prototype. Root Compose, the worker Makefile and
the manager's worker build targets agree on `pagewright-worker:m2.7`. The old
manager `Dockerfile.worker` is retained only as explicitly labelled history.

Before submitting jobs, build the selected image:

```sh
docker compose --profile worker build worker
docker compose up -d --build
```

Do not start a standing `worker` Compose service without a canonical job. The
manager creates one container per accepted job. It does not pull missing images;
operators build/load the explicit tag (or configure a digest) first. Untagged and
`:latest` images are rejected. This tag identifies the current launchable runner,
which contains pinned real Codex CLI 0.153.4. See [CLI and sandbox acceptance](WORKER_CLI.md).
Trusted compiler integration remains M2.4; a successful launch does not prove a
paid-provider edit or a compiled build.

## Configuration and authority

The implementation uses the Unix-socket [Docker Engine v1.45 API](https://docs.docker.com/reference/api/engine/version/v1.45/)
with bounded acknowledgements and HTTP timeouts. The daemon must support that
API version. Remote TCP/context selection, shell interpolation, image pulls,
privileged workers, worker host mounts and host-port publication are not used.

M2.7 requires the [fenced attempt/commit contract](FENCED_COMMITS.md); drain and
upgrade manager, storage and worker together rather than mixing generations.

Root Compose derives the worker network from its own project name, sets `/work`
and `http://storage:8080`, and supplies the manager callback URL. For standalone
manager use, configure:

| Variable | Default / requirement |
| --- | --- |
| `PAGEWRIGHT_WORKER_IMAGE` | `pagewright-worker:m2.7`; explicit non-latest tag or digest |
| `PAGEWRIGHT_WORKER_APPARMOR_PROFILE` | Empty for Docker default; `pagewright-worker-proc` for M2.6 on AppArmor hosts after explicit profile loading. Legacy `pagewright-worker` retains Docker proc defaults. See [isolation policy](WORKER_ISOLATION.md). |
| `PAGEWRIGHT_WORKER_NETWORK` | Required dedicated Docker network; root Compose supplies it |
| `PAGEWRIGHT_DOCKER_SOCKET` | `/var/run/docker.sock`; absolute Unix path |
| `PAGEWRIGHT_WORKER_WORK_DIR` | `/work`; clean path at or below `/work` |
| `PAGEWRIGHT_WORKER_STORAGE_URL` | `http://storage:8080`; must be reachable from worker network |
| `PAGEWRIGHT_MANAGER_URL` | Set a callback URL reachable from that network, not host localhost |
| `PAGEWRIGHT_WORKER_LLM_KEY` | Empty; separate operator-provided worker credential |
| `PAGEWRIGHT_WORKER_LLM_URL` | `https://api.openai.com/v1` |

Only the job snapshot, correlation identity, workspace path, manager/storage
endpoints and worker provider key/URL are injected. The gateway provider key,
JWT secret, Redis password and manager environment are not copied. Job/secret
values and raw Docker errors are not logged or returned by the spawner.
The worker retains its image-defined entrypoint and working directory; writable
job data goes into a private, 256 MiB tmpfs at the configured workspace path.
UID/GID 1000 owns the workspace; capabilities are dropped and privilege escalation
is disabled. An embedded default-deny seccomp profile adds only the nested-user-
namespace/mount operations required by bubblewrap. The outer process cannot mount;
the CLI enforces workspace writes and no command networking. Containers do not
restart automatically or auto-remove, preserving daemon evidence for recovery.

**The manager alone receives the host Docker socket. This is host-level authority,
not a sandbox.** Run this stack locally with trusted operators only; the internal
manager/storage APIs are not authenticated yet. Credential allowlisting is not
per-job token issuance or an AI sandbox. M2.6 supplies worker runtime limits and
isolation; scoped service authentication remains M4.

## Outcomes and retry safety

Container names derive deterministically from job identity; labels identify job,
site, worker role and network. Create must return a valid container ID before
start is attempted. A name conflict is never adopted or restarted.

- Invalid launch input or explicit create rejection (400/401/403/404/406/422)
  yields `ErrNotStarted`. Manager persists `failed/spawn_failed` and releases the
  lock; a missing image is included in this category.
- Lost/truncated/invalid create acknowledgements, name conflicts, daemon errors,
  redirects and all unsuccessful/uncertain start acknowledgements are ambiguous.
  The dispatcher records an uncertain outcome, retains the durable running reservation
  and does not release the lock. Known container ID/name is stored via a metadata
  merge, without overwriting a fast terminal callback.
- Retrying the same job/idempotency key retrieves the reservation; it never
  creates a second container. Gateway already treats unknown submission errors
  as uncertain and reconciles the same identity.

M2.7 renews current attempts up to a bounded lifetime and enforces fencing at
storage/result commit. Do not infer that an expired lock or a container in
`created`/`exited` state proves execution never happened. Restart reconciliation
and recovery from an ambiguous launch remain M2.8/M2.9.
M2.2 supplies [bounded asynchronous dispatch](QUEUE_DISPATCH.md):
submission acknowledges `201 pending`, and launch failures are read through job
status rather than returned synchronously. Exited containers retain their environment in daemon metadata until
removed; cleanup/retention and redacted log policy remain M2.10. No automatic
production container deletion is introduced here.

## Verification

`make test-docker-spawner` explicitly grants its isolated runner Docker socket
access. It builds a no-AI fixture image, creates a unique network, performs real
create/start/inspect operations, verifies reachable endpoints and writable private
workspace, checks credential/socket exclusions, refuses duplicate names, and
checks definitive missing-image rejection. Tests and cleanup remove only workers
labelled with that generated test network and that run's Compose resources.

Unix-socket simulated-engine tests cover the wire contract and lost/invalid
acknowledgements; Redis dispatcher tests cover retained reservations, retry deduplication
and fast callbacks. CI runs this dedicated daemon test separately from
`make test-integration`. The latter builds a manager with the `integration` tag
and uses `test-manual` to preserve M1's deterministic launch bridge. Production
builds do not register that spawner; no runtime production mock flag is added.
