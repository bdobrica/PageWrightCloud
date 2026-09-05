# PageWrightCloud MVP plan

Assessed: 2026-09-05. Execution checklist: [TODO.md](TODO.md).

## Outcome and scope

Deliver an invite-only MVP in which a non-technical user signs in, creates a site, requests a change in chat, sees a completed version, previews it, publishes it, and can restore an earlier version. A second edit must preserve the first edit. Failed builds must leave the published site intact.

Planning assumptions: one operator, one Docker Compose host, a small group of testers, one bundled `starter` theme, text-only chat, and platform-controlled subdomains. These are proposed scope choices, not existing capabilities. First establish the complete local workflow; admit remote testers only after the release gate below. Retain the Go services, React UI, compiler, PostgreSQL and Redis. A service rewrite is unnecessary for this milestone.

Defer arbitrary custom domains, Google OAuth, attachments, a theme marketplace, billing, teams, Kubernetes, alternative queues/storage providers, autoscaling, and extensive analytics. Hide or disable unfinished controls and their unsupported API operations in the MVP configuration. Email/password login remains the supported authentication path; remote testers need working account recovery.

## What the repository actually provides

There is useful implementation across all services, but the application is still a collection of partially connected components. The March README and review describe some UI and service features as complete that have not been connected end to end. Existing tests passing does not establish a working MVP.

| Area | Evidence in the current code | Consequence / required work |
| --- | --- | --- |
| Job submission | M1.1 aligns the [canonical job contract](docs/JOB_CONTRACT.md); M1.2 adds [durable submissions](docs/BUILD_SUBMISSIONS.md), independent IDs and retry-key deduplication across gateway/manager/UI. | Wire and pre-dispatch persistence gaps fixed and HTTP-tested; reliable dispatch/recovery and live status synchronization remain M2/M3. |
| Execution | [Docker spawner](pagewright/manager/internal/spawner/docker/docker.go) and Kubernetes spawner only log. [Worker Dockerfile](pagewright/worker/Dockerfile) installs a mock command. | An accepted job cannot execute the intended AI workflow. There are also two worker implementations/images; select `pagewright/worker` as the MVP runner. |
| First site | [CreateSite](pagewright/gateway/internal/handlers/sites.go) inserts a DB row only. [Build](pagewright/gateway/internal/handlers/build.go) falls back to `initial`; the worker always downloads a source artifact. UI selects `template-1`, while the supplied theme is `starter`. | Bootstrap a valid, versioned source and map the supported template explicitly. |
| Artifact transport | M1.3 (`0fa1044`) aligns raw gzip transport; M1.4 (`73d3635`) adds manifest-last completion. M1.5 (`0cbeaa5`) makes files write-once with retry/conflict semantics and disables version deletion in UI/API. | Storage is still opaque, not an archive-validation, authorization or publishing gate. Retention and future coordinated deletion remain separate work. |
| Compilation | [Worker runner](pagewright/worker/cmd/runner/main.go) edits and repacks source without invoking the compiler; `ChecksPassed` is hard-coded. [Serving](pagewright/serving/internal/artifact/manager.go) requires an archive containing `public/`. | Integrate `pagewrightc`, generate `public/index.html`, and derive validation results from actual checks. |
| Version state | M1.2 commits the job mapping and version using `target_version` before dispatch, with atomic outcome/status writes. [Version listing](pagewright/gateway/internal/handlers/versions.go) still returns storage records that differ from UI types. | Submission mapping fixed; reconcile later completion and normalize version listing in M1.9/M3.1. |
| Live updates | [UI socket](pagewright/ui/src/hooks/useWebSocket.ts) sends a query token; [auth middleware](pagewright/gateway/internal/middleware/auth.go) accepts only a bearer header. [Hub](pagewright/gateway/internal/websocket/hub.go) has no caller publishing build results and no implemented ownership filter. M1.1 aligns status vocabulary to `pending/running/completed/failed` and validates UI payloads. | Schema mismatch fixed; delivery and authorization remain broken. Implement owner-checked retrieval/polling and repair WebSockets before enabling them. |
| Deploy / preview | [Gateway serving client](pagewright/gateway/internal/clients/serving.go) sends `version_id`; [serving types](pagewright/serving/internal/types/types.go) require `version`. Preview UI only opens a URL and does not activate a preview. | Repair the payload, preview action, returned URLs, and preview assets/navigation. |
| Deployment consistency | [DB update](pagewright/gateway/internal/database/sites.go) sets both live and preview IDs, clearing one when the other changes. Serving removes the old symlink before creating the new one. | Preserve the other pointer, handle DB failures, and replace symlinks using an atomic rename. |
| Hosting | [Compose](docker-compose.yaml) separates serving and nginx, but serving executes local `nginx -s reload`. Preview activation does not create nginx config. | Make nginx configuration/reload part of a supported topology; first preview must work before first publish. |
| Job reliability | Manager queues and immediately spawns inside the HTTP handler; no queue consumer runs in server main. Lock renewal and worker timeout configuration are not wired into the lifecycle. Redis has no persistent volume in root Compose. | Add durable dispatch, bounded concurrency, lock renewal, timeout handling and restart recovery. |
| User-facing gaps | File uploads are multipart in the UI but build handler decodes JSON. Reset email is a TODO and reset tokens are logged. Site links assume HTTPS without the local port. | Ship only working controls, configure returned hosting URLs, and complete recovery before remote access. |
| Boundaries | Internal write APIs have no authentication and their ports are published. FQDNs reach filesystem/nginx paths without adequate validation. Wildcard HTTP/socket origins remain. | Enforce service authorization, validate identifiers, restrict exposure, and isolate worker credentials and generated content. |

