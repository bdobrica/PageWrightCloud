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
| Execution | M2.1–M2.8 implement Docker launch, bounded dispatch, pinned CLI/compiler, isolation, fenced commits and result recovery. M2.9 (`25dcf08`) adds durable gateway history, Redis startup checks and restart recovery. Worker remains `pagewright-worker:m2.8`; Kubernetes remains a historical logging stub. | Container/log retention and real-provider acceptance remain M2.10 onward. Missing evidence follows the conservative operator runbook, never speculative redispatch. Drain/reconcile before coordinated upgrades; internal services require trusted operation. |
| First site | M1.6 (`a43d0ac`) reserves deterministic starter source and exact upload bytes, commits the `initial` archive/metadata before DB readiness, and exposes retry-safe setup through UI/API. M1.7 adds revision-2 layout metadata without changing persisted retry bytes. | New sites have validated source, not compiled or hosted output. Legacy sites are not automatically repaired; compilation remains M2. |
| Artifact transport | M1.3 (`0fa1044`) aligns raw gzip transport; M1.4 (`73d3635`) adds manifest-last completion. M1.5 (`0cbeaa5`) makes files write-once with retry/conflict semantics and disables version deletion in UI/API. | Storage is still opaque, not an archive-validation, authorization or publishing gate. Retention and future coordinated deletion remain separate work. |
| Archive layout | M1.7 (`1d9b5bf`) enforces [content/public/layout metadata](docs/ARCHIVE_LAYOUT.md), excludes runtime files, bounds staged extraction and deploys only public files. | Editable source survives future edits; structural checks do not identify secrets disguised in allowed files or establish safe HTML generation. |
| Compilation | M1.8 (`6bc86f1`) adds [compiler fixtures and filesystem checks](docs/COMPILER_CONTRACT.md). M2.4 integrates trusted compilation and named static checks; M2.5 verifies two accumulated edits through the production runner. Serving validates compiled archive structure. | Browser checks and paid-provider acceptance remain unverified; static checks do not establish safe HTML for remote multi-user hosting. |
| Deterministic round trip | M1.10 (`92ea617`) verifies [HTTP bootstrap → job → real worker/compiler → immutable storage → serving/nginx](docs/DETERMINISTIC_ROUNDTRIP.md), including byte-identical hosted HTML/assets and private-path 404s. | M1 contract exit verified with a test-only executor and launch bridge. This does not implement the production spawner, AI execution or root Compose hosting topology. |
| Version state | M1.2 persists the job/target mapping before dispatch. M1.9 normalizes storage versions for UI. M2.9 adds PostgreSQL lifecycle history and atomically reconciles verified manager outcomes into submission/version status. | Owner-checked history retrieval, browser refresh and combined DB/storage views remain M3.1. Conservative terminal outcomes must not be reopened by late materialization. |
| Live updates | [UI socket](pagewright/ui/src/hooks/useWebSocket.ts) sends a query token; [auth middleware](pagewright/gateway/internal/middleware/auth.go) accepts only a bearer header. [Hub](pagewright/gateway/internal/websocket/hub.go) has no caller publishing build results and no implemented ownership filter. M1.1 aligns status vocabulary to `pending/running/completed/failed` and validates UI payloads. | Schema mismatch fixed; delivery and authorization remain broken. Implement owner-checked retrieval/polling and repair WebSockets before enabling them. |
| Deploy / preview | M1.9 aligns gateway deploy/live/preview bodies with serving's `version` field. Preview UI still only opens a URL and does not activate a preview. | Implement the preview action, correct returned URLs and verify preview assets/navigation in M3. |
| Deployment consistency | [DB update](pagewright/gateway/internal/database/sites.go) sets both live and preview IDs, clearing one when the other changes. Serving removes the old symlink before creating the new one. | Preserve the other pointer, handle DB failures, and replace symlinks using an atomic rename. |
| Hosting | [Compose](docker-compose.yaml) separates serving and nginx, but serving executes local `nginx -s reload`. Preview activation does not create nginx config. | Make nginx configuration/reload part of a supported topology; first preview must work before first publish. |
| Job reliability | Durable dispatch, fencing and result recovery are complemented by M2.9's [Redis durability gate, gateway recovery, TTL protection, audit and retention policy](docs/JOB_DURABILITY.md). Abrupt Redis/gateway/manager restart and replacement-manager reconnect are tested. | Intent is never replayed. Missing/legacy evidence and storage outages retain uncertainty/capacity. Existing data needs verified backup/restore before replacement; arbitrary disk loss, rollback and multi-host HA are not solved. |
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
2. **Keep editable source with each version.** Use an archive containing `content/`, generated `public/` when available, and safe layout metadata. Keep source-version provenance, theme/compiler versions and check results in the private build manifest; trusted compiler/check integration remains M2. Execution logs are private metadata. Publish only `public/`; never expose source, prompts, instruction files or secrets through nginx. Render from a trusted bundled theme outside the AI-writable directory. Preserve the same artifact when promoting preview to live.
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

