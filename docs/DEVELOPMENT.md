# Development baseline

The supported local baseline is Go 1.24.10, Node 24.11.1 (npm 11.6.2), GNU Make,
and Docker Engine with the Compose v2 plugin and BuildKit enabled. Compose 2.40.3
is the version used for the initial verification. Use Linux or WSL2 with Docker
available to that environment. Run commands from the repository root.

The compiler's `go.mod` requires Go 1.24.10; the other modules declare Go 1.22.
All selected Go image builders use 1.24.10. The UI lockfile's Vite and React
plugin require Node `^20.19.0 || >=22.12.0`; Node 18 is incompatible. `.nvmrc`
and the UI builder select Node 24.11.1. These are a verified compatibility
baseline, not a claim that dependencies or base images have passed a security audit.

```bash
nvm install
nvm use
go version
docker compose version
cd pagewright/ui
npm ci
npm run build
cd ../..
docker compose --profile worker build gateway manager storage serving themes ui worker
```

`npm ci` installs the committed lockfile. Docker build contexts exclude local
dependencies, build output and environment files; Vite configuration is passed
using explicit build arguments. The themes image uses Dockerfile heredoc syntax,
so BuildKit is required. The compiler is a local binary at this milestone, not
a standalone Compose image. The selected worker image still contains a mock
executor until M2; a successful image build does not establish a working AI build.

## Checks

```bash
make test-all          # All six modules, no external services
make test-compiler-smoke # Starter fixture pages/assets, temporary output
make test-integration  # Isolated PostgreSQL, Redis, manager/storage and Go runner
cd pagewright/ui
npm run test:contracts
npm run lint -- --max-warnings=0
npm run build
```

`make test` / `make test-unit` in a Go service run untagged package tests.
`make test-integration` in a service delegates to the root integration harness.
The harness runs worker, gateway, manager, storage and serving suites with `-race`, fresh state
and no published ports, then removes only its generated test project and data,
including on failure. It does not load the application `.env`. Docker image
layers remain cached. Worker coverage includes manager callback contracts and
produces the shared artifact fixture consumed by gateway and serving tests.
Serving coverage verifies downloads and extraction, not nginx activation;
there is no compiler integration suite yet. See [artifact transport](ARTIFACT_TRANSPORT.md).
Manager tests currently exercise its mock spawner, HTTP API, Redis and locking;
they do not claim worker execution coverage. Gateway tests use a private schema.
Direct tagged tests require explicit test database/service URLs; prefer the harness.

See [TESTING.md](TESTING.md) for the CI jobs, the five remaining skipped tests
and the difference between baseline verification and full MVP acceptance.

## Database migrations

Gateway startup applies the embedded `pagewright/gateway/migrations/*.up.sql`
files. SQL files are the single source; binaries and integration tests use the
same runner. Pending migrations and their version records commit together under
a PostgreSQL transaction advisory lock. Startup has a bounded migration timeout.
Repeated startup is safe, and a binary refuses unknown newer recorded versions.

Existing databases with versions 1–5 are upgraded by migration 006, which
reconciles column widths, defaults, indexes and the user identity constraint.
If legacy users have neither a password hash nor a complete OAuth identity,
the upgrade fails and rolls back. Back up the database and repair those accounts
deliberately before retrying; migrations do not delete or invent credentials.
File-based/manual schemas without version tracking are also covered by tests.
Rollback after application upgrades should use a backup and the matching binary;
the startup runner does not automatically execute historical `.down.sql` files.

Migration 007 adds durable build submissions and preserves existing version rows
without inventing missing job mappings. See [submission/retry semantics](BUILD_SUBMISSIONS.md)
before integrating API callers: builds now require an `Idempotency-Key` header.

## Single-host storage and startup smoke test

Root Compose uses the existing filesystem backend directly on a named volume.
Its backend identifier remains `nfs` for compatibility, but no NFS daemon, mount
or privileged container is needed. The volume remains named `nfs_data` to retain
existing installations' artifacts. Container storage uses `/nfs`; port overrides
change only host-published ports, while service-to-service URLs and healthchecks
use stable internal ports. Container names are scoped by Compose project.

```bash
PAGEWRIGHT_STORAGE_PORT=18080 make docker-up
make smoke-stack
```

The smoke test uses a generated project, empty named volumes and randomly
allocated host ports. It checks auth, a site DB record, storage round-trip, UI,
theme registry and nginx configuration, recreates containers with the same
volumes, and verifies the data again. Its cleanup removes only its own volumes
and containers. Node from the documented baseline is required. This checks
startup/persistence; it does not compile or publish an AI-generated site, and
the separate serving/nginx reload limitation remains M3 work.