The compiler is a useful trusted build component, but its presence and the worker's instruction file do not themselves enforce an execution security boundary. The worker currently runs a command with its inherited environment and has no integrated trusted-output validation.

## Initial assessment baseline (before M0 implementation)

Historical results from the assessment follow. M0 progress below records subsequent
fixes and passing verification; these initial failures are not the current status.

- `go test ./...` in gateway, manager, storage, worker, serving and compiler, using `GOCACHE=/tmp/pagewright-review-go-cache GOPROXY=off`: all passed. Compiler reports no test files; several worker and serving tests explicitly skip cases. These runs do not include integration-tagged tests or the race detector.
- Compiler sample: `go run ./cmd/pagewrightc build --theme ../themes/starter --content ./test-site/content --out /tmp/pagewright-review-compiler-output` from `pagewright/compiler`: passed, producing home, about and contact pages and assets.
- UI `npm run build`: passed using installed dependencies, with an unexpected `}` CSS minification warning. This is not a clean dependency-install verification.
- UI `npm run lint`: failed with 17 errors and 1 warning, covering explicit `any` types, auth context effects/exports, socket callback initialization and a hook dependency.
- `docker compose config --quiet`: valid, with missing Google and LLM environment warnings. `docker compose ps --format json` returned no project containers. The application stack and a paid AI request were not started.

No full browser journey or service integration run was performed. The existing `docker-verify-local-domain-strict` target writes placeholder HTML and manually reloads nginx; retain it as a hosting diagnostic, not the MVP acceptance test. Root integration targets also omit manager coverage and do not provision all prerequisites required by the service tests.

## Implementation decisions