M1.5 targets trusted Linux local storage/named volumes,
not unverified network filesystem semantics or hostile filesystem mutation.
Process death may leave hidden temporary names; no unsafe online sweep is added.
Errors after publication remain uncertain and require byte-identical retry.
Hardware power-loss behavior, worker fencing, callback reconciliation, archive
safety and full AI publication are still outside this acceptance result.

M1.6 completed (2026-09-05) in `a43d0ac`. Migration 008 adds explicit
legacy/pending/ready site initialization and durable bootstrap byte reservations.
Authenticated creation normalizes/validates FQDN and maps `starter` (plus the old
`template-1` alias) to the bundled starter seed. Site and archive/manifest/log bytes
are reserved in one transaction before storage HTTP. Artifact → log → manifest
writes use those same bytes on retries; only then are the database initial version
and ready state committed together. Initial sites are disabled with no live/preview
assignment. Pending build/enable calls are rejected before downstream work.

Same-owner/domain retries and concurrent requests return the same site and initial
version; different owners/templates conflict. UI now selects starter and exposes
pending sites with Resume Setup, preserving the domain in the creation form.
Retryable failures return `503`; immutable data conflicts return `409` with repair
guidance. Existing sites remain legacy, not falsely certified initialized. The
seed includes valid site configuration and home Markdown; no public output,
instructions, credentials or trusted theme code is embedded in the source archive.
See [bootstrap semantics and compatibility](docs/SITE_BOOTSTRAP.md).

Verification passed: six-module package baseline and final gateway race tests;
two five-module isolated integration runs; migration 007→008, legacy preservation,
reservation rollback and DB reconnect; partial artifact/log/manifest failures,
lost manifest acknowledgement, failed DB confirmation, concurrency and ownership
tests; deterministic archive/config tests; starter compiler fixture including this
seed; UI contract checks, zero-warning lint and build; affected gateway/UI image
builds and two startup/recreation runs. Initial source and metadata persist through
container recreation and creation replay. Test stacks/data were cleaned up without
resetting application volumes. Five known skips remain; no paid AI, push or hosted
CI execution occurred.

At M1.6 handoff, M1.7 was next; its completed work is recorded below. M1.6 initial metadata
truthfully says source-only, not compiled/checks-passed; it does not implement the
real worker compiler path, latest-draft selection, legacy repair, domain ownership,
cross-service atomicity, orphan cleanup or publishing. Full M1 exit remains unverified.

M1.7 completed (2026-09-06) in `1d9b5bf`. The
[archive contract](docs/ARCHIVE_LAYOUT.md) defines editable `content/`, optional
generated `public/`, and a strict versioned layout manifest, distinct from private
build metadata/logs. Packing includes only those trees and generated metadata;
root execution instructions, logs, credentials and theme code do not travel.
Unsafe paths, links/special entries, duplicates, extended tar metadata, known
private filenames, public source files and inconsistent layouts are rejected.
Compressed/expanded/per-file/entry limits and gzip trailer checks bound extraction.

Worker extraction stages into an absent/empty workspace and retains source for
subsequent edits. Serving validates all entries but extracts only public files;
staged failures leave existing versions/live links intact. SHA-256 markers outside
the public root support same-byte retries and reject conflicting/unverified cache
replacement. Concurrent retries are covered; retention skips in-progress stages.
Bootstrap revision 2 includes source-layout metadata. Persisted revision-1 initial
archives remain worker-compatible, without rewriting immutable reservation bytes.
Worker private metadata counts actual archive files and no longer claims stubbed
checks passed.

