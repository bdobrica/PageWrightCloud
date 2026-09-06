# Real-service browser acceptance (M3.12)

Run from the repository root with Docker Engine/Compose and an installed matching
Playwright/Firefox pair. No provider account, key, DNS setup or application data is
needed. The Docker host must support the production worker sandbox; do not use
privileged containers or weaken sandbox settings to make this test pass.

```sh
node --test scripts/browser-provider.test.mjs
node scripts/browser-acceptance.mjs /absolute/path/to/playwright/index.mjs /absolute/path/to/firefox
```

The runner builds the production worker and services, starts a uniquely named
stack, binds published ports to random loopback ports, and builds the UI against
that stack's gateway. Firefox resolves `*.localhost` to loopback, so live and
preview use their actual distinct hosts through the production nginx edge.
An existing administrator-installed worker AppArmor profile can be selected with
`PAGEWRIGHT_WORKER_APPARMOR_PROFILE`; no other application environment is inherited.
The private `.env` is not loaded. Production volumes and running stacks are not
used. Run against a local Docker daemon, not a remote Docker context.

## What the suite proves

The browser registers, creates a starter site, submits two sequential source
edits, reloads while a job is active and after completion, activates the first
preview before publishing (live remains 404), enables the initially disabled
site through the dashboard once routing exists, and publishes via version actions.
Both Chat and Dashboard must expose the correct live/preview destination links.
The second source edit must retain the first heading. Merely building does not
change either hosted version; preview activation does not change live. A third
request damages source configuration through the same sandboxed editor and must
fail without creating a completed version or changing either hosted page. The
browser publishes version two and rolls live back to version one, preserving the
second preview, then reloads and checks durable history and dashboard pointers.
Hosted pages and emitted theme assets are fetched from the real nginx hosts.

There are no intercepted application APIs, injected login tokens, direct SQL
writes, precompiled HTML uploads, manually dispatched workers or manually
reconciled jobs. Gateway background recovery and UI polling observe real jobs.

## Deterministic provider boundary

Only model responses are replaced. A read-only, non-root local HTTP fixture
answers gateway clarity/instruction requests and emits Responses SSE function
calls to the installed production CLI. The actual `exec_command` tool writes
source inside the unchanged worker sandbox. Compiler, artifact validation,
storage, manager callbacks, gateway reconciliation and deployment remain real.
The fixture accepts only known scenario markers and dummy credentials, has no
upstream forwarding, and shares an internal Docker network with the workers.
It requires a successful matching tool result before returning a final summary.

The SSE framing was checked using the OpenAI Docs skill against the official
[streaming Responses guide](https://developers.openai.com/api/docs/guides/streaming-responses),
and follows the repository's installed-CLI acceptance fixture. This is a narrow
test double, not a general provider emulator. It proves workflow integration, not
real-model quality or provider availability; paid-provider acceptance remains the
separate, explicitly authorized M2.12 suite. This run costs $0 in provider credit.

## Evidence and cleanup

The runner prints its unique project name and evidence directory under `/tmp`.
Playwright traces and hosted-page/dashboard screenshots remain there for review;
they contain synthetic account/session data and should not be publicly shared.
On ordinary success or failure, the runner stops dispatch, removes only workers
labelled with its unique network, and removes its Compose containers and volumes.
Deleted test data is disposable and cannot be recovered from those volumes.
Docker build caches/images are retained. Abrupt process/host termination can
interrupt cleanup; use the printed project/network to inspect and remove only
that test run's resources. Never run broad Docker pruning as test cleanup.

This desktop Firefox journey complements M3.10's mocked recovery regressions and
M3.11's responsive/keyboard audit. It does not certify other browsers, real-device
input, production DNS/TLS, remote deployment or model correctness.

## Verification recorded on 2026-09-06

The full journey passed in two fresh stacks, followed by a passing extended run
including dashboard Enable and actual View Live navigation. The final trace and
screenshots are in `/tmp/pagewright-browser-r2EDsz`; these local evidence files
are not committed. Tested with cached Playwright 1.58.2 and Firefox 146.0.1
(revision 1509), Node 24.11.1 and the production pinned CLI 0.153.4.

Provider-fixture tests (four test cases, including execution of fixed
source-edit programs against an in-memory filesystem), UI contract tests,
zero-warning lint, type checking/production build, existing rendered draft
recovery regression, JavaScript syntax and whitespace checks passed. Screenshots
of real compiled output and rollback dashboard state were reviewed. Initial
harness failures corrected timeout/lock configuration, internal-network host
ports, starter-source expectations and browser cache revalidation. No production
code, schema or sandbox policy changed; full Go suites were not rerun for this
test-only addition. The suite is local acceptance, not a hosted CI result.