1. **Keep identifiers explicit.** `job_id` identifies an execution; `target_version` identifies its immutable artifact. Persist their association with site and owner before dispatch. Use `pending → running → completed | failed` consistently, with bounded retries and idempotent terminal updates. Reject stale worker results using job identity and fencing/attempt information.
2. **Keep editable source with each version.** Adopt an archive containing `content/`, generated `public/`, and a manifest recording source version, theme version, compiler version and checks. Execution logs are private metadata. Publish only `public/`; never expose source, prompts, instruction files or secrets through nginx. Render from a trusted bundled theme outside the AI-writable directory. Preserve the same artifact when promoting preview to live.
3. **Define the editing base.** Default the next edit to the latest completed draft, then the live version, then the bootstrapped source. Surface the selected base in the UI. A failed job must not become the next base; this avoids losing consecutive unpublished edits.
4. **Start with reliable polling.** Gateway exposes owner-checked job status and persisted history; UI polls while a build is active and recovers after refresh. Disable the broken socket path in the MVP until it supports browser authentication, owner filtering, stable reconnection and the same event schema. WebSockets are optional, durable state is required.
5. **Use one local deployment path.** Prefer a named volume for the existing filesystem storage backend on a single host; the privileged NFS server is unnecessary for this topology. Give the selected worker image an explicit tag that matches manager configuration. Define the Docker network, credentials and resource limits used by spawned jobs. The manager's Docker authority must remain inaccessible to worker processes.
6. **Make serving own its nginx lifecycle.** For the MVP, run the serving controller and nginx together under a documented supervisor, with config validation and controlled reload. This removes the current cross-container reload gap. Return explicit live and preview URLs from the gateway, including local scheme/port. Use a preview hostname/origin strategy that supports promoting the same artifact; test links and assets under both hosts. Keep generated-site origins separate from the application origin, especially if cookies are introduced.
7. **Bound AI execution.** Pin and verify a real supported non-interactive CLI at implementation time; verify authentication, instruction discovery, filesystem restrictions, timeout and cancellation with that installed version. Use a deterministic fake executor only in tests. Constrain runtime, concurrency, output size and per-user build usage before allowing paid requests from testers.

## Milestones and exit criteria

Estimates are planning ranges for one developer familiarizing themselves with the code, excluding DNS/email provisioning delays. Re-estimate after M1. Each milestone needs its own verification; avoid a final week of integration fixes.

### M0 — Reproducible baseline (1–2 days)

Progress (2026-09-05): M0.1 completed in `8f404df`. Go builders use 1.24.10,
UI builder / `.nvmrc` use Node 24.11.1; local and container clean installs and
all seven image builds passed. See [development instructions](docs/DEVELOPMENT.md).
The install audit reported 19 dependency advisories (2 low, 3 moderate, 14 high);
dependency remediation remains required before the remote pilot. The CSS warning
is tracked in M0.2. These build checks do not verify AI execution.

M0.2 completed in `b32037f`: typed API errors, validated synchronous saved-session
restoration, separated context exports, stable socket callbacks/cleanup and alias
loading, and the stray CSS brace are fixed. Local and clean-container lint with
zero allowed warnings and production builds passed. Browser authentication and
job delivery remain later milestone work; review also identified the chat route
parameter mismatch, now tracked in M1.9.

M0.3 completed in `828d8cf`: `make test-all` needs no infrastructure;
`make test-integration` builds a separate ephemeral Compose project with no
published ports or application volumes. Gateway tests use private schemas and
database-dependent CLI checks are integration-tagged instead of silently skipped.
Both commands passed; all five CLI DB checks executed. Test containers, network
and ephemeral data were removed after each run; cached images remain available.

M0.4 completed in `946554c`: gateway and tests use the same embedded SQL files.
All pending DDL/version records commit under one transaction and advisory lock.
Migration 006 reconciles legacy schemas without deleting data. PostgreSQL tests
passed for empty/repeated startup, tracked/manual upgrades, rollback/retry,
invalid legacy rows, concurrent starts and newer-version rejection. The full
integration harness and rebuilt gateway image also passed.

M0.5 completed in `eb6e0ec`: root Compose uses its existing artifact volume
directly, with no privileged NFS daemon; host-port overrides preserve internal
ports and service health checks. Names are project-scoped. `make smoke-stack`
passed fresh startup and container recreation with persisted user/site/artifact
data, served UI HTML/JavaScript, theme registry and nginx config validation.
The smoke test exposed and fixed the themes IPv6-localhost health probe mismatch.
Its temporary project and volumes were removed; existing app volumes were untouched.

