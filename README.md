# PageWrightCloud

An AI-assisted static website builder: create a site, edit it through chat, preview
an immutable version, publish it, and roll back without rebuilding.

## Verified state

As of 2026-09-11, M1–M3 and M4.1–M4.8/M4.10–M4.13 are implemented and locally
verified. Acceptance covers real services, sandboxed workers, compilation, browser
publishing/rollback, cross-user isolation, restart recovery, delivered-email reset
using a test SMTP server, and coordinated backup/restore. A separately authorized
real-AI smoke passed. See [release evidence and limitations](docs/RELEASE_ACCEPTANCE.md).

A closed pilot runs on pagewright.io, but **public release sign-off is still open**:
M4.9 public failure-recovery/full-browser gates remain, and the latest hosting-cache
fix has not been rolled out there. Local evidence does not establish that the pilot
runs this checkout. Real inbox delivery and production restore are not established
by local fixtures. No hosted CI result is claimed without a corresponding pushed
run. [TODO.md](TODO.md) tracks remaining work; [PLAN.md](PLAN.md) retains evidence.

## Local setup

Use Linux/WSL2 with Docker Engine, Compose v2 and BuildKit. The verified development
toolchain is Go 1.24.10 and Node 24.11.1. From the repository root:

```sh
# Only for a new checkout without an existing .env:
cp .env.example .env
```

Privately configure independent PostgreSQL/Redis passwords and JWT/service secrets
using [configuration security](docs/CONFIGURATION_SECURITY.md) and
[internal authentication](docs/INTERNAL_AUTH.md). Keep closed signup and zero AI
allowance until intentionally configured. Editing an env file does not rotate an
existing database's stored password.

```sh
docker compose --profile worker build worker
docker compose up -d --build --wait
docker compose ps
```

Check [worker host prerequisites](docs/WORKER_ISOLATION.md), including the reviewed
AppArmor profile where required. Never use privileged or unconfined workers.
Provision an account using the [operator procedure](docs/PILOT_LIMITS.md), then
open http://localhost:3000. AI needs an explicit allowance and provider setup;
adding a key alone does not enable it. No paid tests run by default.

Root Compose publishes UI (3000), gateway (8085) and generated-content edge (8084);
internal APIs and databases are private. Root bindings are not a public deployment
recipe. Use the reviewed [pilot HTTP-01 runbook](docs/PILOT_HTTP01.md) and pilot
overlay for loopback bindings behind host Nginx. Configure separate app/live/preview
origins and local DNS before testing hosted URLs; see [development](docs/DEVELOPMENT.md).
Legacy service-only Compose and manual site-seeding diagnostics are not the release path.

```sh
docker compose stop
# Resume retained state:
docker compose up -d --wait
```

Never use `down --volumes` or `make docker-clean` on data you intend to preserve.
Back up before upgrades; [operator runbooks](docs/OPERATIONS.md) cover recovery.
Attachments, customer domains/aliases, OAuth and deletion are unsupported; see the
[capability decision](docs/adr/0022-architecture-decisions-mvp-capabilities.md).

## Checks

```sh
make test-all
make test-integration
make smoke-stack
make test-compiler-smoke
node scripts/check-doc-links.mjs
cd pagewright/ui
npm ci
npm run test:contracts
npm run lint -- --max-warnings=0
npm run build
```

Integration/smoke tests use disposable projects, not application volumes. See
[testing](docs/TESTING.md), [browser acceptance](docs/BROWSER_ACCEPTANCE.md) and the
explicitly authorized [real-provider smoke](docs/PROVIDER_SMOKE.md).

## Documentation

- [Operations and verification](docs/README.md): incidents, secrets, capacity,
  backup/restore, rollback, TLS and acceptance.
- [Architecture decisions](docs/adr/README.md): identities, submissions, fencing,
  immutable artifacts, worker execution and deployment protocols.
- [Plan](PLAN.md) and [checklist](TODO.md): evidence and remaining gates.

Gateway owns identity/site metadata; manager coordinates durable jobs; isolated
workers edit source and run the trusted compiler; storage retains immutable
artifacts; serving/Nginx hosts selected versions. Component READMEs contain detail
and historical commands; use the root setup and current runbooks for deployment.

## License

See [LICENSE](LICENSE).
