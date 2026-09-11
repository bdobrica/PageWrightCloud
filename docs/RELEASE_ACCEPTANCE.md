# M4.13 release acceptance record

Run date: 2026-09-11. Starting revision: `f1d9175`; the changes accompanying this
record close acceptance-harness gaps and a mutable-hosting cache defect. Local
deterministic acceptance passed; a fresh paid-provider smoke remains pending.
This is a local release-candidate record, not approval to admit public testers.

## Scenario matrix

Numbers correspond to [PLAN.md release scenarios](../PLAN.md#release-acceptance-scenarios).
Results distinguish real services from deterministic model responses and rendered
tests with mocked APIs. A previous paid run is not a fresh candidate run.

| Scenario | Evidence and remaining boundary |
| --- | --- |
| 1. Fresh startup, provisioning, login, domain validation | Disposable startup/recreation and final Chromium closed-signup/operator-provisioned UI login passed. Gateway tests cover duplicate/invalid names. |
| 2. Job lifecycle and changed compiled content | Five-service race integration passed. Firefox completed two actual CLI/compiler jobs with the deterministic provider. Updated offline provider smoke passed; fresh paid run awaits authorization. |
| 3. Preview before live, pages/assets, publication | Final normal-cache Chromium and real-edge integration passed after correcting stale preview caching. Public existing pilot app/API/live/preview HTTPS returned 200. |
| 4. Sequential unpublished edits, pointer independence, rollback | Compiled round-trip integration and Firefox journey passed these checks. Chromium's initial cache-related failure is retained, not silently rerun away. |
| 5. Refresh, expiry/re-auth, retry | Actual browser journey refreshed active/completed builds; focused rendered tests passed expiry, retained input, owner isolation, retry identity, clarification and failure recovery with a mocked gateway. Race integration separately proves durable idempotency/state recovery. |
| 6. Worker failures and manager/Redis restart | Actual runner subprocess SIGTERM/SIGKILL/deadline/upload/callback matrix passed; Redis recovery/fencing and Docker cleanup tests passed. Disposable startup SIGKILL/recreation passed. These are layered tests, not a paid-provider crash run or multi-host HA. |
| 7. Failed deploy/config, active pins, immutable versions | Race integration covered real Nginx validation/acknowledgement/rollback, failed artifact/activation, retention/pins, immutable writes and unchanged live bytes. Cache regression added to real edge HTTP checks. |
| 8. Two-user and internal/security boundaries | Final Chromium two-account operation matrix, generated-page CSP violation, origin headers and internal/scoped-worker access probes passed. Integration covered traversal/symlinks and malformed/oversized content. Public API denied live/preview/foreign origins (403), allowed the exact app origin (200). Firefox generated-page origin phase timed out; its failure remains separate. |
| 9. Delivered reset link and restore | New TLS/STARTTLS SMTP-to-real-database reset test passed: link extracted from received message, one successful reset, reuse 400, old login 401/new login 200. Backup drill destroyed source volumes and recovered ownership, history and live/preview content in fresh volumes. No external mail provider or real inbox delivery is claimed. |

## Commands and observations

Local tools: Go 1.24.10, Node 24.11.1, Docker Engine 29.0.1, Compose 2.40.3.
Docker used the local Unix socket, with seccomp and cgroup namespaces; no AppArmor
profile was needed on this local host. No worker restrictions were relaxed.

Passed before the final cache-fix reruns:

```sh
make test-all
make test-race-state
make test-integration
npm --prefix pagewright/ui run test:contracts
npm --prefix pagewright/ui run lint -- --max-warnings=0
npm --prefix pagewright/ui run build
make test-compiler-smoke
make smoke-stack
make test-backup-restore
make test-docker-spawner
make test-worker-cli
make test-worker-compiler
make test-provider-budget
node --test scripts/browser-provider.test.mjs
node scripts/provider-smoke.mjs --offline
```

The package baseline covers six modules; local state races repeat three times.
The integration suites use `-race` in test binaries, not in separate service images.
The delivered-email test was added after the first integration run, then passed
in a fresh complete run (`pagewright-test-1789152835-338963`). UI contracts: 48 passed.
Provider budget/preflight tests: 7 passed; browser-provider fixture tests: 4 passed.
Focused gateway mail vet and JavaScript syntax checks also passed.

Rendered draft/session/reset and five-viewport keyboard/reflow audits passed via:

```sh
node pagewright/ui/test/draftBrowser.mjs PLAYWRIGHT_MODULE FIREFOX_EXECUTABLE
node pagewright/ui/test/draftBrowser.mjs PLAYWRIGHT_MODULE FIREFOX_EXECUTABLE --accessibility
```

Playwright 1.58.2 used Firefox build 1509 (146.0). Matching Chromium headless build
1208 (145.0.7632.6) was installed in the local browser cache for an independent run.
It was not added to production images or project dependencies.

### Browser failures and cache correction

Firefox command:

```sh
node scripts/browser-acceptance.mjs PLAYWRIGHT_MODULE FIREFOX_EXECUTABLE
```

Evidence `/tmp/pagewright-browser-A9QvE1`: two successful jobs, expected third-job
failure, preview/live/rollback checks and two-account isolation passed. The run
failed at `originBrowser.mjs` navigating the generated page while waiting for
`commit`; internal-access and closed-pilot phases after it were not reached.
A tiny local page with isolation headers also produced an inconsistent navigation
timeout despite a 200 response and navigation event. This is an unresolved Firefox
runtime/harness limitation, not a full-browser pass or proof of its precise cause.

Initial Chromium evidence `/tmp/pagewright-browser-VD8wkg`: failed after activating
the second preview because the browser reused first-version HTML. Trace responses
contained old ETag/Last-Modified validators, repeated first-version content, and no
explicit cache policy. Firefox's harness cache-disable preferences had hidden this
problem. Neither increasing timeouts nor disabling Chromium caching is the fix.

The generated-content edge now applies `Cache-Control: no-store` to pages, assets
and errors, hides upstream cache validators/policy and strips conditional validators
on upstream requests. Live/preview paths are mutable selection URLs, not immutable
content-addressed assets. This deliberately trades bandwidth/cache efficiency for
correct publication and rollback. Existing browser entries created before this
policy may require a hard refresh until their prior freshness expires.

Final post-fix Chromium run passed all phases with normal browser caching:

```sh
node scripts/browser-acceptance.mjs \
  /home/bogdan/.npm/_npx/e41f203b7505f1fb/node_modules/playwright/index.mjs \
  /home/bogdan/.cache/ms-playwright/chromium_headless_shell-1208/chrome-headless-shell-linux64/chrome-headless-shell chromium
make test-integration
```

Evidence `/tmp/pagewright-browser-FrDJQX`: first version
`578720e7-7df6-4355-b751-ddb442b5cc0d`, second version
`062df7e3-00c8-49e6-a8f7-9a71d8831fac`, expected failed third version
`46b3bd62-ae1b-456b-a3d5-afb212ed2c5e`. The browser verified source inheritance,
live/preview asset independence, rollback, failed-build preservation, two-account
ownership, actual generated-page `connect-src` enforcement, direct internal API/
Redis rejection, scoped worker misuse rejection, closed signup, operator-provisioned
UI login and zero-AI allowance rejection. Final integration passed all five
race-enabled suites, delivered-email recovery, conditional-hosting requests and
dependency outage/recovery probes. Both generated stacks were removed successfully.

Regression checks require no-store/no exposed validators and current content in
the browser, plus conditional-request checks through the actual edge. Security
headers and worker isolation remain unchanged. Only a local candidate is updated;
the pilot needs a reviewed edge configuration deployment/reload before this policy
applies there. Do not overwrite unrelated host Nginx or blog configuration.

### Offline provider and backup evidence

Offline smoke: `/tmp/pagewright-provider-smoke-ZQJSsp/acceptance.json`, job
`1d0646c5-2223-4851-8eb7-0b88cd43a0dc`, worker image
`sha256:30a990edc20bc5bcf649b4f9e47e6660104971141ac16449fa8db15603a286ac`.
Two synthetic model responses produced the requested source/HTML heading and left
initial bytes unchanged. Its usage/cost fields are synthetic; actual provider
spend was zero. The harness now builds a unique current-checkout worker and uses
independent scoped service/provider credentials rather than a historical M2.12 tag.

Backup evidence: `/tmp/pagewright-backup-drill-30s_oxa1`, source
`pw-backup-src-b2578975d6c0`. Original volumes were removed before restore. The new
target retained all three versions, archive checksums, serving receipts, ownership,
spending/Redis reservations, live/preview bytes and monotonic deployment sequence;
a repeated restore into populated state was refused.

### Paid-provider gate

No new paid requests have been sent in this release run. A fresh run requires the
user's explicit total budget authorization. Prior M2.12 and pilot success remain
historical evidence only. Official GPT-5.6 Luna pricing/context assumptions were
rechecked on 2026-09-11 using OpenAI Docs and the
[official model page](https://developers.openai.com/api/docs/models/gpt-5.6-luna).
The unchanged guard reserves at most $1.6192368 per invocation (three requests),
retains reservations on uncertainty, and never automatically retries upstream.
Do not start a second invocation without accounting for the first reservation.
See [provider procedure](PROVIDER_SMOKE.md); real inbox delivery, public failure
recovery/full-browser gates and release rollout are separate operator work.

### Public read-only checkpoint

Existing `app.pagewright.io`, `api.pagewright.io`, `m49-check.pagewright.io` and
`m49-check.preview.pagewright.io` passed trusted HTTPS and TLS-readiness probes.
All returned HSTS `max-age=86400` and nosniff; app/generated pages retained their
separate CSPs and opener isolation. The API allowed the exact app origin and
denied generated/foreign origins. No login, deployment, certificate request, remote
configuration change, production failure injection or external email occurred.
These checks do not establish that the remote pilot runs this candidate revision.

## Remaining gates

Fresh paid smoke authorization/result remains pending; M4.13 is not marked complete.
The Firefox runtime/harness limitation remains documented despite the independent
Chromium pass. M4.9's remaining public
failure-recovery and full-browser acceptance is not closed by local tests. Retained
traces and fixture backups are local evidence and may contain synthetic credentials;
do not publish them indiscriminately. Test cleanup removes generated resources only;
image/browser caches and selected evidence remain available.