M0.6 completed in `1cc5cd9`: CI defines Go/compiler, UI, isolated integration,
and image/startup/recreation jobs. The compiler fixture and actionlint passed;
the underlying checks have local verification above. Five remaining explicit
worker/serving skips and coverage limits are inventoried in [testing instructions](docs/TESTING.md).
Independent workflow review found no blocking issue. Hosted CI has not run:
the workflow will execute on a future push or pull request; no push was requested.

Scope: resolve the UI lint/CSS baseline, align toolchains, standardize checks,
make startup reproducible, consolidate migrations, and correct startup/status docs.

**Exit:** a fresh checkout can build the UI and all selected service images, initialize the DB once, restart safely, and run documented checks. CI starts running these checks.

M0.7 completed in `fca46ca`: README now documents the actual root-stack startup,
ports, preserved volumes, migration precautions and local-only security limits.
It separates seeded hosting diagnostics, startup smoke checks and the future MVP
journey. Independent review, local documentation links, Make dry-runs and both
Compose configurations passed. Final Go package tests and zero-warning UI
lint/production build passed again.

**M0 handoff:** all seven implementation items are committed and locally verified.
Hosted CI execution is the only unobserved part of the exit criterion; the branch
has not been pushed. At M0 handoff the next item was M1.1; see its progress below.
The application is not yet an end-to-end MVP, and no paid AI request was made.

### M1 — Contracts, bootstrap and artifact round trip (3–5 days)

M1.1 completed (2026-09-05) in `ef544f9`. The [job contract](docs/JOB_CONTRACT.md)
defines required identities, prompt/source/target fields, canonical statuses,
terminal results and manager error envelopes. Gateway derives `owner_id` from
the authenticated site's owner and returns the manager's accepted identity;
manager rejects malformed/obsolete payloads and mismatched callbacks without
mutating the job or releasing its lock. Worker launch validation and actual
callback payloads align, and UI API/socket paths validate the same schemas.
The worker's unanchored ignore rule had hidden `cmd/runner/`; it is corrected
and the entrypoint and tests are now tracked. Selected build sources were audited
for additional ignored entrypoints; none were found.

Verification: the six-module package baseline and gateway/manager/worker race
tests passed. The four-module isolated integration harness passed twice, with
the final run covering authenticated gateway → PostgreSQL/manager acceptance,
owner rejection, clarification/follow-up, lock conflict and failure retrieval,
plus actual worker callbacks for completed/failed outcomes. Only provider HTTP
responses are faked; no paid provider or executor ran. Eleven UI contract tests,
zero-warning lint/build, affected gateway/manager/worker/UI images, actionlint,
Compose configuration and documentation link checks passed. Independent review
identified worker snapshot and UI timestamp validation drift; both were fixed
and regression-tested before committing. CI includes UI contract tests and the
expanded integration harness; hosted execution still awaits a push.

M1.2 completed (2026-09-05) in `d86f737`. Migration 007 creates durable submissions
and their pending versions in one transaction, preserving legacy rows without
inventing mappings. Gateway allocates independent execution/artifact IDs and
requires an owner/site-scoped `Idempotency-Key`; retries reuse the committed
prompt/source/IDs before calling the provider. An atomic dispatch claim permits
one POST. Manager atomically reserves explicit job IDs once in Redis, rejects
conflicting reuse, remembers definite rejections and never respawns a reservation
on replay. Worker metadata merges do not overwrite fast terminal callbacks.
The UI reuses uncertain retry keys and synchronously blocks duplicate sends.

