# PageWrightCloud

Pilot access now defaults to operator-provisioned accounts and disabled paid AI.
See [pilot limits and provisioning](docs/PILOT_LIMITS.md) before upgrading or
enabling an allowance. Remaining M4 security/release gates still apply; this is
not yet a remotely deployable pilot release.

An AI-assisted static website builder for non-technical users, built with Go services and a React UI.

## Current status

The reproducible development baseline (M0) and M1.1–M1.6 contracts, storage and initial-site bootstrap are implemented and locally verified. This is **not yet a working end-to-end MVP**: real worker execution, live job status and publishing still need integration. Start with [PLAN.md](PLAN.md) for evidence and [TODO.md](TODO.md) for remaining M1 work.

Verified baseline: clean dependency/image builds, zero-warning UI lint/build, Go package tests, PostgreSQL migration and API integration tests, compiler fixture output, and fresh-stack/container-recreation checks. Five existing tests remain skipped; see [test coverage and CI](docs/TESTING.md). Hosted CI runs after a push; local checks are not a hosted CI result. Dependency advisories and security hardening remain open before any remote pilot.

## Start locally

Use Linux or WSL2 with Docker Engine, Compose v2 and BuildKit. Local checks use Go 1.24.10, Node 24.11.1 (npm 11.6.2) and GNU Make. Go/Node need not be installed on the host just to run the containerized stack; Node is required for the startup smoke test. See [development setup](docs/DEVELOPMENT.md) for details.

From a checkout of this repository:

```bash
# Only if you do not already have .env:
cp .env.example .env
# Review .env; preserve existing configuration when resuming an old checkout.
docker compose up -d --build --wait
make docker-ps
```

Open http://localhost:3000 for the UI. Email/password registration and login work; new sites receive [retry-safe starter source](docs/SITE_BOOTSTRAP.md), not a generated or hosted website. Google OAuth and AI credentials are not needed for startup checks. Adding an API key alone will not complete the unfinished build pipeline.

This configuration is for trusted local development only: it publishes internal APIs/database ports and uses development credentials. Do not expose it to the internet; M4 is the remote-pilot gate. The root Compose database credentials are currently constants; changing `POSTGRES_*` in `.env` does not change them.

| Component | Default host port | Baseline role |
| --- | --- | --- |
| UI | 3000 | Built React app (Vite development server uses 5173) |
| Gateway | 8085 | Auth and site metadata |
| Manager | 8081 | Job API and Redis-backed coordination; execution integration pending |
| Storage | 8080 | Filesystem artifacts on the existing `nfs_data` named volume |
| Serving | 8083 | Hosting control API; lifecycle integration pending |
| nginx | 8084 | Hosted-content HTTP server |
| Themes | 8086 | Bundled starter theme registry |
| PostgreSQL / Redis | 5432 / 6379 | Local infrastructure |

There is no privileged NFS server in the single-host stack. The optional `worker` profile still builds a mock executor; it is not needed for the baseline. The compiler runs separately through its fixture command.

`PAGEWRIGHT_*_PORT` settings change host ports without changing internal container ports. For example:

```bash
PAGEWRIGHT_STORAGE_PORT=18080 docker compose up -d --build --wait
```

If you change the gateway host port, update browser-facing `VITE_PAGEWRIGHT_API_URL` and rebuild the UI. The MVP uses bounded job polling; WebSockets and the former socket URL setting are disabled. See [.env.example](.env.example) for defaults. Historical per-service Compose files are not the supported root-stack startup path.

Hosting uses a [supervised serving API/nginx pair](docs/HOSTING_LIFECYCLE.md) behind
the fixed public nginx proxy. Site changes are validated, acknowledged and rolled
back on reload failure; interrupted config transactions recover before startup.
Remove legacy no-op reload overrides and upgrade serving/proxy together without
deleting their data volumes. Preview assets/remaining URL wiring and pilot security
are still tracked in `TODO.md`.

### Inspect, stop and resume

```bash
curl --fail http://localhost:8085/health
make docker-logs-gateway
make docker-down
# Recreate containers using retained named volumes:
docker compose up -d --build --wait
```

