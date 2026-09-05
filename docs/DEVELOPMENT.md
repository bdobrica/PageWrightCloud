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
make test-integration  # Isolated PostgreSQL, Redis, manager/storage and Go runner
cd pagewright/ui
npm run lint -- --max-warnings=0
npm run build
```

`make test` / `make test-unit` in a Go service run untagged package tests.
`make test-integration` in a service delegates to the root integration harness.
The harness runs gateway, manager and storage suites with `-race`, fresh state
and no published ports, then removes only its generated test project and data,
including on failure. It does not load the application `.env`. Docker image
layers remain cached. There are no worker/serving/compiler integration suites yet.
Manager tests currently exercise its mock spawner, HTTP API, Redis and locking;
they do not claim worker execution coverage. Gateway tests use a private schema.
Direct tagged tests require explicit test database/service URLs; prefer the harness.