Verification passed: final six-module package baseline; worker/serving archive and
worker runner race tests; two five-module isolated integration runs; starter and
bootstrap compiler fixtures; gateway/worker/serving image builds; fresh startup and
volume-preserving recreation. Tests cover second-edit source preservation, private
runtime canaries, public-root HTTP 404s, source-only deployment rejection, hostile
paths/links, checksum damage, all size/entry limits, failed writes/extraction,
concurrent deployment retries and staging cleanup protection. Policy-file parity,
shell syntax and staged whitespace checks passed. Disposable test stacks/data
were removed; application volumes were untouched. Five known skips remain.

At M1.7 handoff, M1.8 was next; its completed work is recorded below. M1.7
does not scan arbitrary content for disguised secrets, prove compiler execution
or output freshness, repair historical archives/caches, make symlink activation
atomic, or establish internal-service authorization. Trusted compilation,
credential isolation and publication remain M2/M3/M4. No paid AI, push or hosted
CI run occurred; the complete M1 exit remains unverified.

M1.8 completed (2026-09-06) in `6bc86f1`. Committed fixtures exercise the actual
starter theme with Markdown/MDX home discovery, nested/grouped pages, full navigation
titles, active links, repeated-heading TOC anchors, component rendering, token
overrides and exact text/binary asset copying. Adversarial cases cover malformed
site/theme config, missing/invalid templates, unknown components, malformed props,
numeric diagnostic locations, raw-script/unsafe-URL escaping, route traversal,
duplicate routes and input/output symlinks.

The tests reproduced and fixed MDX discovery, overbroad home-prefix removal,
silent route overwrites, order-dependent navigation titles, lost formatted title
text, mismatched TOC IDs across components, nondeterministic token ordering and
asset/write symlink following. Component blocks in fenced code stay literal;
malformed JSON-looking props, non-string leaves and duplicate/empty keys fail.
The documented YouTubeVideo component name works with its previous spelling
retained as an alias. CSS tokens use a conservative string grammar and base URLs
are limited to HTTP(S) origins.

Compiler output must now be absent/empty and disjoint from source/theme.
Rendering stages privately and publishes only after every phase succeeds;
old output and outside sentinel files survive rejected builds. Unique file
temporaries replace predictable `.tmp` names. Path checks inspect symlinks before
normalizing `..`, while resolving the inherited working-directory alias.
These checks assume quiescent local Linux inputs, not a hostile concurrent writer.
Worker integration must compile fresh output before replacing workspace public
files; this dependency is recorded in M2.4.

The compiler's binary ignore rule also accidentally excluded `cmd/pagewrightc/`.
It now applies only to the root binary; CLI source and subprocess exit-code tests
are tracked. The [compiler contract](docs/COMPILER_CONTRACT.md) and README describe
the actual scope rather than claiming an AI sandbox or existing worker integration.

Verification passed: six-module package baseline; compiler race tests with 86.3%
statement coverage across internal packages; compiler vet; original starter and
gateway bootstrap CLI smoke builds; staged whitespace checks. A fresh export of
the staged Git index passed compiler race/CLI tests and both smoke builds, proving
the result does not depend on ignored local CLI files. The disposable export was
removed; repository/application data were untouched. Five pre-existing worker/
serving skips remain unchanged; no compiler skips were added. No Docker service
code changed, and no paid AI, browser journey, push or hosted CI run was performed.

At M1.8 handoff, M1.9 was next; its completed work is recorded below. Real worker
compilation, credential/resource isolation, production output validation and
hosting activation remain M2–M4. The full M1 exit is still unverified.

M1.9 completed (2026-09-06) in `8609d32`. Gateway now sends `version` in
serving deployment, live activation and preview bodies, normalizes trailing base
slashes and rejects redirects. The owner-checked version endpoint maps storage's
committed records to explicit `id/site_id/build_id/status/created_at` summaries.
Artifact identity is distinct from a DB row/job identity; timestamps normalize to
UTC, status is `completed`, sorting is deterministic and malformed records fail
instead of leaking incomplete UI data. Pagination handles invalid defaults,
empty collections and huge page numbers without overflow.

UI version parsing checks identities, canonical status, timestamps and pagination.
Load failures are visible; stale selections/lists and late responses are cleared
or ignored when the site changes. Shared `/chat/:fqdn` routing and encoded site
links agree with Chat's FQDN lookup, covered by executable router matching.
See [version/serving contract and remaining boundaries](docs/VERSION_API.md).

