# Supervised hosting lifecycle (M3.5)

## Supported topology

The root Compose `serving` container now runs the Go API and hosting nginx under
`hosting-supervisor`, with `tini` as PID 1. Both processes must be alive. Either
child's exit stops its sibling and fails the container, allowing Compose's
`unless-stopped` policy to restart the pair. Shutdown forwards TERM to both process
groups, allows ten seconds, then kills/reaps survivors. The API drains requests
for up to five seconds. Compose grants twenty seconds before forcible termination.

| Endpoint | Responsibility |
| --- | --- |
| Public `nginx`, host port 8084 by default | Fixed proxy to `serving:80`; preserves Host and re-resolves Docker DNS after replacement |
| `serving:80` | Actual site/preview nginx; reads managed configs and public artifact symlinks |
| `serving:8083` | Existing management API; trusted-network exposure remains M4 work |
| `127.0.0.1:8089` inside serving | Generation/readiness probe; neither published nor proxied |

The public proxy no longer mounts writable site configuration or artifacts. It
requires no reload when sites change. Serving has no Docker socket, privileged
mode or host PID namespace. Worker sandbox/resource restrictions are unchanged.
Integration uses the same production image, supervisor, nginx config and proxy,
not the former `nginx && exec runner` test-only startup override.

## Configuration transaction

One supervisor holds an exclusive advisory writer lock on the configuration
volume. API mutations are serialized inside its manager. A second supported
supervisor cannot recover or write that volume concurrently.

For create/update/delete/maintenance/first-preview operations:

1. Save prior config bytes/existence and the prior generation config to a synced
   `.pagewright-transaction` journal. Temporary/journal/lock names are hidden and
   excluded from nginx's site glob.
2. Write and sync a temporary config, rename it atomically, and sync the directory.
   Install a random generation marker on the loopback-only listener.
3. Run `nginx -t`, then the configured reload command (default `nginx -s reload`).
   Each command has a ten-second limit and does not invoke a shell.
4. Within five seconds, require a fresh HTTP connection to observe that exact
   generation. A successful signal or no-op reload command alone is insufficient.
5. Remove/sync the journal only after acknowledgment, then return success.

Failure restores the previous files, validates/reloads them, and confirms the
previous generation. If rollback cannot be confirmed, retain the journal, reject
further writes and report unhealthy. Readiness requires nginx and no pending
transaction. Brief health failures during a configuration transaction are expected.
First-preview routing preserves existing aliases/disabled policy.

Activation routes establish confirmed routing **before** changing an artifact
pointer. Therefore validation/reload failure does not switch that pointer. This
does not make activation atomic with the gateway database: M3.7 still owns that
reconciliation; M3.8 still owns atomic artifact pointer switching/extraction.

## Restart and upgrade

Before starting nginx, the supervisor acquires the writer lock and restores any
interrupted journal to its prior bytes. Even a change nginx had loaded is rolled
back if its transaction was not durably finalized. It then installs a startup
generation and validates the full configuration. Corrupt journals, invalid saved
configurations or an occupied writer lock fail startup rather than discarding data.
Site configs, artifacts and live/preview symlinks remain in their existing volumes.

Upgrade serving and the public proxy together during a maintenance window, after
backing up configuration, artifacts and DB state. Do not delete volumes. Remove
old `PAGEWRIGHT_NGINX_RELOAD_COMMAND=true` overrides; the local-domain overlay no
longer supplies one. Preserve the default config-directory paths unless you also
provide a matching nginx configuration. Do not run bare `serving-runner` or multiple
writers against the same volume; use the image's default supervised entry point.

For a failed rollback, stop mutations and inspect serving logs and the journal
from a trusted operator environment. Restarting the stopped pair retries recovery.
For corrupt evidence or invalid pre-existing configs, preserve a backup and repair
from a verified prior configuration before restarting. Do not blindly delete the
journal to make health green. Arbitrary disk loss, malicious filesystem changes,
distributed HA and automatic operator-corruption repair are not claimed.

## Acceptance evidence

Unit tests cover serialization, invalid input, validation/reload failure, rollback,
interrupted journal recovery, corrupt-evidence refusal, writer exclusion and child
shutdown. Race-enabled integration runs real nginx: invalid syntax is rejected and
prior bytes restored; a no-op reload fails generation acknowledgment and restores
the old configuration. The deterministic worker/compiler/storage journey verifies
first preview before publish and unchanged live output through the production proxy.

The root-stack smoke verifies real enable/disable routing, nginx-master crash causing
container restart, and saved configuration after abrupt serving/Redis/gateway/manager
stop and recreation. Its virtual-host requests use Node's HTTP client because the
installed `fetch` implementation ignores a custom Host header. Only disposable
test resources are used; no paid provider, production data migration or remote
deployment is part of this milestone. Preview asset/base-path behavior remains M3.6,
full browser acceptance M3.12, and pilot security/TLS M4.
