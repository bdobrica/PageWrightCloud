# Baseline checks

See [development prerequisites](DEVELOPMENT.md) for pinned toolchains. Run from the repository root:

| Check | Command | Scope |
| --- | --- | --- |
| Go packages | `make test-all` | Six Go modules; no running infrastructure required |
| Compiler fixture | `make test-compiler-smoke` | Starter theme produces three pages and required assets in a temporary directory |
| UI | `cd pagewright/ui && npm ci && npm run test:contracts && npm run lint -- --max-warnings=0 && npm run build` | Lockfile install, job response parsers, zero-warning lint, production build |
| Integration | `make test-integration` | Gateway PostgreSQL/migrations/CLI, manager/storage HTTP tests and worker callback contract round-trips in an isolated Compose project |
| Images | `docker compose --env-file /dev/null --profile worker build` | Selected service images, including the optional mock worker |
| Startup/recreation | `make smoke-stack` | Fresh stack, UI assets, auth, site metadata and storage; repeat after container recreation with volumes retained |

The integration and startup checks use disposable, uniquely named projects and remove only their own test volumes. They do not require your development stack or `.env`. The image-build command only builds images; it does not start or remove containers. Startup needs Node as well as Docker. No paid AI credentials are needed.

## CI

[MVP baseline](../.github/workflows/ci.yml) runs four independent jobs on pushes, pull requests and manual dispatch: Go/compiler/workflow validation, UI, isolated integration, and image/startup/recreation checks. Workflow configuration follows the upstream [checkout v4](https://github.com/actions/checkout/tree/v4), [setup-go v5](https://github.com/actions/setup-go/tree/v5), and [setup-node v4](https://github.com/actions/setup-node/tree/v4) documentation. YAML and workflow expressions are checked with [actionlint v1.7.7](https://github.com/rhysd/actionlint/releases/tag/v1.7.7).

These checks have local verification evidence in [PLAN.md](../PLAN.md). A hosted CI result requires pushing the branch; local validation is not a claim that GitHub Actions has run.

The [job wire contract](JOB_CONTRACT.md) describes M1.1's gateway/manager/worker/UI
schemas and HTTP acceptance tests. The instruction provider is faked in contract
tests; no paid provider or worker executor is invoked.

[M1.2 submission tests](BUILD_SUBMISSIONS.md) add PostgreSQL reservation/outcome
atomicity, concurrent duplicate HTTP requests, failed/lost delivery, restart
replay, real Redis deduplication scripts, and UI retry identity. The integration
harness provides `TEST_REDIS_ADDR` for the isolated Redis tests; they do not flush
an existing application's Redis data.

## Known skipped tests

Five existing tests explicitly skip. Passing package checks do not mean these behaviors have coverage. Keep verbose test output visible and remove skips only when the underlying behavior and tests are repaired.

| File | Test | Gap / follow-up |
| --- | --- | --- |
| `pagewright/worker/internal/codex/executor_test.go` | `TestExecutorMock` | Disabled output parsing; M2.11 |
| Same file | `TestExecutorKill` | Cancellation race; M2.11 |
| Same file | `TestParseOutput` | Wrapper stdout parsing; M2.11 |
| `pagewright/serving/internal/config/config_test.go` | `TestLoadConfigDefaults` | Environment-sensitive defaults; M4.12 |
| `pagewright/serving/internal/artifact/manager_test.go` | `TestCleanupOldVersions` | Flaky cleanup coverage; M3.8 / M4.12 |

## What passing does not prove

- The compiler fixture checks output presence, not complete rendering correctness or adversarial-input safety; the compiler currently has no Go test files.
- UI lint/build does not exercise browser interactions.
- Integration checks cover canonical migrations, rollback, restart, legacy adoption, concurrent startup and invalid history/data, but not a complete edit/build/publish workflow.
- Startup creates metadata and stores synthetic bytes. It does not build a website, run a real AI worker, publish, validate tenant isolation, or prove production readiness.
- Local-domain verification targets are seeded hosting diagnostics, not the MVP acceptance test. They mutate the selected development stack and are not isolated like `smoke-stack`.

M1–M3 add the missing pipeline/browser coverage; M4 covers pilot hardening. See [TODO.md](../TODO.md) for acceptance criteria.