Verification passed: six-module package baseline; gateway client/handler race
tests; five-module isolated integration using real PostgreSQL/storage for version
listing, ownership, pagination and upstream failures; 20 UI contract tests;
zero-warning UI lint and production build; affected gateway/UI image builds and
fresh startup/persistent-volume recreation. Disposable test stacks were removed
without changing application volumes. Five existing skips remain unchanged.

At M1.9 handoff, M1.10 was next: deterministic compiled-artifact service round trip. The version
list is committed-artifact history, not pending/failed job reconciliation, and
completed initial source is not a publishable build. UI preview activation,
hosting URLs, atomic symlinks, preservation of the other DB version pointer and
serving/DB failure compensation remain M3. No paid AI, browser publishing journey,
push or hosted CI execution occurred; the full M1 exit was still unverified at that handoff.

M1.10 completed (2026-09-06) in `92ea617`. The integration-only deterministic
executor edits bootstrapped Markdown and invokes the actual compiler with the
trusted starter theme. A worker test entrypoint calls the production `runJob`
using the manager's complete HTTP snapshot, preserving internal lease fields.
It fetches/unpacks source, executes, packs/validates, commits archive/log/manifest
and reports completion through the actual service interfaces.

The end-to-end scenario registers and creates the site through real gateway HTTP
handlers/auth middleware and submits the canonical build. It uses PostgreSQL,
Redis, manager, storage and serving services in isolated containers. Gateway
list/download/deploy routes lead to real serving activation and nginx hosting;
HTML and every public asset match the archive bytes. Source survives in the
immutable archive, private paths return 404, a conflicting overwrite preserves
bytes, and a rejected source-only deployment leaves the compiled live page intact.
No public files are manually seeded and the scenario makes no direct DB edits.
See [test topology and limitations](docs/DETERMINISTIC_ROUNDTRIP.md).

Verification passed: six-module package baseline; five-module race-enabled
isolated integration including `TestCompiledArtifactRoundTrip`; shell syntax and
staged whitespace checks. Test nginx's hostname hash size was adjusted after a
minimal nginx check reproduced rejection of the long UUID-based fixture domains.
Disposable stacks and their ephemeral data were removed; application volumes
were untouched. Five existing skips remain unchanged. No UI code changed, and no
paid AI, browser journey, push or hosted CI run was performed.

M1's deterministic contract exit is now verified. At M1.10 handoff, next was M2.1: implement the
production Docker spawner. The test harness deliberately bridges that stub;
the fixture binary is absent from production images. Production compiler/output
validation remains M2.4 (`checks_passed` is still false), and root Compose's
separate nginx/reload topology, preview and deployment reconciliation remain M3.
The test-only co-located nginx topology is not a production deployment fix.

Repair job, storage, serving and UI contracts together, with tests exercising real HTTP handlers. Bootstrap a site using `starter`, record initial source, and define archive/manifest storage. Compile a deterministic edit fixture and round-trip its archive through storage and serving. Add version deletion support or disable the corresponding UI/API until implemented.

**Exit:** without an AI dependency, create a fresh site, submit the canonical job, obtain a valid immutable artifact and fetch its `public/index.html` through hosting. No manually seeded serving files or direct DB edits.

### M2 — Real worker and recoverable job lifecycle (4–7 days)

M2.9 completed (2026-09-06) in `25dcf08`. Worker execution and isolation remain
unchanged on `pagewright-worker:m2.8`. Migration 009 adds PostgreSQL lifecycle
history and durable recovery diagnostics. A bounded gateway loop claims never-
dispatched ready submissions once and observes exact manager identities thereafter.
Verified outcomes update submission, version and history atomically; terminal
outcomes cannot regress or be revised by late filesystem materialization.
Claimed-before-send submissions with missing manager evidence remain uncertain
and operator-visible, never automatically redispatched.

Production manager startup now verifies AOF/fsync/no-eviction settings and AOF
write health, protects surviving legacy job/receipt/fence/index TTLs, and never
makes site leases permanent. Root/standalone Compose explicitly reject truncated
AOF loading. Thirty-day incremental cleanup removes only disposable terminal
dispatch metadata. Canonical job/request/version identities, receipts and fences
remain nonexpiring; the lifecycle journal has at most one event per status/job.
This bounds bookkeeping and polling history, not total retained identity storage.
The packaged read-only audit and [operator runbook](docs/JOB_DURABILITY.md) provide
a conservative quarantine/restore path when evidence is missing or corrupt.

