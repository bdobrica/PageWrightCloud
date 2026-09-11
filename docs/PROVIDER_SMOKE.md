# Explicit real-provider smoke (M2.12)

The harness builds a uniquely tagged production worker from the current checkout
for each invocation (updated for M4.13; historical M2.12 results below retain their
original image identity). This is a manually authorized test,
never a dependency of normal tests or CI. It submits one synthetic job directly
to a disposable production manager, uses the actual sandboxed CLI and trusted
compiler, stores fenced results in production storage, and verifies the requested
heading in both source and `public/index.html`. It does not publish a site or test
the browser/gateway conversation journey.

## Run safely

Use a Linux Docker host that passes the [worker isolation checks](WORKER_ISOLATION.md).
The manager needs its existing Docker socket authority; workers never receive it.
For AppArmor hosts, install the reviewed profile as documented and export
`PAGEWRIGHT_WORKER_APPARMOR_PROFILE=pagewright-worker-proc`. Do not use privileged,
unconfined or added-capability workarounds.

Test without a key first; the harness builds its own worker image:

```sh
make test-provider-budget
make smoke-provider PROVIDER_SMOKE_ARGS='--offline'
```

The offline mode uses a deterministic provider response but executes a real CLI
tool command, compiles and verifies HTML. Its usage numbers are synthetic, not
charges. The real test reads exactly one key variable from the root `.env` as
data; it never sources the file, prints values or forwards unrelated variables.
The default key name is `PAGEWRIGHT_LLM_KEY`; override it with `--key-env=NAME`.