Failure handling is explicit: reservation failure sends no manager request;
definite rejection atomically records a failed version; lost responses or failed
outcome writes retain the mapping and reconcile by GET. Manager redirects are
disabled so a later redirect dial failure cannot be mistaken for nondelivery.
See [submission state and retry rules](docs/BUILD_SUBMISSIONS.md).

Verification passed: six-module package baseline; gateway/manager race tests;
two full isolated PostgreSQL/Redis/API integration runs; 16 UI contract/retry
tests, zero-warning lint and production build; affected image builds; fresh-stack
startup and data-preserving recreation with migration 007. Tests prove mapping
visibility before manager POST, transactional rollback, concurrent deduplication,
upgrade/reconnect, site deletion preservation, lost response, rejection/outcome
write failure and conservative behavior when manager evidence is missing.
Independent reviews covered persistence/orchestration and documented limits.
Synthetic test projects/data were removed; no application volumes were reset.

M1.2 does not
provide automatic recovery for crashes between claim/send or lost manager state:
those remain explicitly uncertain rather than being blindly redispatched. New
Redis reservations have no TTL but root Redis is still nonpersistent; durable
recovery and bounded retention remain M2.9. Real spawners must distinguish definite
and ambiguous start failures. Status is a saved submission snapshot, not live
callback synchronization; that remains M3.1. Bootstrap, real execution, callback
authentication/fencing, polling and publishing remain unfinished. No paid AI
request ran and no push/hosted CI execution was performed. Full M1 exit is unverified.

M1.3 completed (2026-09-05) in `0fa1044`. All three artifact clients use
`/sites/{site_id}/artifacts/{build_id}` with validated identifier segments and
redirects disabled. Worker sends the open gzip file directly; storage rejects
multipart and unsupported content encodings before writes. Downloads explicitly
request identity encoding and validate the gzip media type. Gateway streams its
authenticated response; worker/serving stage unique sibling temporary files and
preserve existing destinations on failed transfers. Storage/gateway abort broken
streams, packing checks finalization errors, and both extractors validate the gzip
trailer after tar EOF. See [transport contract and limits](docs/ARTIFACT_TRANSPORT.md).

Verification passed: six-module package baseline; worker/serving and targeted
storage race tests; two full five-module isolated integration runs; affected
gateway/storage/worker/serving images; fresh startup and persistent-volume
recreation; shell/JavaScript syntax and staged whitespace checks. The integration
harness first packs/uploads a shared text/binary fixture with the worker, then
gateway and serving fetch the same stored object. All three clients preserve
exact compressed bytes, and extraction matches the fixture file set. Gateway's
authenticated download and interrupted upstream response are covered. Independent
review found repeated Content-Encoding header handling differed across clients;
it was aligned and regression-tested in the final integration run. Synthetic test
projects/data were removed; application volumes were untouched. Five known test
skips remain unchanged; hosted CI still awaits a push.

M1.3 does not repair existing malformed artifacts or prove immutable
writes, archive safety, nginx activation or an AI build/publish workflow. Those
remaining gates keep the overall M1 exit unverified.

M1.4 completed (2026-09-05) in `73d3635`. Canonical per-version `/logs` and
`/manifest` POST/GET endpoints preserve JSON metadata outside archives with
restricted file permissions and no-store responses. The manifest is the final
commit record and requires the artifact and private log first. Metadata writes
check write/sync/close before rename; artifact close failures also propagate.
The worker uploads artifact → log → manifest and attempts a completed manager
callback only after all three succeed. Its reported manifest path is canonical.
Storage listings contain one completed entry per valid manifest, newest first,
without prompt/output content. Legacy event records no longer create entries;
existing files are preserved and raw artifact retrieval remains available.
See [metadata contract and compatibility boundaries](docs/VERSION_METADATA.md).