Acceptance passed: repository-wide package suite, full race-enabled service
integration, gateway/manager race and vet, root production image builds, packaged
audit, independent-manager reconnect, and isolated root-stack SIGKILL/recreation
checks. Tests cover atomic journal rollback, concurrent recovery claims, gateway
recreation, missing manager evidence without POST, late verified completion,
terminal non-regression, legacy TTL protection and retained deduplication after
metadata cleanup. The crash smoke verifies manager job persistence and recovered
PostgreSQL history while a claimed-but-never-received job remains uncertain.
Two existing serving skips remain. No paid calls, remote changes, application-data
migration, isolation relaxation or push occurred; disk-loss/backup-restore/HA
safety is not claimed.

Next: **M2.10**, exited/orphan worker and temporary/private-log cleanup with
documented retention. M3.1 still owns owner-scoped history endpoints and browser
refresh; M4 owns service authentication. Back up and review surviving legacy TTLs
before coordinated upgrade; do not enable writers on rolled-back data until
possibly executed work is quarantined or its exact evidence is restored.

M2.8 completed (2026-09-06) in `e0c484e`. Selected image/build defaults now use
`pagewright-worker:m2.8`. Result delivery makes at most four identical attempts
within 25 seconds, bounds each response and resolves ambiguous acknowledgements
through exact job-outcome lookup. Terminal callback duplicates still return 409.
Uncertain manifest/completion delivery no longer sends a contradictory failure.

The manager's bounded Docker reconciler verifies container ownership and attempt
identity, observes exits or Redis-clock lifetime expiry, and checks artifact,
private-log and manifest bytes against the exact durable digest reservations.
Complete materializations can recover completion after lease expiry without
granting new write authority. Missing/incomplete bytes fail conservatively while
preserving receipts. Redis compares the observed receipt and current attempt,
then atomically records terminal outcome and releases matching lease/site/capacity
reservations. Definite spawn failure now releases its lease in the same atomic
dispatch outcome. See [recovery semantics and limits](docs/RESULT_RECOVERY.md).

Acceptance passed: repository-wide package suite, full race-enabled service
integration, manager/worker race and vet, selected production worker image build,
installed CLI/compiler isolation regressions, and real-Docker launch/ownership
inspection. Recovery tests cover disconnected callback responses, exact terminal
lookup, bounded retry/deadline behavior, stale receipts and terminal races,
replacement-lease protection, missing materialization, expired-attempt completion,
timeout and atomic spawn-failure release. Two existing serving skips remain.
No privileged containers, isolation relaxation, paid calls, remote changes,
production deployment or push were part of M2.8.

Next: **M2.9**, broader restart durability and missing-evidence recovery policy.
Storage outages retain capacity until evidence is available. Legacy/corrupt Redis
records remain fail-closed. Already-reserved bytes may materialize late without
reopening a conservative failed job. Timeout fencing does not guarantee a worker
was killed when Docker is unavailable; orphan cleanup/retention remains M2.10.
Drain/reconcile before coordinated upgrades; service authentication remains M4.

M2.7 completed (2026-09-06) in `9740d79`. Selected image/build defaults now use
`pagewright-worker:m2.7`. The existing random lock token is the attempt identity;
per-site fence allocation and lease acquisition are atomic. Redis-clock renewal
checks running attempts, active/site reservations and live tokens/fences, never
reacquiring an expired lease. All replicas can renew authoritative attempts up
to the configured lifetime. Startup validates TTL/renewal/lifetime relationships.

Storage stages and syncs complete request bytes before obtaining an atomic Redis
digest reservation tied to the live attempt and target version. That reservation
is the logical write commit; a no-replace filesystem link materializes the exact
approved bytes. This deliberately does not claim a transaction spans Redis and
the filesystem. Manifests require artifact/log prerequisites and matching fencing.
Completion callbacks require the manifest reservation. Outcome, lock removal and
site/capacity release are atomic; stale/superseded and duplicate terminal callbacks
return 409 without mutation. Admission retries remain idempotent. See
[commit semantics and upgrade requirements](docs/FENCED_COMMITS.md).

Acceptance passed: six-module package suite, full race-enabled service
integration, manager/storage/worker race and vet, selected production image build,
installed CLI/compiler regressions and real-Docker launch. New tests cover two
concurrent jobs beyond two original TTLs, expired-lease non-resurrection,
monotonic replacement fences, stale/changed-byte writes, missing-manifest and
duplicate terminal rejection, and an upload expiring before publication. The
compiled worker round-trip uses the actual fenced storage and callback path.
Two existing serving skips remain. Disposable test containers/networks were
removed; caches may remain. No paid-provider/browser/hosted-CI/production-deployment
acceptance or push. AppArmor policy is unchanged; no host administrative action
was required for M2.7.