After checking current pricing and obtaining authorization for the **total across
all attempts**, invoke explicitly (replace the date with today's UTC date):

```sh
make smoke-provider PROVIDER_SMOKE_ARGS='--execute --authorize-usd=1.62 --price-verified-on=YYYY-MM-DD --key-env=PAGEWRIGHT_LLM_KEY'
```

The harness creates uniquely named services/networks and an owner-only temporary
key file. A non-root budget gateway alone reads that file. The worker receives a
random run-local proxy token, not the OpenAI key, and uses an internal Docker
network without external routing. Only the gateway has the real credential and
forwards generation to the fixed `https://api.openai.com/v1/responses` endpoint.
Manager and storage use an independent run-local service credential. Host fixture
requests send it only to those resolved service origins; the worker receives a
scoped job token, never the service signing secret. Remote Docker contexts are
refused before loading credentials or creating resources.
Loopback-only control ports allow the host harness to reach manager/storage; they
do not change the worker's network, namespace or tool restrictions.

## Budget policy

Pricing rechecked on 2026-09-11: GPT-5.6 Luna input $0.20/million tokens, cached input
$0.02/million, output $1.20/million; documented context window 1,050,000 tokens.
Inputs above 272,000 tokens have 2x input and 1.5x output pricing; cache writes
cost 1.25x uncached input. See
[official model pricing](https://developers.openai.com/api/docs/models/gpt-5.6-luna).
The guard accepts only `gpt-5.6-luna` and validates the returned model ID exactly.
There is no automatic model fallback.

Each request reserves the full 1,050,000-token context at the long-context
cache-write price ($0.50/million), plus 8,192 output tokens at the long-context
price ($1.80/million): **$0.5397456**. Reserving even more than the model's
documented maximum input is intentional. At most three requests can be forwarded:
**$1.6192368 maximum reserved model-token cost per invocation**. Requests force
`max_output_tokens=8192`, default processing tier, low reasoning effort, streaming
and `store=false`. Reasoning tokens are included in output usage. Only local
function/custom/shell tool definitions are accepted; hosted tools, media/file
inputs, background work and server-managed histories are rejected. CLI-only
attribution metadata is stripped.

Reservations use integer nanodollars and are synced to a private journal before
network submission. There are no refunds and no automatic upstream retries.
Concurrent requests, a fourth request, oversized bodies, redirects and unexpected
endpoints fail closed. A non-200 response, timeout, malformed stream, incomplete
completion or unexpected usage/model/tier permanently blocks the run and retains
its reservation. The gateway buffers a bounded stream and verifies terminal usage
before returning it to the CLI. The CLI can request another turn/retry only within
the same counter; it cannot reset or bypass the guard.

Journal creation is exclusive: restarting the gateway against an existing state
directory fails instead of resetting the counter. A new explicit harness invocation
has a **new** budget. The operator must include previous reservations in their
total authorization, especially after uncertain responses. Do not rerun blindly.
The printed token-cost upper bound is calculated from API usage, not an invoice.
If the API omits cache-write counts, all non-cached input is priced as cache writes
rather than assuming a cheaper charge. Missing/invalid aggregate usage still blocks.
Account taxes, unrelated requests, changed prices and account-specific charges
are outside this model-token envelope. Recheck prices before later runs. The
same-day CLI acknowledgement records operator review, not automatic price discovery.

This policy follows the reserve-before-submit and retain-on-uncertainty principles
in [OpenAI's per-run spending guidance](https://developers.openai.com/cookbook/articles/per_run_spending_controller_responses_api).
Unlike that example, it reserves the full model context and never refunds; it
supports this pinned CLI's bounded SSE transport rather than arbitrary requests.
This smoke-only guard is not a production multi-user billing or quota system.

## Evidence, cleanup and remaining scope

The harness prints its private temporary evidence directory. Successful runs retain
`acceptance.json`, `budget-summary.json`, the append-only budget journal and the
synthetic bootstrap fixture. Records include job/site/version IDs, image ID, model,
token counts, cost/reservations, HTML hash and verified heading. No API key or raw
provider request/response is retained. Known provider error categories may be
recorded, but not error prose. Offline-only diagnostics can list rejected field names.

Finally, the gateway is stopped first, the exact generated worker is ownership-
checked and removed, disposable services/networks are removed, and the temporary
key file is unlinked. Application volumes and the user's `.env` remain untouched.
Ordinary SIGINT/SIGTERM triggers spending shutdown and cleanup; SIGKILL, host loss
or Docker failure can interrupt this. In that case use the recorded project/job
identity, stop its budget container first, preserve its journal, and remove only
that run's temporary key/resources. Never recreate its gateway to reset the budget.

Offline tests cover request/tool restrictions, durable reservations, concurrency,
body bounds, usage accounting, credential redaction, permanent uncertain-charge
blocking and restart refusal. Existing package/race/service/installed-CLI/compiler
and Docker tests remain required regressions. M3 owns browser history/publishing;
M4 owns internal-service authorization and pilot hardening.

## M4.13 candidate acceptance — 2026-09-11

One explicitly authorized $2-total invocation on revision `02bb2c6` passed with
the current-checkout worker and current service/scoped-worker authentication.
The seven spending/preflight tests also passed. Command:

```sh
node scripts/provider-smoke.mjs --execute --authorize-usd=2 --price-verified-on=2026-09-11
```

| Request | Input | Cached input | Cache writes | Output | Cost upper bound USD |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 7,069 | 0 | 7,066 | 106 | 0.00189430 |
| 2 | 7,203 | 7,066 | 134 | 58 | 0.00024502 |
| 3 | 7,285 | 7,200 | 82 | 90 | 0.00027310 |
| Total | 21,557 | 14,266 | 7,282 | 254 | **0.00241242** |

The usage-derived amount is not an invoice. Conservative reservations remained
**$1.6192368**, below the authorized total, with no retries/new invocation and no
guard block. Evidence is retained in `/tmp/pagewright-provider-smoke-6pt8lu`:

- Job: `ca76d708-e592-474d-8a21-8c79f057b4e6`
- Site: `e269eb36-3f91-41d1-a546-28958def4945`
- Version: `b28c1c0c-3c1e-4402-a2c5-b5455ec2cbb5`
- Worker image ID: `sha256:caed14a99a18e406b60c89796f24a73a70168f1ce24dba9afc0b904245c37f46`
- HTML SHA-256: `25b7f4b86e3f0449d4947350af1936c3e3d13e6ce8a1d16441a3197df94fd685`
- Observation: `2026-09-11T19:23:28.194Z`

The real worker changed only `content/home/index.md`; compiler 0.1.0 and static
checks passed, source and HTML contained `PageWright provider smoke verified`,
and initial source bytes were unchanged. Browser checks were explicitly unperformed
by this harness (the separate M4.13 Chromium journey passed). Non-root, read-only,
capability-dropped, non-privileged internal-network isolation and scoped credentials
passed. Cleanup exited 0; independent exact-project/job queries found no containers
or networks, and the temporary key file was absent. Evidence and image caches were
retained; application data and remote pilot were untouched.

## Acceptance record — 2026-09-06

User authorized $2 total and selected `gpt-5.6-luna`. The successful real run
used three requests and returned complete usage, including cache-write counts:

| Request | Input | Cached input | Cache writes | Output | Calculated USD |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 7,068 | 0 | 7,065 | 110 | 0.00189885 |
| 2 | 7,206 | 7,065 | 138 | 68 | 0.00025800 |
| 3 | 7,298 | 7,203 | 92 | 31 | 0.00020486 |
| Total | 21,572 | 14,268 | 7,295 | 209 | **0.00236171** |

This is a usage-derived model-token amount, not an account invoice. Two earlier
`gpt-5.1-codex-mini` attempts each stopped after one HTTP 404; the second recorded
the allowlisted `model_not_found` category. The model-list endpoint nevertheless
listed that model. No usage was returned, so both $0.116384 reservations remain
counted conservatively. No GPT-5.4 Mini paid request was sent. Total maximum
reservations across the two failures and successful Luna run were **$1.8520048**,
below the $2 authorization. No additional paid calls were needed.

Successful evidence:

- Job: `9c7175b3-c92d-4bdb-97eb-59a6137131ca`
- Site: `e559fc67-ba43-437e-91a6-18564e098330`
- Version: `d677cb8a-3b97-472b-8133-b3427dc13e28`
- Worker image ID: `sha256:ded142ac004ea95211a30b69241f371058539191bbaaec33886467cf6d5e4b91`
- Verified heading: `PageWright provider smoke verified`
- HTML SHA-256: `25b7f4b86e3f0449d4947350af1936c3e3d13e6ce8a1d16441a3197df94fd685`
- Observation: `2026-09-06T15:54:49.577Z`

The manifest reported successful static checks, compiler 0.1.0, only
`content/home/index.md` changed, and browser checks unperformed. Source and HTML
contained the requested new heading; original source bytes were unchanged.
Docker inspection confirmed non-root execution, read-only root, all capabilities
dropped, internal worker network, no privileged mode and no container restart.
Production worker/manager/storage and actual fenced callbacks were used, not
the integration-only executor.

The successful local evidence directory is `/tmp/pagewright-provider-smoke-6jQbWG`;
it is temporary and may be removed by the host. This document preserves the
non-secret acceptance summary. Disposable services, worker, networks and temporary
key files were removed. The user's `.env` and application data were not changed.