Verification passed: six-module package baseline; storage/worker race tests,
including final targeted reruns; two five-module isolated integration runs;
worker/storage image builds; baseline startup/recreation and an expanded smoke
run verifying manifest/private-log persistence across actual container recreation.
Tests cover log/manifest write failures, incomplete commit rejection, empty logs,
retry without duplicate entries, backend restart, hidden missing/corrupt commits,
ordering, privacy in listings, payload limits and callback failure gates. The
shared worker fixture commits real metadata before gateway/serving consume it,
while exact archive bytes and extracted files remain unchanged. Synthetic test
projects/data were removed; no application volumes were reset. Five known skips
remain; no paid AI request, push or hosted CI execution occurred.

M1.4 alone did not enforce immutable writes. Its commit is not a distributed
transaction or a publication gate. Lost upload/callback responses can leave storage and manager disagreeing;
reconciliation remains M2. Private metadata is separated from public output, but
internal storage authentication/redaction remain M4. Early failed-run log capture,
truthful compiler checks and complete archive validation also remain unfinished.
The full M1 exit is still unverified.

M1.5 completed (2026-09-05) in `0cbeaa5`. Storage writes artifact, private log,
manifest and timestamped event files through unique temporary files with checked
copy/fsync/close, atomic no-replace hard-link publication and directory sync.
Independent writers/processes cannot overwrite a winner. Same-size/SHA-256 retries
succeed; different bytes conflict with `409`, before or after manifest commit.
Manifest publication syncs prerequisite directories first. Existing legacy files
are protected too; partial versions retain their first-published files.

The allowed disabled-deletion option was selected: UI action and deletion clients
are removed, authenticated gateway DELETE returns `501` without DB/storage calls,
and storage DELETE remains unsupported (`405`). No version files are deleted,
including active live/preview versions. Future deletion must coordinate active
references and metadata cleanup; no retention/garbage collection is claimed.
See [immutability, retry and filesystem limits](docs/IMMUTABLE_VERSIONS.md).

Verification passed: six-module package baseline; storage and final gateway race
tests; concurrent backend-instance and competing-subprocess tests; five-module
race-enabled isolated integration; UI contract checks, zero-warning lint and
production build; affected gateway/storage/UI images through stack rebuild; fresh
startup and volume-preserving recreation. Smoke checks verify identical retries,
all three replacement conflicts, both deletion API responses, and unchanged
artifact/private metadata before and after recreation. Synthetic test data/stacks
were removed, not application volumes. Five known skips remain; no paid AI,
push or hosted CI run occurred.

M1.6 is next: bootstrap valid initial source from bundled `starter` with safe retry
and visible failure handling. M1.5 targets trusted Linux local storage/named volumes,
not unverified network filesystem semantics or hostile filesystem mutation.
Process death may leave hidden temporary names; no unsafe online sweep is added.
Errors after publication remain uncertain and require byte-identical retry.
Hardware power-loss behavior, worker fencing, callback reconciliation, archive
safety and full AI publication are still outside this acceptance result.

Repair job, storage, serving and UI contracts together, with tests exercising real HTTP handlers. Bootstrap a site using `starter`, record initial source, and define archive/manifest storage. Compile a deterministic edit fixture and round-trip its archive through storage and serving. Add version deletion support or disable the corresponding UI/API until implemented.

**Exit:** without an AI dependency, create a fresh site, submit the canonical job, obtain a valid immutable artifact and fetch its `public/index.html` through hosting. No manually seeded serving files or direct DB edits.

### M2 — Real worker and recoverable job lifecycle (4–7 days)

Implement the Docker spawner and a queue dispatcher with bounded concurrency. Build the selected real worker image with the compiler and trusted theme. Bootstrap/download source, execute a bounded edit, validate allowed changes, compile to `public/`, validate output, store the artifact and report completion. Add lease renewal, fencing enforcement, worker cleanup, callback retries and reconciliation for lost callbacks or manager restarts. Persist Redis data and reconcile gateway uncertainty; define safe retention for M1.2's nonexpiring reservations and legacy expiring jobs.