Next: **M2.8**, bounded result delivery and reconciliation of uncertain outcomes.
Drain/reconcile active jobs before upgrading manager, storage and worker together;
do not mix old unfenced workers, reset fences or delete digest reservations.
Reserved-but-not-materialized writes and lost acknowledgements require M2.8/M2.9
reconciliation; retention remains M2.10 and service authentication remains M4.

M2.6 completed (2026-09-06) in `9fceb7c`. Selected image defaults and build
targets now use `pagewright-worker:m2.6`. Fixed CPU/memory/PID/storage/FD/log
ceilings, read-only root, outer CLI filesystem/PID isolation and process-tree
cancellation bound execution. The CLI stops at 10 minutes or 1 MiB output; job
context expires at 15 minutes and the whole-runner watchdog at 16 minutes.
Kill/SIGTERM/SIGINT cancel storage I/O and compilation; upload checks cancellation.

The user-approved proc compatibility work is accepted on `ublo.ro` (Linux
6.12.86+deb13-amd64, Docker 29.5.0, cgroup v2). The separate enforcing
`pagewright-worker-proc` AppArmor profile replaces Docker's proc overmounts with
explicit denies while preserving non-proc masks. A tool-only seccomp filter
blocks namespace/mount operations after trusted bubblewrap setup. The manager
requires a compatible image label and resolves to image ID; startup checks actual
tool namespace denial before provider contact. No privileged/unconfined mode or
host-wide sysctl change. See [host evidence](docs/M2_6_HOST_ACCEPTANCE.md) and
[worker isolation / kernel-surface assessment](docs/WORKER_ISOLATION.md).

Acceptance passed: six-module package tests, full isolated service integration,
real-Docker spawner, local and enforcing-AppArmor host installed CLI/compiler
suites, manager/worker race/vet, selected production image build and actionlint.
Tests verify actual workspace editing, sibling/socket/theme/provider-key denial,
network/namespace denial, protected proc access, resource ceilings and detached
child teardown after proof of startup. The kill test is restored; two pre-existing
serving skips remain. Disposable test containers/networks were removed; image
caches and remote evidence remain. No paid-provider, browser, hosted-CI or
production-deployment acceptance, and no push. Runtime-loaded profiles remain;
persistent installation is an administrator deployment step.

Next: **M2.7**, lock renewal and fenced/attempt-scoped result acceptance.
OOM/hard-kill recovery remains M2.8/M2.9; retention remains M2.10 and broader
runner failure coverage M2.11. Internal authorization and comprehensive secret
policy remain M4. Isolation limits visibility and lifetime but is not proof
against kernel exploits or a compromised trusted CLI.

M2.5 completed (2026-09-06) in `cb9c35e`. New submissions choose the latest
manifest-committed non-bootstrap build from owner-scoped storage, then live,
then bootstrap. Sorting matches version history (manifest timestamp descending,
build ID ascending for ties); stale gateway status rows are not completion
evidence. Publishing/rollback does not discard newer draft history. Unavailable
or malformed storage responses stop new submissions before reservation/dispatch.
The existing durable submission pins the base across retry, restart, newer drafts
and storage outages. Chat displays the actual accepted `source_version`.

Acceptance passed: six-module package suite; full isolated service integration;
gateway race/vet; UI contract tests, zero-warning lint and production build.
The HTTP/database replay test proves source pinning and no fallback dispatch on
storage failure. The service round trip performs two unpublished source edits
through the production worker/compiler, verifies both changes in source and HTML,
preserves the first draft bytes and leaves live unset before explicit publication.
Three existing skips remain. Disposable integration containers/networks were
removed; application data was untouched. No paid calls, browser/hosted-CI
acceptance or push. The selected worker remains `pagewright-worker:m2.4` because
this milestone changes gateway/UI selection and test-only executor fixtures.

Next: **M2.6**, complete the broader worker isolation/resource/cancellation gate.
Durable browser history remains M3. Selection is a persisted submission-time
snapshot, not a queued-job rebase. See [build source semantics](docs/BUILD_SOURCE.md).

