# Baseline checks

See [development prerequisites](DEVELOPMENT.md) for pinned toolchains. Run from the repository root:

| Check | Command | Scope |
| --- | --- | --- |
| Go packages | `make test-all` | Six Go modules; no running infrastructure required |
| Compiler fixture | `make test-compiler-smoke` | Starter theme produces three pages and required assets in a temporary directory |
| UI | `cd pagewright/ui && npm ci && npm run test:contracts && npm run lint -- --max-warnings=0 && npm run build` | Lockfile install, job response parsers, zero-warning lint, production build |
| Integration | `make test-integration` | Gateway PostgreSQL/migrations/CLI, manager/storage HTTP, worker callbacks, and shared artifact round-trips through worker/gateway/serving in isolated Compose |
| Images | `docker compose --env-file /dev/null --profile worker build` | Selected service images, including the optional mock worker |
| Startup/recreation | `make smoke-stack` | Fresh stack, UI assets, auth, bootstrap storage; SIGKILL this disposable project's Redis/gateway/manager, recreate with retained volumes, verify history recovery and missing-evidence non-redispatch |

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

[M1.3 artifact transport tests](ARTIFACT_TRANSPORT.md) pack one shared fixture
with the worker, persist it through real storage, and compare exact bytes and
extracted files across all three storage clients. Gateway's authenticated
download and interrupted upstream response are also exercised. Serving tests
use its real extractor, not nginx activation or the publish workflow.

[M1.4 metadata tests](VERSION_METADATA.md) verify manifest-last completion,
required private-log writes, hidden partial/legacy versions, safe retry,
backend restart and worker callback gating. Integration uses the actual worker
metadata client and checks committed-version visibility through gateway.
Startup smoke also checks bootstrap private metadata after container recreation.
M2.9 additionally runs the packaged read-only recovery audit, verifies an
acknowledged manager job survives AOF restart, reconciles its PostgreSQL lifecycle
history, and preserves a gateway claimed-before-send fixture as uncertain.
See [durability acceptance and operator limits](JOB_DURABILITY.md).

[M1.5 immutability checks](IMMUTABLE_VERSIONS.md) add concurrent instance/process
writes, retries, conflict preservation, failed-stream cleanup and disabled
deletion. Startup/recreation smoke verifies all immutable object types and both
deletion responses. UI contracts guard the removed deletion action.

[M1.6 bootstrap checks](SITE_BOOTSTRAP.md) verify deterministic starter source,
compiler compatibility, durable reservation/reconnect, partial-write/lost-response
recovery, concurrent creation and pending-build rejection. Startup/recreation
now creates the initial source through gateway rather than metadata alone.

[M1.7 archive checks](ARCHIVE_LAYOUT.md) cover source/public separation, unsafe
paths and links, private runtime-file canaries, archive/decompression limits,
legacy source-only compatibility, staged failure preservation and concurrent
deployment retries. Worker and serving enforce identical policy files (checked
by integration). Source survives a second edit; private paths return 404 from
the public HTTP root. These checks do not certify compiler or nginx behavior.

[M1.8 compiler fixtures](COMPILER_CONTRACT.md) exercise real starter rendering,
Markdown/MDX discovery and failures, navigation/TOC consistency, tokens/assets,
malformed configuration, escaping, filesystem boundaries and CLI exit codes.
Run `go test -race -count=1 -coverpkg=./internal/... ./...` in
`pagewright/compiler` for race-enabled cross-package coverage.

[M1.9 version API checks](VERSION_API.md) verify serving JSON fields, normalized
owner-checked committed-version lists, pagination and upstream failures against
real storage/PostgreSQL. UI contracts exercise version parsing and FQDN routing.
These do not establish the full publish/preview journey.

## Known skipped tests

Two serving tests explicitly skip. Executor parsing and cancellation tests were
restored in M2.3/M2.6 and remain enabled. M2.11 adds
[actual runner failure and restart acceptance](RUNNER_ACCEPTANCE.md).
Keep verbose test output visible and remove skips only when the underlying
behavior and tests are repaired.

| File | Test | Gap / follow-up |
| --- | --- | --- |
| `pagewright/serving/internal/config/config_test.go` | `TestLoadConfigDefaults` | Environment-sensitive defaults; M4.12 |
| `pagewright/serving/internal/artifact/manager_test.go` | `TestCleanupOldVersions` | Flaky cleanup coverage; M3.8 / M4.12 |

## What passing does not prove

M2.10 adds Redis-backed terminal cleanup/diagnostic TTL guards, real-Docker
non-forced removal, live-upload locking and staging preservation tests, and
bounded private-log redaction tests. See [retention acceptance](WORKER_RETENTION.md).
These use disposable local resources, not production-host cleanup.

- Compiler fixtures cover specific rendering and adversarial regressions, not all possible HTML/CSS content or hostile concurrent filesystem changes.
- UI lint/build does not exercise browser interactions.
- Integration checks cover canonical migrations, rollback, restart, legacy adoption, concurrent startup and invalid history/data, but not a complete edit/build/publish workflow.
- Startup creates initial source and stores synthetic transport fixtures. It does not compile through the worker, run real AI, publish, validate tenant isolation, or prove production readiness.
- Local-domain verification targets are seeded hosting diagnostics, not the MVP acceptance test. They mutate the selected development stack and are not isolated like `smoke-stack`.

M1–M3 add the missing pipeline/browser coverage; M4 covers pilot hardening. See [TODO.md](../TODO.md) for acceptance criteria.