Stopping with `make docker-down` retains database/artifact volumes. Do not use `make docker-clean` or `down --volumes` if you want to keep data. Before upgrading an old installation, back it up and read the [migration notes](docs/DEVELOPMENT.md#database-migrations). If the previous configuration left an obsolete NFS container, `docker compose down --remove-orphans` removes project containers without deleting named volumes; use it only for the intended development project.

The [text-only MVP capability policy](docs/MVP_CAPABILITIES.md) makes the gateway's
`PAGEWRIGHT_SITE_DOMAIN` authoritative for new sites. The UI loads it at runtime;
the old Vite default-domain setting is no longer used. Attachments, custom-domain
creation, aliases, Google sign-in and site deletion are unavailable. Upgrade UI
and gateway together and configure the namespace before resuming site setup.

## Run checks

```bash
make test-all             # Six Go modules; no running infrastructure
make test-compiler-smoke  # Compile starter fixture; check pages/assets
make test-integration     # Isolated PostgreSQL/Redis/API suites with race checks
make smoke-stack          # Isolated fresh startup and persistence after recreation

cd pagewright/ui
npm ci
npm run lint -- --max-warnings=0
npm run build
```

Integration/startup checks create unique disposable Compose projects and remove their own synthetic data. They do not reset the development database or require your application `.env`. Image builds and these checks are configured in [CI](.github/workflows/ci.yml). See [TESTING.md](docs/TESTING.md) for exact coverage, prerequisites and known skips.

## Hosting diagnostics versus MVP acceptance

[Deployment recovery](docs/DEPLOYMENT_RECOVERY.md) records publishing intent before
activation, fences retries and reconciles serving/DB state after lost responses or
gateway restart. Upgrade gateway and serving together; enrolled sites retain their
deployment records and cannot yet be deleted through the UI/API.

`make smoke-stack` verifies startup, UI assets, auth, initial-site source, immutable storage and persistence across recreation. It does **not** exercise AI editing, compilation through the worker, preview or publishing.

The optional [local-domain overlay](docker-compose.local-domain.yaml) uses `pagewright.io` as the app hostname. Map `pagewright.io`, a test hostname such as `demo.pagewright.io`, and `preview.demo.pagewright.io` to your Docker host in local DNS or your hosts file, then:

```bash
make docker-up-local-domain
make docker-verify-local-domain
# Only on a disposable development stack: seeds a user, site and placeholder HTML.
make docker-verify-local-domain-strict
make docker-down-local-domain
```

These older diagnostics assume default host ports. The basic check accepts a missing/unavailable site response. The strict check manually seeds files and reloads nginx; it mutates the selected stack and is not an isolated test or evidence of a working build/publish flow. See [preview hosting and upgrade guidance](docs/PREVIEW_ACTIVATION.md) for configured URLs, separate preview DNS/TLS and migration of existing sites.

The actual MVP acceptance journey is create → edit → completed version → preview → publish → second edit → rollback, with failed builds preserving live content. It remains planned in M1–M4.

## Project map

Gateway owns users/sites/versions, manager coordinates jobs, worker will edit and compile versioned source, storage keeps artifacts, and serving/nginx deliver generated public files. The compiler renders Markdown/content with a trusted theme. These are intended responsibilities, not a claim that all interfaces or security boundaries are complete.

- [MVP plan](PLAN.md) and [execution checklist](TODO.md)
- [Development and migrations](docs/DEVELOPMENT.md)
- [Checks, CI and coverage limits](docs/TESTING.md)
- [Architecture notes](pagewright/README.md)
- [Compiler](pagewright/compiler/README.md) and [themes](pagewright/themes/README.md)
- [Gateway](pagewright/gateway/README.md), [manager](pagewright/manager/README.md), [storage](pagewright/storage/README.md), [worker](pagewright/worker/README.md), [serving](pagewright/serving/README.md), [UI](pagewright/ui/README.md)

Older component documentation and code reviews may describe intended behavior or historical commands; the root setup and tracked acceptance evidence above are authoritative for this baseline.

## License

See [LICENSE](LICENSE).