M2.4 completed (2026-09-06) in `ef864e5`. The selected image is
`pagewright-worker:m2.4`, retaining the pinned CLI and existing unprivileged
sandbox while packaging compiler 0.1.0 and root-owned read-only starter 1.0.0.
Worker snapshots reject forbidden source/output/instruction edits and special
files. Accepted content is digest-verified into a sibling outside the agent's
writable root, compiled with trusted inputs into fresh output, checked for HTML
structure/local references and archive validity, then used to replace workspace
`public/`. The uploaded archive contains exactly the frozen source compiled.
Compile/validation failures preserve old output and never begin upload; stale
pages cannot survive a successful fresh build.

Manifests now report filesystem-derived changed paths, compiler/theme versions
and four named checks. `checks_passed=true` means those static gates passed;
`browser_checks_performed=false` prevents interpreting the legacy zero console
count as browser validation. Source assets use `content/<page>/assets/`.
The deterministic executor edits source only; production runner compiles it in
the service round trip and the persisted manifest is asserted.

Verification: all six module package suites and full isolated service integration;
worker race/vet and manager config race/vet; installed compiler builds, exact
assets, deletion/failure preservation and read-only input checks under UID 1000,
no network/capabilities; installed CLI sandbox regression; selected production
image build and actionlint. Acceptance caught and fixed writable theme modes
inherited from the checkout. Three existing skips remain. Disposable integration
containers/networks were removed; application data was untouched and image caches
may remain. No paid calls, browser acceptance, hosted CI verification or push.

Next: **M2.5**, latest-draft source selection and accumulating unpublished edits.
Broader cancellation/resource/credential isolation remains M2.6; real-provider
acceptance remains M2.12. See [trusted build scope and limitations](docs/WORKER_BUILD.md).

M2.3 completed (2026-09-06) in `b8fb6c0`. The selected worker image is now
`pagewright-worker:m2.3`, with integrity-locked Codex CLI 0.153.4 instead of a mock.
The native executable receives explicit Responses-provider authentication,
developer instructions and a stdin prompt using fresh per-job state. Shell
snapshots are disabled and secret-name exclusions are explicit; capture is
bounded and exact-key redacted. Unsupported hosts fail a sandbox preflight
before any provider call; no unsafe fallback is offered.

With explicit user approval, the minimum runtime compatibility prerequisite was
brought forward from M2.6. Workers run as UID/GID 1000 with zero capabilities,
no-new-privileges and an owned private tmpfs. A pinned default-deny seccomp
profile adds only audited nested-user-namespace and mount operations. The outer
user still cannot mount; the inner CLI sandbox denies outside writes and command
network access. No privileged containers, unconfined profiles, daemon defaults or
global sysctls were introduced. An explicit worker-only AppArmor profile and
fixed-name opt-in support AppArmor hosts; syntax was checked without kernel
loading, but enforcement is not verified on this WSL host. CI includes the named
profile setup and denial tests; hosted CI has not been run.

Verification passed: installed CLI/fake-API authentication, instructions, prompt,
actual shell execution, key exclusion, permitted writes and denied outside writes,
reachable-loopback denial, outer capability/mount checks and runtime cleanup;
six-module package baseline; full isolated service integration; manager/worker
race/vet; real-Docker launch/inspect and workspace checks; production image build;
fresh startup and persistence recreation; AppArmor parsing, Compose overlay merge
and actionlint. Two skipped executor/parsing tests were restored; three existing
skips remain. Disposable test resources were removed without changing application
data; test image caches may remain. No paid calls or push occurred.

Next: **M2.4**, trusted compiler/theme integration. Full resource limits,
cancellation, credential isolation, AppArmor host enforcement and user-namespace
attack-surface review remain M2.6; real-provider acceptance remains M2.12. See the
[CLI contract, provenance and host prerequisites](docs/WORKER_CLI.md).

M2.2 completed (2026-09-06) in `5da77ad`. HTTP admission now atomically stores
and queues `pending` jobs without spawning. A background dispatcher enforces a
shared active-job cap (default 4), claims with Redis-clock leases and random
ownership tokens, and records irreversible launch intent before Docker. Expired
pre-intent claims recover without allowing stale owners to launch; intent is
never replayed. Terminal updates atomically free site admission/capacity and
preserve container/lock metadata even when callbacks race dispatch completion.
Manager shutdown stops claims and waits for bounded dispatch operations.