**Exit:** one real text request visibly changes a new site's HTML, two successive edits preserve each other, a concurrent same-site build is handled predictably, and worker failure/timeout/restart produces a recoverable terminal status without affecting live content.

### M3 — Complete browser journey and publishing (3–5 days)

Connect persisted job status to chat and version history; normalize timestamps/statuses. Fix preview activation and URL construction, first-preview nginx provisioning, atomic promotion/rollback, pointer preservation and deployment error reconciliation. Preserve user input across auth expiry and report actionable errors. Hide attachments, arbitrary domains and unsupported authentication choices. Verify dashboard/chat/modal use at desktop and mobile sizes.

**Exit:** through the UI only, a tester creates a site, edits, reloads to recover status, previews, publishes, edits again, and rolls back. Preview leaves live unchanged, all pages/assets resolve, and failed deployment leaves the last working version served.

### M4 — Controlled remote pilot (3–5 days)

Carry boundary protections into M1/M2 as those interfaces are implemented; complete this gate before remote access. Add explicit origin policies, internal request authentication, per-job callback credentials, request/archive limits, path and hostname validation, worker isolation, per-user usage limits and secret validation. Disable open signup or require invitations. Deliver reset email without logging tokens; make tokens single-use and handle session expiry. Configure HTTPS for app and hosted sites, private infrastructure ports, persistent Redis state, backups, restore instructions and useful correlated error logs.

**Exit:** two accounts cannot access each other's sites, jobs, artifacts or updates; unauthenticated callers cannot mutate internal services; invalid/oversized archives and paths fail safely; a backup restore and bounded AI failure test succeed. All MVP release checks below pass.

Dependency order: **M0 → M1 → M2 → M3 → M4**. Security work belongs alongside each relevant change, with M4 verifying the combined deployment. Rough total: **14–24 focused development days**, subject to M1 findings and the real CLI integration.

## Release acceptance scenarios

Automate the deterministic scenarios against actual service interfaces and browser UI. Keep the real-provider smoke test separate, explicitly invoked and cost-bounded.

1. From empty disposable volumes, start the stack and register/sign in. Create a unique platform subdomain with the starter theme; duplicate/invalid names return useful errors.
2. Submit an edit; observe a job ID, pending/running state and a completed version with a real manifest. Repeat with the real executor and confirm the requested content changed.
3. Preview before any live publication. Check home, another page, CSS, JS and an asset. Publish the exact previewed version and verify the live URL over HTTP locally / HTTPS in the pilot.
4. Make two unpublished edits; both survive in the newest draft. Publishing and previewing update only the intended DB pointer. Roll back to an earlier version and verify its actual HTML.
5. Refresh during a build, disconnect/reconnect, let authentication expire, and retry a failed request. Job state remains accurate, input is preserved, and duplicate requests/results do not create contradictory versions.
6. Kill a worker, exceed its deadline, fail artifact upload, lose a callback, and restart the manager/Redis. Each accepted job recovers or terminates visibly within its configured limit; locks and orphan containers are cleaned up.
7. Make a deploy/config validation fail. Verify the previous live site still responds; neither cleanup nor deletion can remove an active preview/live artifact. Reject writes to an already committed version.
8. With two users, test direct IDs as well as UI navigation for every site/job/version operation. Reject unauthorized internal writes, disallowed origins, traversal, symlink escapes, malformed manifests and oversized requests/archives.
9. Complete password recovery through delivered email, then reject token reuse. Restore a backup into disposable infrastructure and verify site ownership, version history and published content.

## Deferred roadmap

Preserve the longer-term ideas from the original TODO as a backlog, not release requirements: additional themes, SEO feeds/sitemaps, incremental compilation, attachments/screenshots, custom-domain verification and automated certificates, OAuth/2FA, team collaboration, billing, search/bulk APIs, storage providers, alternative queues, Kubernetes/autoscaling, tracing/metrics dashboards and multi-region operations. Reprioritize these after testers complete the core workflow and identify their actual constraints.