Minimum Redis persistence was brought forward from M2.9: root and standalone
Compose use a named volume and AOF with `appendfsync always`. Existing application
Redis data was not migrated; preserve/export and restore writable-layer data
before replacing an old container. Startup imports bounded legacy queue entries,
preserves history and reserves capacity for old running jobs without relaunch.
Malformed/conflicting legacy state fails closed. See the
[dispatch contract and upgrade requirements](docs/QUEUE_DISPATCH.md).

Verification passed: six-module package baseline; two full isolated integration
runs including real-Redis expiry, stale claims, intent interruption, independent
manager clients/restarts, global capacity, legacy import, corruption and fast
callbacks; manager race/vet; isolated real-Docker launch regression; root-stack
recreation preserving an acknowledged job, its failure and retry deduplication.
No AI worker/provider ran in the persistence smoke test (unique missing image).
Five existing skips remain. Disposable test containers/networks/volumes were
removed; application data was untouched, hosted CI was not run and nothing was
pushed.

Next: **M2.3**, package/pin the real non-interactive AI CLI. M2.2 recovers only
pre-intent dispatch automatically. A crash after intent, even before Docker,
holds capacity conservatively; unresolved launches, missing callbacks, lease
renewal, worker fencing, cancellation, history retention and gateway uncertainty
still require M2.7–M2.9. This is not full restart recovery or a working AI MVP.

#### M2.1 implementation record

M2.1 completed (2026-09-06) in `d9bb650`. The Docker spawner now creates and starts
containers through the Unix-socket Engine API with bounded acknowledgements and
timeouts. A deterministic job-derived name prevents adoption/restart after name
conflicts; labels preserve job/site/network correlation. Root Compose derives the
worker network from its project, uses a private `/work` tmpfs and passes explicit
manager/storage endpoints. Worker credentials use a separate provider key and
an environment allowlist, not the manager/gateway environment. Workers receive
no Docker socket, host binds or published ports; capabilities are dropped and
privilege escalation is disabled.

The supported image/build targets agree on `pagewright-worker:m2.1` from
`pagewright/worker`; untagged/latest images are rejected and missing images are
not automatically pulled. The old manager worker image is marked historical and
no supported target builds it. M1 integration uses a build-tag-only manual bridge
instead of a production logging stub; production does not register that bridge.

Only explicit no-start evidence produces durable `failed/spawn_failed` and early
lock release. Ambiguous create/start errors return `spawn_uncertain`, keep the
existing running reservation and do not release the lock. Container identity is
merged without overwriting a racing terminal callback; persistence uses a bounded
context independent of client disconnect. Same-identity retries retrieve the
reservation without another launch. The lock still expires normally: renewal,
fencing, durable recovery and queue dispatch are not implemented by this change.

Verification passed: six-module package baseline; repeated manager race tests and
vet; separate real-Docker fixture acceptance for create/start, network health,
workspace writes, credential/socket exclusions, duplicate-name refusal and missing
image rejection; full five-module isolated integration including M1 compiled
nginx hosting; selected worker image build; production-manager stack startup and
persistent-volume recreation. A test-server counter race exposed by dropped
acknowledgements was fixed. Dedicated daemon acceptance is wired into CI; hosted
CI was not run. Disposable test containers/networks/data were removed without
changing application volumes. Five existing skips remain; no paid provider or
browser publishing journey was run and nothing was pushed.

Next: M2.2, bounded queue dispatch. See [configuration, authority and remaining
limitations](docs/DOCKER_SPAWNER.md). The manager now has host-level Docker socket
authority; this is not a remote-safe deployment or an AI sandbox. The worker still
contains the placeholder executor, its compiler is not integrated, callback/storage
credentials are not per-job tokens, and exited containers retain daemon metadata
until removal. Real AI/compiler integration, full resource isolation, callback
recovery and cleanup remain M2.3–M2.10; internal authentication remains M4.

Replace request-handler launching with a queue dispatcher with bounded concurrency. Build the selected real worker image with the compiler and trusted theme. Bootstrap/download source, execute a bounded edit, validate allowed changes, compile to `public/`, validate output, store the artifact and report completion. Add lease renewal, fencing enforcement, worker cleanup, callback retries and reconciliation for lost callbacks or manager restarts. Persist Redis data and reconcile gateway uncertainty; define safe retention for M1.2's nonexpiring reservations and legacy expiring jobs.

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
