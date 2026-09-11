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
| Job submission | M1.1 aligns the [canonical job contract](docs/adr/0001-architecture-decisions-job-contract.md); M1.2 adds [durable submissions](docs/adr/0002-architecture-decisions-build-submissions.md), independent IDs and retry-key deduplication across gateway/manager/UI. | Wire and pre-dispatch persistence gaps fixed and HTTP-tested; reliable dispatch/recovery and live status synchronization remain M2/M3. |
| Execution | M2.1–M2.11 implement isolated Docker execution, trusted compilation, fenced persistence, recovery, retention and fault acceptance. M2.12 (`4f17a3c`) verifies a budget-guarded real Luna edit in compiled HTML. Selected worker: `pagewright-worker:m2.12`; Kubernetes remains a historical logging stub. | M2 items complete; browser history/publishing and pilot security remain M3/M4. Missing evidence stays quarantined, never replayed or speculatively deleted. Drain/reconcile before coordinated upgrades, including storage writers; internal services require trusted operation. |
| First site | M1.6 (`a43d0ac`) reserves deterministic starter source and exact upload bytes, commits the `initial` archive/metadata before DB readiness, and exposes retry-safe setup through UI/API. M1.7 adds revision-2 layout metadata without changing persisted retry bytes. | New sites have validated source, not compiled or hosted output. Legacy sites are not automatically repaired; compilation remains M2. |
| Artifact transport | M1.3 (`0fa1044`) aligns raw gzip transport; M1.4 (`73d3635`) adds manifest-last completion. M1.5 (`0cbeaa5`) makes files write-once with retry/conflict semantics and disables version deletion in UI/API. | Storage is still opaque, not an archive-validation, authorization or publishing gate. Retention and future coordinated deletion remain separate work. |
| Archive layout | M1.7 (`1d9b5bf`) enforces [content/public/layout metadata](docs/adr/0007-architecture-decisions-archive-layout.md), excludes runtime files, bounds staged extraction and deploys only public files. | Editable source survives future edits; structural checks do not identify secrets disguised in allowed files or establish safe HTML generation. |
| Compilation | M1.8 adds compiler fixtures; M2.4 integrates trusted compilation/static checks; M2.5 verifies accumulated edits. M2.12 verifies a paid Luna edit through the actual sandboxed CLI and compiler. Serving validates archive structure. | Browser checks remain unperformed; one synthetic paid edit and static checks do not establish safe HTML for remote multi-user hosting. |
| Deterministic round trip | M1.10 (`92ea617`) verifies [HTTP bootstrap → job → real worker/compiler → immutable storage → serving/nginx](docs/DETERMINISTIC_ROUNDTRIP.md), including byte-identical hosted HTML/assets and private-path 404s. | M1 contract exit verified with a test-only executor and launch bridge. This does not implement the production spawner, AI execution or root Compose hosting topology. |
| Version state | M1.2 persists job/target identity; M2.9 atomically reconciles verified manager outcomes into submission/version/history. M3.1 (`38e908e`) exposes owner-scoped history and intersects completed submissions with committed storage. M3.2 (`b1af7d6`) polls active jobs on the current history page with bounded backoff and refreshes versions on completion. | Conservative terminal outcomes are never reopened by late materialization. Reads report the last saved observation; polling pauses at explicit limits and supports manual resume. Actual browser acceptance remains M3.12. |
| Live updates | M3.2 supplies bounded owner-checked history polling. M3.3 (`7f99456`) removes the broken browser socket transport and gateway hub/upgrader; `/ws` returns 501. | Polling is the MVP transport. Future sockets require browser-compatible authentication, strict origins, owner/site filtering, real event delivery/resynchronization and tested reconnect cleanup; no re-enable flag exists. |
| Deploy / preview | M3.4 activates Preview before opening its validated URL. M3.6 (`830c476`) shares configured URLs across API/UI entry points and serves preview on `preview.<site-fqdn>/`; nested navigation/assets and exact-artifact promotion pass real compiler/nginx integration. | Configure DNS/TLS for both hosts and migrate existing generated configs by reactivating Preview; custom configs require operator review. Actual browser journey remains M3.12. |
| Deployment consistency | M3.7 (`ec47934`) persists sequenced intent and reconciles exact serving receipts into one DB pointer transaction. M3.8 (`4466271`) replaces pointers atomically and protects active/receipt-pinned cache versions through retention and rollback. | Preserve sequence/receipt evidence and single-writer operation. Post-rename errors remain uncertain, not speculative rollbacks. Atomic pointer selection is not a multi-request browser snapshot; enrolled-site deletion remains guarded until a coordinated tombstone protocol exists. |
| Hosting | M3.5 (`2e1eea2`) supervises API/hosting nginx behind a fixed public proxy with recoverable config changes. M3.6 adds separate preview hosts; M3.7/M3.8 provide receipt recovery, atomic selection and active-aware cache retention. Production and integration share this topology. | Upgrade coordinated services without deleting volumes. Old nginx workers can briefly drain after new-generation readiness. Review the soft cache budget/grace policy before enabling cleanup on existing installations; evicted rollback needs canonical storage. Pilot security remains M4. |
| Job reliability | Durable dispatch, fencing and result recovery are complemented by M2.9's [Redis durability gate, gateway recovery, TTL protection, audit and retention policy](docs/JOB_DURABILITY.md). Abrupt Redis/gateway/manager restart and replacement-manager reconnect are tested. | Intent is never replayed. Missing/legacy evidence and storage outages retain uncertainty/capacity. Existing data needs verified backup/restore before replacement; arbitrary disk loss, rollback and multi-host HA are not solved. |
| User-facing gaps | M3.9 gates unsupported capabilities; M3.10 preserves tab drafts and retry identities. M3.11 (`375def7`) adds native modal focus, keyboard navigation and responsive-layout fixes verified in a five-viewport Firefox audit. Reset email remains M4.8; M4.2 (`544901a`) removes routine reset-token logging. | Upgrade UI/gateway together and configure the platform namespace. Drafts are local, not server backups. Real-service browser acceptance remains M3.12 and reset-email/pilot security M4. Focused Firefox acceptance is not WCAG certification or screen-reader/cross-browser coverage. |
| Boundaries | M4.3 (`8bc3619`) authenticates internal access and scopes worker capabilities. M4.4 (`da73a4d`) validates names/IDs and filesystem containment. M4.5 (`954ab9a`) bounds JSON, archives and compiler resources. M4.6 (`82c293f`) enforces exact origins/security headers. M4.7 (`9f3a6e0`) verifies cross-user isolation. M4.8 (`55a6074`) adds TLS reset email, atomic hashed-token consumption and recovery acceptance. | The control plane shares one credential and in-host HTTP; trusted volumes and runtime quotas remain necessary. Generated preview HTML is public. Reset does not immediately revoke issued JWTs. Configure/verify production SMTP, finish TLS and remaining M4 gates before remote release. |

The worker now has tested namespace/sandbox boundaries, an environment allowlist,
resource ceilings and integrated trusted-output validation. Instructions alone are
not a security boundary; internal-service authorization and combined deployment
hardening remain M4. The smoke-only budget gateway is not a production quota system.

## Initial assessment baseline (before M0 implementation)

Historical results from the assessment follow. M0 progress below records subsequent
fixes and passing verification; these initial failures are not the current status.

- `go test ./...` in gateway, manager, storage, worker, serving and compiler, using `GOCACHE=/tmp/pagewright-review-go-cache GOPROXY=off`: all passed. Compiler reports no test files; several worker and serving tests explicitly skip cases. These runs do not include integration-tagged tests or the race detector.
- Compiler sample: `go run ./cmd/pagewrightc build --theme ../themes/starter --content ./test-site/content --out /tmp/pagewright-review-compiler-output` from `pagewright/compiler`: passed, producing home, about and contact pages and assets.
- UI `npm run build`: passed using installed dependencies, with an unexpected `}` CSS minification warning. This is not a clean dependency-install verification.
- UI `npm run lint`: failed with 17 errors and 1 warning, covering explicit `any` types, auth context effects/exports, socket callback initialization and a hook dependency.
- `docker compose config --quiet`: valid, with missing Google and LLM environment warnings. `docker compose ps --format json` returned no project containers. The application stack and a paid AI request were not started.

No full browser journey or service integration run was performed. The existing `docker-verify-local-domain-strict` target writes placeholder HTML and manually reloads nginx; retain it as a hosting diagnostic, not the MVP acceptance test. Root integration targets also omit manager coverage and do not provision all prerequisites required by the service tests.

## Architecture decisions

See [MVP topology and principles](docs/adr/0000-architecture-decisions-mvp-topology.md)
and the [numbered decision index](docs/adr/README.md). Operational procedures live
in the [operations index](docs/README.md); this plan tracks milestones and evidence.

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

M1.1 completed (2026-09-05) in `ef544f9`. The [job contract](docs/adr/0001-architecture-decisions-job-contract.md)
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
See [submission state and retry rules](docs/adr/0002-architecture-decisions-build-submissions.md).

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
trailer after tar EOF. See [transport contract and limits](docs/adr/0003-architecture-decisions-artifact-transport.md).

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
See [metadata contract and compatibility boundaries](docs/adr/0004-architecture-decisions-version-metadata.md).

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
See [immutability, retry and filesystem limits](docs/adr/0005-architecture-decisions-immutable-versions.md).

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
See [bootstrap semantics and compatibility](docs/adr/0006-architecture-decisions-site-bootstrap.md).

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
[archive contract](docs/adr/0007-architecture-decisions-archive-layout.md) defines editable `content/`, optional
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
are tracked. The [compiler contract](docs/adr/0008-architecture-decisions-compiler-contract.md) and README describe
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
See [version/serving contract and remaining boundaries](docs/adr/0009-architecture-decisions-version-api.md).

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

M2.12 completed (2026-09-06) in `4f17a3c`. Selected worker is now
`pagewright-worker:m2.12`. [Provider smoke evidence](docs/PROVIDER_SMOKE.md) records
an actual manager-launched, sandboxed CLI edit using user-selected `gpt-5.6-luna`,
trusted compilation, fenced storage and terminal callback. Only the requested
source file changed; both source and compiled HTML contained the exact new heading,
and initial artifact bytes were unchanged. No browser publication was performed.

Optional model configuration is forwarded explicitly without changing the default
when unset. A separately invoked smoke gateway holds the real API key; the worker
receives a run-local token on an internal network. The guard pins the model/default
tier, limits output to 8,192 tokens and three requests, rejects hosted tools and
persists worst-case reservations before submission. Full-context reservations include
Luna's cache-write and long-context rates, are never refunded within a run, and
survive uncertainty; restarting the same journal is refused. No paid run is part
of normal tests or CI. Later invocations require new explicit authorization and
price review, including accounting for prior uncertain reservations.

The successful run used three requests, 21,572 input and 209 output tokens. Its
usage-derived model-token cost was **$0.00236171**, including reported cache writes.
Two earlier Codex Mini attempts each stopped on a 404 (confirmed model_not_found
despite model-list presence) and returned no usage. Conservatively retaining both
$0.116384 reservations puts the combined maximum reservation envelope at
**$1.8520048**, below the authorized $2. No GPT-5.4 Mini paid request was sent.
The amount calculated from usage is not an account invoice.

Acceptance passed: offline spending-guard tests and full installed-CLI smoke,
repository packages, full race-enabled service integration, manager/worker race and
vet, selected worker image build, installed CLI/compiler and real-Docker checks,
followed by the successful paid Luna test. Two unrelated serving skips remain.
Disposable services/networks/worker and temporary key files were removed; non-secret
evidence was retained. The user's `.env`, application data and isolation policy
were unchanged. No remote deployment or push occurred.

All M2 items are complete. At M2 handoff, next was **M3.1**, owner-scoped job/history retrieval and
browser-refresh recovery. Full UI preview/publish/rollback acceptance remains M3;
internal authorization, production quotas and pilot hardening remain M4.

M2.11 completed (2026-09-06) in `bb5f877`. Selected worker is now
`pagewright-worker:m2.11`. The [runner acceptance matrix](docs/RUNNER_ACCEPTANCE.md)
uses production-main subprocesses with a deterministic test-only CLI and the real
compiler. It verifies exact upload/callback order and exit codes across successful
compilation, instruction tampering, invalid compiled references, compiler errors,
artifact/log/manifest failures, callback retry exhaustion/lost acknowledgements,
SIGTERM and SIGKILL before/after manifest. A small context boundary permits a
short deadline test through the actual pipeline; production signal handling,
15-minute job deadline, 10-minute executor limit and 16-minute watchdog remain.
No runtime isolation bypass or adjustable production timeout was added.

Restart acceptance reconnects a fresh manager backend/reconciler to durable Redis
evidence. Complete materialization recovers completion; missing bytes fail
conservatively, preserving receipts and releasing terminal reservations. Existing
dispatch and root-stack restart checks verify no launch-intent replay and durable
history/uncertainty recovery. These are layered checks, not one end-to-end
paid-provider crash test or a disk-loss/HA guarantee. Workers are not restarted
to replay fenced attempts. Executor parsing/cancellation tests restored in M2.3
and M2.6 remain enabled; the obsolete skip inventory was corrected.

Acceptance passed: repository package suite, full race-enabled service integration
twice, final runner matrix repeated three times in a container without external
networking, worker/manager race and vet, selected production image build, installed
CLI/compiler checks, real-Docker launch/cleanup and isolated root-stack crash/restart
smoke. Two unrelated serving skips remain. Only disposable test resources were
removed; no paid calls, remote cleanup/deployment, privileged containers, isolation
relaxation or push occurred.

Next: **M2.12**, one explicitly invoked, cost-bounded provider smoke test verifying
the requested change in compiled HTML. M2 is not fully accepted until that gate;
browser publish/owner history and service authentication remain M3/M4. Retain the
coordinated upgrade and storage-writer drain requirements from M2.10.

M2.10 completed (2026-09-06) in `ddbb163`. Selected worker is now
`pagewright-worker:m2.10`; sandbox isolation and pinned CLI/compiler are unchanged.
The [retention runbook](docs/WORKER_RETENTION.md) documents one-hour terminal
container grace and seven-day operational diagnostics/staging retention. Cleanup
requires full Docker launch identity, unchanged canonical terminal evidence and
no active/site reservation. Diagnostics must be saved before kill/removal; running
terminal workers require a later fresh inspection before non-forced deletion.
Unknown, legacy or conflicting orphans remain quarantined for operator review.

Storage removes only old regular `.upload-*` files under an exclusive root lock;
all immutable writers hold a shared lock through publication. Passes have deletion,
entry and time limits. Published objects, durable identities and receipts remain
untouched. Reserved-but-unpublished staging bytes can expire after seven days;
this cannot reopen a terminal attempt. New private log records are bounded to
4 KiB and withhold all arbitrary executor output, while preserving site/job/version
correlation and output byte count. Historical immutable logs are not rewritten,
and per-version logs have no independent TTL because receipt verification needs
their bytes. This is not general artifact/content sanitization or a total-disk bound.

Acceptance passed: repository package suite, full race-enabled service integration
twice, manager/storage/worker race and vet, selected worker image build, installed
CLI/compiler checks, real-Docker identity/non-force removal and isolated root-stack
restart smoke. Coverage includes active-upload exclusion, final-file/symlink and
receipt preservation, terminal/reservation guards, diagnostic-storage failure,
retry TTL preservation and secret-sentinel omission. Two existing serving skips
remain. No paid calls, remote deployment/cleanup, privileged containers, isolation
relaxation or push occurred. Only disposable test resources were removed.

Next: **M2.11**, broader actual runner failure/restart acceptance; M2.12 remains
the explicitly cost-bounded provider run. Drain all storage writers before upgrade
because older writers lack the lock protocol. Never replace `.uploads.lock` during
operation. Busy writers, unavailable evidence and oversized trees may defer
cleanup; use backed-up offline maintenance when required, not broad pruning.

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
dispatch outcome. See [recovery semantics and limits](docs/adr/0016-architecture-decisions-result-recovery.md).

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
[commit semantics and upgrade requirements](docs/adr/0015-architecture-decisions-fenced-commits.md).

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
snapshot, not a queued-job rebase. See [build source semantics](docs/adr/0014-architecture-decisions-build-source.md).

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
acceptance remains M2.12. See [trusted build scope and limitations](docs/adr/0013-architecture-decisions-worker-build.md).

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
[CLI contract, provenance and host prerequisites](docs/adr/0012-architecture-decisions-worker-cli.md).

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
[dispatch contract and upgrade requirements](docs/adr/0011-architecture-decisions-queue-dispatch.md).

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
limitations](docs/adr/0010-architecture-decisions-docker-spawner.md). The manager now has host-level Docker socket
authority; this is not a remote-safe deployment or an AI sandbox. The worker still
contains the placeholder executor, its compiler is not integrated, callback/storage
credentials are not per-job tokens, and exited containers retain daemon metadata
until removal. Real AI/compiler integration, full resource isolation, callback
recovery and cleanup remain M2.3–M2.10; internal authentication remains M4.

Replace request-handler launching with a queue dispatcher with bounded concurrency. Build the selected real worker image with the compiler and trusted theme. Bootstrap/download source, execute a bounded edit, validate allowed changes, compile to `public/`, validate output, store the artifact and report completion. Add lease renewal, fencing enforcement, worker cleanup, callback retries and reconciliation for lost callbacks or manager restarts. Persist Redis data and reconcile gateway uncertainty; define safe retention for M1.2's nonexpiring reservations and legacy expiring jobs.

**Exit:** one real text request visibly changes a new site's HTML, two successive edits preserve each other, a concurrent same-site build is handled predictably, and worker failure/timeout/restart produces a recoverable terminal status without affecting live content.

### M3 — Complete browser journey and publishing (3–5 days)

M3.12 completed (2026-09-06) in `77b4f14`.
[Real-service browser acceptance](docs/BROWSER_ACCEPTANCE.md) records the runner,
provider boundary, prerequisites, evidence and cleanup. A uniquely named Compose
stack runs the production gateway (including background reconciliation), Redis
dispatcher, sandboxed installed CLI, compiler, storage, hosting supervisor/nginx
and built UI. Only model-provider responses are deterministic local fixtures;
the CLI's actual exec_command tool edits source, and all application mutations
come from browser actions. No SQL, manually uploaded HTML, manually launched jobs,
injected auth state or intercepted application APIs are used.

The browser registers and creates starter source, reloads an active build and
recovers its completion, activates preview before publication with live still
404, enables the initially disabled site through Dashboard and publishes through
version actions. It opens the real View Live link, submits a second source edit
which preserves the first, proves building alone changes neither hosted target,
and activates the new preview without changing live. The invalid-source third
build starts from the latest unpublished version and fails without a saved
completed version or hosted-content changes. Publishing version two and rolling
live back to version one leaves preview on version two. Reloads preserve exact
job outcomes, labels and dashboard pointers; both hosting links and compiled
theme assets resolve on the configured real live/preview hosts.

Workers/provider stay on an internal network; only browser-facing services have
an additional host-reachable network and random loopback ports. The runner skips
the private .env, uses dummy keys, rejects remote Docker contexts and cleans only
its own labelled workers, Compose containers and volumes. Isolation and the
production sandbox remain unchanged, with no privileged containers or fallback.
OpenAI Docs informed SSE framing for the narrow provider fixture; this does not
replace the separate explicitly authorized paid-provider M2.12 acceptance.

Verification passed: two clean full-journey runs plus the extended Enable/link
run, four provider-fixture cases, UI contracts, zero-warning lint, type checking/
production build, existing rendered draft-recovery regression, screenshot review
and JavaScript/whitespace checks. Tested cached Playwright 1.58.2 / Firefox 146.0.1
(revision 1509); final trace/screenshots: `/tmp/pagewright-browser-r2EDsz`.
Harness timeout/lock, Docker internal-network ports, starter-source expectations
and cache revalidation were corrected before acceptance. No production code or
schema changed; full Go suites were not rerun for this test-only addition.
Read-only post-run checks found no disposable containers, networks or volumes.
No provider spending, remote deployment, application-data changes or push occurred.

M3 is complete for the deterministic local browser journey. Hosted CI, production
DNS/TLS, other browser engines and model quality are not implied. Next: **M4.1**,
invitation/operator provisioning and bounded per-user/site usage before a remote
pilot. M4 remains a release gate, not authorization to expose this stack publicly.

M3.11 completed (2026-09-06) in `375def7`.
[Keyboard/focus and responsive acceptance](docs/UI_ACCESSIBILITY.md) records the
behavior, repeatable audit and verification limits. Version actions now use
labelled native modal dialogs with initial close-button focus, inert background,
keyboard containment, Escape/close/backdrop dismissal and opener restoration
(versions-heading fallback when needed). Cleanup restores body scrolling, and
closing pending work does not cancel a deployment or reopen the dismissed UI.

Skip/main landmarks, route focus and current-page navigation support keyboard
movement. Version buttons retain Enter/Space activation; the composer regains
focus after submission only when it would otherwise be lost to the body.
Shift+Enter remains a newline, while composition and repeat events cannot send.
History updates no longer scroll the whole page. A native disclosure starts
collapsed for mobile versions, keeping long histories from preceding the editor.
Layout changes remove constrained nested scroll areas, wrap long IDs/domains and
bound a single dialog scroller to the viewport. Explicit colors, 44px button
heights, visible-focus overrides and reduced-motion rules address base-CSS
conflicts and system color preferences.

Verification passed: UI contracts, zero-warning lint/type-check/production build,
rendered Firefox keyboard/reflow checks at 1280×900, 768×1024, 390×844, 320×568
and 640×450, screenshot inspection, the existing rendered draft-recovery regression
and final production Compose startup/nginx-crash/recreation smoke. Assertions
cover skip/route focus, disclosure activation, modal focus containment and inert
background, Escape/close restoration, pending dismissal, scroll cleanup,
IME/newline safety, target size, page/dialog overflow, dialog bounds, primary
text contrast, focus visibility and reduced motion with a dark system preference.
The audit caught a driver/browser mismatch, a mismatched synthetic version ID
and an overridden focus rule; all were corrected before final acceptance.
Cached Playwright 1.58.2 matches installed Firefox revision 1509; no browser or
application dependencies were installed.

No backend/schema/provider or hosting-policy changes were made. The full Go
suites were not rerun for this UI-only change. No paid calls, remote deployment,
application-data changes or push occurred; disposable stacks were removed.
This audit is not WCAG certification: screen-reader speech, real-device touch/
virtual keyboards, actual zoom, forced colors and other browser engines remain
unverified. The viewport tests and synthetic IME events do not imply those checks.

At M3.11 handoff, next was **M3.12**, the real-service browser journey, sequential edits, independent
preview/live state, refresh, build failure and rollback without manual HTML/DB seeding.

M3.10 completed (2026-09-06) in `5ad744d`.
[Draft recovery and publication feedback](docs/adr/0023-architecture-decisions-draft-recovery.md) defines the
account/site-scoped, current-tab storage contract. Text changes are saved
synchronously; exact payload fingerprints and request keys are persisted before
dispatch. Reload or same-account re-authentication restores uncertain submissions
for explicit same-key retry, without auto-submitting. Clarification questions,
original text and answers survive browser reload; confirmed responses clear the
sent draft or store the next clarification.

Protected-request expiry remembers an allowlisted local return route, while
failed login credentials stay on the form and late 401s from replaced tokens are
ignored. Account changes remount protected UI, and the client checks draft
ownership immediately before dispatch without sending the local guard marker.
Late/unmounted build responses do not rewrite another account's state. Explicit
logout clears this tab's drafts; expiry preserves them. Invalid/unavailable storage
is reported, blocks sending before dispatch and supports retrying reads or an
explicit discard. Existing in-memory gateway clarification context is not made
durable: after its loss, restarting with saved original/answer text is an explicit
new-request action, with no silent truncation or automatic duplicate work.

Chat now exposes submission/uncertainty/storage feedback. Version lists offer
retry/refresh, older/newer pagination, timestamps, exact IDs and independent
confirmed Live/Preview labels. Publishing has visible progress and actionable
failure messages; success and uncertainty both refresh the hosting snapshot.
Dashboard reads confirmed state on entry, focus/visibility return, manual refresh
and after enable/disable attempts. Duplicate toggles are guarded, stale data is
identified after failed reads, and local booleans are not speculatively flipped.
Read requests are bounded to 10 seconds and build/deploy/toggle requests to 30;
timeouts do not imply server cancellation. Read cleanup and mounted/account checks
prevent stale callbacks from replacing current UI state.

Verification passed: UI contracts, zero-warning lint/type-check/production build,
focused rendered Firefox regression with an intercepted gateway, and final
production Compose startup/nginx-crash/recreation smoke. Browser tests exercise
draft reload, expiry, bad credentials, same-account return, exact-key uncertain
retries, account isolation, clarification recovery, publishing failures/success,
toggle failures/success, focus refresh, version retry, storage failure/repair and
explicit logout. Pure tests cover malformed storage, safe return routes,
stale-token handling, deletion scope and independent version labels. The installed
Firefox harness uses its native viewport for protocol compatibility. No browser
dependencies were installed. No backend/schema changes or full Go-suite rerun
were needed for this UI-only change; no paid provider calls, remote changes,
application-data migration or push occurred. Disposable stacks were removed.

At M3.10 handoff, next was **M3.11**, full keyboard/focus and mobile/desktop usability verification.
The mocked rendered regression does not complete M3.12's real-service browser
journey. Tab-local storage is not encrypted backup, cross-device persistence or
server-side conversation history.

M3.9 completed (2026-09-06) in `d61a2e2`.
[Text-only MVP capabilities](docs/adr/0022-architecture-decisions-mvp-capabilities.md) documents the release boundary,
API behavior and legacy-site upgrade path. This release is MVP-only, without a
switch that enables unaccepted features. The gateway's `PAGEWRIGHT_SITE_DOMAIN`
(default `pagewright.dev`, local-domain override `pagewright.io`) is authoritative.
Public, no-store capabilities include the platform domain and disabled feature
flags; OPTIONS support permits the cross-origin UI's authorization/JSON preflight.
The old Vite default-domain configuration has been removed.

Creation validates one DNS label under the configured domain, reserves app/api/www/
preview, and rejects arbitrary or nested domains before database/bootstrap writes.
The UI loads configuration with timeout/retry and fails closed without a fallback.
Resume Setup locks the original name; incompatible legacy names display an
operator-recovery message rather than creating a replacement. Existing sites,
aliases, artifacts, owner-scoped builds and deployments are preserved. This
does not provision DNS/certificates or verify arbitrary domain ownership.

Chat now explicitly supports text only, retaining clarification and idempotent
submission semantics. Attachment components, file state, multipart submission and
alias UI/client paths are removed. Gateway rejects explicit non-JSON build media
types with 415 and unknown attachment fields with 400 before downstream calls.
Legacy JSON callers without Content-Type remain compatible. Both Google endpoints
and all alias actions return no-store 501 without inspecting provider credentials
or writing state. All site deletion is conservatively unavailable until coordinated
DB/storage/serving cleanup exists; lower-level deployment-record and active-output
guards are retained, and no application data was deleted or migrated.

Verification passed: six-module package baseline, gateway race/vet, full isolated
race-enabled integration, UI contracts/zero-warning lint/build, and final production
Compose startup/nginx-crash/recreation smoke. Tests exercise invalid namespace
bypasses, nil-dependency rejection before side effects, OAuth without redirects/
cookies, text-only identity, locked resume policy, capabilities preflight and
disabled direct API calls while preserving bootstrap/build/preview/publish/rollback.
Verification caught and corrected a leftover attachment-state reference; final
smoke includes the OPTIONS route added during review. One existing serving skip
remains (`TestLoadConfigDefaults`). UI checks cover contracts/source wiring and
builds, not rendered browser acceptance. Disposable stacks/data were removed;
no paid provider calls, remote changes or push occurred.

At M3.9 handoff, next was **M3.10**, preserve drafts across session expiry/re-authentication and improve
progress, retry/empty states, version labels and dashboard refresh.

M3.8 completed (2026-09-06) in `4466271`.
[Atomic activation and retention guide](docs/adr/0021-architecture-decisions-atomic-activation.md) defines the
filesystem and upgrade contract. Compiled output is validated/extracted privately,
synced and renamed into its immutable cache directory before activation. Serving
creates a temporary symlink on the destination filesystem and renames it over only
the selected public/preview pointer, then syncs the site directory. It no longer
unlinks active output first. Invalid replacement paths and unrecognized pointers
fail before cutover; errors after rename retain M3.7's uncertainty semantics rather
than assuming rollback. Explicit rollback uses a newer fenced deployment sequence.

The artifact writer mutex serializes deployment, activation, retention and deletion.
Retention parses exact canonical live/preview targets and the shared M3.7 receipt
schema; malformed/unreadable evidence or path indirection prevents deletion. Active
and receipt-pinned versions plus one minute of newly created/selected/retired output
are protected. Directory timestamps provide grace without altering artifact bytes.
The configured maximum is a soft total cache budget; protected entries may exceed
it, and remaining entries are ordered deterministically by timestamp/name. Private
staging entries are untouched. Only serving-cache copies are removed; canonical
storage and version history remain available for restaging an evicted rollback.

Fenced deployments now run guarded retention after terminal receipt persistence;
cleanup failure is deferred maintenance, not a false activation failure. Whole-site
removal checks active pointers and receipt evidence before its nginx-removal callback
under the writer lock. A legacy active site therefore keeps both routing and output
when deletion is refused. M3.7's enrolled-site/tombstone guard remains in place;
coordinated deletion is not implemented, and M3.9 should hide unsupported controls.

Verification passed: six-module Go baseline, final serving race tests/vet, full
race-enabled service integration, production startup/nginx-crash/recreation smoke
and whitespace checks. The previously skipped cleanup test is now deterministic
and passing. Added tests cover continuous complete-file reads through repeated
switches/cleanup, forced lock overlap, rename failure preserving old output,
post-rename uncertainty followed by recreated-manager rollback, exact versus
substring pins, receipt protection, staging preservation, grace-period overflow,
stable ties, symlink/regular-file rejection and deletion guards preceding routing
removal. Serving tests evict an inactive cached version, re-download it through a
newer fenced rollback, preserve preview and reject delayed older requests. The real
compiler/storage/nginx journey still passes promotion, rollback and lost-ack recovery.

One pre-existing serving skip remains (`TestLoadConfigDefaults`). Evidence:
`/tmp/pagewright-m38-final-integration.log`, `/tmp/pagewright-m38-unit.log`,
`/tmp/pagewright-m38-smoke.log`. No schema or UI source changes; production smoke
builds the images. No paid calls, remote deployment, application-data changes or
push occurred; disposable test stacks were removed.

Drain deployments before upgrading serving; preserve compatible gateway/receipts
and the single-writer volume lock. Existing inactive cache entries can become
eligible on the next cleanup, so review the soft cache limit first. Atomic rename
does not provide a multi-request browser snapshot, and the grace period is not a
request-lifetime lease. Hidden crash-left staging directories remain for operator
inspection. Full rendered-browser acceptance remains M3.12; arbitrary disk loss,
coordinated backup/restore and security remain M4 work.

At M3.8 handoff, next was **M3.9**, hide/disable unsupported MVP controls and backend actions.

M3.7 completed (2026-09-06) in `ec47934`.
[Deployment consistency/recovery runbook](docs/adr/0020-architecture-decisions-deployment-recovery.md) defines the
protocol. Migration 010 adds a bounded current deployment record per site and a
monotonically increasing PostgreSQL sequence. Site-row reservation admits only one
unresolved operation across live/preview and gateway instances; identical pending
requests share identity, competing selections return 409. Ownership, identifiers,
target and configured URL are validated before intent persistence/external writes.

Serving serializes deployment work and atomically writes/syncs a private receipt
before side effects. It stages the immutable artifact and confirms existing-policy
nginx routing, persists `activating` before the symlink change, verifies the selected
link, then persists completion. Pre-activation failures are durable terminal failures;
interrupted activation remains uncertain and retries only the exact artifact.
Same-sequence conflicts, delayed older requests, corrupt receipts and mismatched
completed pointers fail closed. Completed replay does not download/reactivate.
The M3.5 single-serving-writer volume lock remains a prerequisite.

Gateway accepts only an identity-matching bounded terminal receipt. A conditional
transaction saves its status and only its selected site pointer; DB failure rolls
both back, retaining the pending intent. Startup/five-second recovery scans bounded
oldest-first batches with per-attempt deadlines and never invents a rollback or new
selection. An owner-authenticated no-store status endpoint exposes the current
operation. Detailed UI recovery presentation remains M3.10.

Legacy artifact/activation/deletion calls cannot bypass an enrolled site's fence.
Enrollment and never-enrolled deletion are serialized; serving deletion failure no
longer silently proceeds to DB deletion. Enrolled-site deletion is refused even
after terminal failure, preserving sequence evidence against delayed requests.
The new route does not invoke unsafe automatic artifact retention. Safe active-aware
retention/atomic symlinks remain M3.8; unsupported deletion controls belong in M3.9.

Verification passed: final six-module Go baseline, gateway/serving race tests/vet,
full race-enabled service integration, root Compose startup/nginx-crash/recreation
smoke and whitespace checks. Tests include concurrent reservations and replay,
opposite-pointer preservation, actual PostgreSQL pointer-write failure/transaction
rollback, stale completion rejection, owner-only status and deletion guards.
Serving tests recreate handlers over persisted receipts, model a crash after the
pointer switched, retain uncertainty through an outage, and reject corrupt/stale
evidence. The real worker/compiler/storage/hosting journey loses the serving
acknowledgment after actual activation, confirms DB uncertainty, then starts a fresh
gateway recovery instance and verifies agreement without changing preview.

Initial verification corrected an absent gateway test dependency and migration-count
expectations. Integration also reproduced the documented nginx worker-drain window;
public-resource tests now bound convergence while retaining exact artifact-byte
checks and immediate private-path 404 assertions. Two existing serving skips remain
(`TestCleanupOldVersions`, `TestLoadConfigDefaults`). Evidence:
`/tmp/pagewright-m37-verified-integration.log`, `/tmp/pagewright-m37-unit-final.log`,
`/tmp/pagewright-m37-smoke.log`. UI source unchanged; root smoke rebuilt its image.

Upgrade gateway/serving together after draining deployment traffic. Retain PostgreSQL
sequence state and serving receipts; do not mix old unfenced gateways or independently
restore volumes. Legacy pre-M3.7 mismatches and arbitrary evidence loss require
operator review; coordinated backup/restore acceptance remains M4.11. No paid calls,
remote deployment, application-data changes or push occurred; disposable stacks
were removed. This is retry/restart reconciliation, not globally atomic publishing.

At M3.7 handoff, next was **M3.8**, atomic artifact-pointer replacement, active-version retention and rollback tests.

M3.6 completed (2026-09-06) in `830c476`.
[Hosting URLs and preview upgrade guidance](docs/adr/0018-architecture-decisions-preview-activation.md) define
shared gateway scheme/port URL generation for site create/list/detail and deployment
responses. Dashboard and Chat validate/use these destinations, disable unavailable
links, and Chat refreshes hosting state after successful preview/publish operations.

Preview is now `preview.<site-fqdn>/`, a separate nginx virtual host selecting the
preview artifact's public output. No HTML rewriting or recompilation is involved;
root-relative and relative navigation, nested pages, theme assets and page-local
images remain on their selected host. Relative directory redirects preserve the
external scheme/port. Promotion selects the exact same immutable artifact.
Live aliases do not become preview aliases. The first `preview` DNS label is
reserved for preview hosts; new site/alias collisions and existing conflicting
nginx host claims fail before config mutation. Existing exact generated legacy
configs migrate transactionally on Preview activation, preserving aliases and
disabled state; unknown/custom legacy configs require operator review.

Verification passed: six-module Go baseline, gateway/serving race tests and vet,
full race-enabled service integration, UI contract tests/zero-warning lint/build,
production root-stack hosting/nginx-crash/recreation smoke, script syntax and
whitespace checks. Real compiler output includes a nested page and page image;
the journey resolves generated links on both hosts, checks directory redirects,
all public bytes and private/missing-path 404s, previews before publication, keeps
live independent of a later preview, then promotes and compares every public file
on both hosts with the unchanged second archive. URL configuration/identity,
namespace rejection and enabled/disabled legacy migration have focused tests.
The existing real-nginx validation/rollback acceptance also passes.

Earlier verification attempts exposed test-fixture issues (duplicate variable,
compiler asset path and changed-file manifest expectations), all corrected before
the passing integration run. Root smoke also observed briefly stale routing while
old nginx workers drain; fresh-connection bounded convergence checks now cover
both hosts and pass, without claiming instantaneous global cutover. Two pre-existing
serving skips remain (`TestCleanupOldVersions`, `TestLoadConfigDefaults`).
Evidence: `/tmp/pagewright-m36-verified-integration.log`,
`/tmp/pagewright-m36-unit.log`, `/tmp/pagewright-m36-smoke-final.log`.

Upgrade gateway/serving/UI together; DNS and HTTPS certificates must cover both
live and preview names. Existing configs are not rewritten automatically at startup:
reactivate Preview after auditing reserved-name conflicts/custom configs. No remote
deployment, paid calls, application-data changes or push occurred; disposable test
stacks were removed. Preview remains public, not an authenticated private workspace.
Distributed deployment reconciliation remains M3.7, atomic artifact-pointer changes
M3.8, and full rendered-browser acceptance M3.12.

At M3.6 handoff, next was **M3.7**, reconcile partial activation and define concurrent deployment policy.

M3.5 completed (2026-09-06) in `2e1eea2`.
[Supervised hosting lifecycle](docs/adr/0019-architecture-decisions-hosting-lifecycle.md) defines the production
topology: the serving API and hosting nginx share a Tini/Go-supervised container;
the existing public nginx is a fixed proxy preserving Host and re-resolving Docker
DNS after replacement. Either supervised child's exit terminates its sibling;
Compose restarts the pair. Readiness checks nginx as well as configuration state.
No Docker-daemon socket, privileged container or host PID access was added.

An exclusive writer lock and manager mutex protect config transactions. Prior
bytes/existence and generation state are journaled/synced before atomic file
replacement. Validation precedes reload; new nginx workers must return the exact
loopback-only generation token before success. Failure restores and confirms the
old generation; uncertain rollback retains its journal and rejects writes. Startup
restores interrupted changes before validating nginx. Routing confirmation now
precedes artifact pointer changes, avoiding pointer changes on reload failure.
Artifact symlink atomicity and DB/serving reconciliation remain M3.8 and M3.7.

Verification passed: six-module Go baseline, serving race/vet, full race-enabled
service integration with the production supervisor/config/proxy, real-nginx invalid
syntax and no-op reload rollback, and root-stack proxy/routing, nginx-master crash,
container restart and abrupt recreation checks. Writer exclusion and conservative
restart recovery have focused tests. The first root smoke failed because Node fetch
ignored its custom Host header; a local diagnostic confirmed this and the corrected
HTTP client passed subsequent runs. Script syntax, Compose overlay configuration
and whitespace checks passed. Two existing serving skips remain. UI source was
unchanged; root acceptance rebuilt the production UI image and checked its bundle.

Serving and proxy must be upgraded together after backups; existing volumes remain
in place. The local-domain no-op reload override was removed. Disposable test stacks
were removed; private `.env` and application data were untouched. No paid calls,
remote deployment or push occurred. Full rendered-browser acceptance remains M3.12.

At M3.5 handoff, next was **M3.6**, configured URLs across UI entry points and preview assets/navigation.

M3.4 completed (2026-09-06) in `e22362d`.
[Preview activation contract](docs/adr/0018-architecture-decisions-preview-activation.md) documents staging,
activation, first-host routing, checked DB state and returned URL sequencing.
The version modal calls the preview deployment API, validates its response and
opens only after confirmation; pending actions are guarded, errors remain visible,
and a normal link handles blocked popups. Closing the modal suppresses late tab
opening, not the server operation.

Serving ensures missing nginx routing exists on first preview without replacing
existing aliases or disabled-site configuration. The real worker/compiler/storage/
nginx integration proves compiled HTML is available in preview before first publish,
live remains 404, and a later preview leaves published HTML and the live pointer
unchanged. Narrow M3.6/M3.7 prerequisites were necessary: explicit public scheme/
port configuration, nil-as-unchanged DB pointer updates, and checked DB write errors.
Other UI entry-point URLs/assets and distributed activation reconciliation remain
in those milestones; no partial-activation rollback claim is made.

Verification passed: two full race-enabled isolated service integration runs,
six-module Go package baseline, gateway/serving race/vet, UI contracts/polling tests,
zero-warning lint, TypeScript and production build. Additional tests cover owner
denial, artifact-before-activation ordering, failure responses without success URLs,
safe URL validation, modal closure, popup fallback, routing policy preservation and
reload errors. Two existing serving skips remain. Disposable integration services
were removed; application data and private `.env` were untouched. No paid calls,
remote deployment or push occurred.

At M3.4 handoff, next was **M3.5**, production serving/nginx lifecycle. M3.4's integration co-locates
nginx and serving; root Compose still requires reload coordination before production
preview can succeed. Preview assets and remaining configured URLs are M3.6, durable
reconciliation/atomic operations M3.7/M3.8, rendered-browser acceptance M3.12.

M3.3 completed (2026-09-06) in `7f99456`.
[WebSocket retirement and re-enable gate](docs/adr/0017-architecture-decisions-job-history-api.md) documents the
polling-only MVP. Removed the browser connection hook, reconnect timer, query-token
URL, socket configuration/build arguments, gateway upgrader/broadcast hub/pumps and
unused Go dependency. Removed code is retained in Git history, not behind a flag.
The public `/ws` retirement handler returns no-store HTTP 501 without inspecting
or echoing credentials; generic CORS OPTIONS preflight remains 200. M3.2 polling
and gateway recovery/dispatch semantics are unchanged. Rebuild/reload older UI
clients; private `.env` values were not edited and obsolete socket settings are ignored.

Verification passed: six-module Go package baseline, gateway race/vet, UI contracts
and polling tests, zero-warning lint/type checking/build, script syntax/whitespace
checks and isolated production root-stack startup/recreation. Handshake tests cover
credentials and same/foreign/missing origins. The rebuilt served UI bundle contains
no socket transport, and the production endpoint returns 501 before and after
container recreation. Source/config regression guards run in existing CI checks.
Two existing serving skips remain. Disposable test services/volumes were removed;
application data was untouched. No paid calls, remote deployment or push occurred.
Full rendered-browser acceptance remains M3.12; the full service integration suite
was not rerun for this transport-removal milestone.

At M3.3 handoff, next was **M3.4**, make Preview activate deployment before opening its returned URL.

M3.2 completed (2026-09-06) in `b1af7d6`.
[Polling contract](docs/adr/0017-architecture-decisions-job-history-api.md) defines sequential checks of active jobs
on the current history page, with 2/4/8/15-second backoff, 10-second transport
timeouts, a 60-round/15-minute budget and a five-consecutive-error limit. Terminal
pages stop; 401/403/404 pause immediately. Manual refresh resumes checks. Other
pages are checked when opened; builds submitted elsewhere require a history refresh
for discovery. There is no indefinite idle-page discovery loop.

The UI validates site/job/source/target identities, ignores stale/regressive results,
retains saved states and safe diagnostic codes during outages, and never converts
polling errors into build failures. Failed builds show useful code explanations and
job IDs for operators. Completion refreshes the version sidebar independently of
history polling. Site-keyed Chat isolates navigation; new submissions reset history
to page one. Cleanup aborts requests, cancels timers and ignores late responses.
Unscoped socket events no longer update Chat; the unused connection remains M3.3.

Verification passed: UI contracts and deterministic polling tests covering lifecycle
transitions, fresh-instance recovery, timer/round/elapsed bounds, transient and access
errors, retained failures, identity filtering, monotonicity, non-overlap and cleanup;
zero-warning lint, TypeScript checks and production build. The polling tests run in
the existing CI contract-test command. No backend code changed, so Go/Docker suites
were not rerun for this UI-only milestone. Actual rendered-browser acceptance remains
M3.12. No paid calls, remote changes, application-data changes or push occurred.

At M3.2 handoff, next was **M3.3**, remove the broken/unused WebSocket connection for the polling MVP.

M3.1 completed (2026-09-06) in `38e908e`.
[Owner-scoped history contract](docs/adr/0017-architecture-decisions-job-history-api.md) documents the authenticated
single-job and paginated site-history endpoints, public allowlisted fields and
diagnostic codes, and last-observed-state semantics. Count and rows share a
read-only PostgreSQL snapshot; reads never contact upstream services or enqueue
work. Both HTTP authorization and SQL owner/site scoping are tested.

Chat now loads durable build history on mounting/refresh, separately from its
ephemeral conversation. It supports older/newer pages and manual retry, ignores
responses after navigation/unmount, and reloads after submissions. Pending,
running, completed, failed and uncertain dispatch states survive loss of browser
or gateway memory. This does not persist clarification transcripts or input drafts.

Version listings exclude known submissions until their durable status is completed,
even when storage has materialized their artifacts. Completed manager observations
still need committed storage before appearing; late artifacts cannot revive failed
jobs. Existing M2.9 background recovery remains the identity-verifying transaction
writer. No migration, deletion, redispatch, source-selection or deployment-policy
change was introduced. Legacy artifacts without submissions retain existing behavior.

Verification passed: six-module package baseline, full race-enabled isolated service
integration, gateway race/vet, UI contracts, lint and production build. New tests
exercise fresh-handler reads for all lifecycle states, cross-owner/cross-site
denial, upstream-independent retrieval, pagination, private-field exclusion and
late-terminal non-regression. The deterministic worker round trip verifies version
visibility before/after authoritative manager observation. The first integration
attempt exposed a new test-fixture response-format error; the corrected run passed.
Two existing serving skips remain. Disposable services/data were removed; no paid
calls, remote deployment, application-data migration or push occurred.

At M3.1 handoff, next was **M3.2**, bounded active-job polling. M3.1's UI checks cover contracts and
build wiring, not a real browser journey (M3.12). The existing socket remains
unchanged pending M3.3, and drafts/session-expiry recovery remains M3.10.

Connect persisted job status to chat and version history; normalize timestamps/statuses. Fix preview activation and URL construction, first-preview nginx provisioning, atomic promotion/rollback, pointer preservation and deployment error reconciliation. Preserve user input across auth expiry and report actionable errors. Hide attachments, arbitrary domains and unsupported authentication choices. Verify dashboard/chat/modal use at desktop and mobile sizes.

**Exit:** through the UI only, a tester creates a site, edits, reloads to recover status, previews, publishes, edits again, and rolls back. Preview leaves live unchanged, all pages/assets resolve, and failed deployment leaves the last working version served.

### M4 — Controlled remote pilot (3–5 days)

M4.13 completed local release acceptance (2026-09-11), implementation `337da9f`;
final authorized paid evidence `b4dbc92`. The
[release evidence matrix](docs/RELEASE_ACCEPTANCE.md) maps all nine scenarios to
commands, actual service/browser checks, test-double boundaries and remaining gates.
Fresh package/race, UI contracts/lint/build, compiler, five-service integration,
SIGKILL/recreation smoke, source-volume-destruction backup restore, Docker-spawner
and installed sandboxed CLI/compiler acceptance passed. A new integration test
joins real TLS/STARTTLS SMTP delivery to PostgreSQL reset handlers: it extracts the
secret only from the received email, consumes it once, rejects reuse, and verifies
old-password rejection/new-password login. No external recipient was contacted.

The old provider smoke referenced an M2.12 worker tag and omitted current service
credentials. It now builds a unique production worker from this checkout, gives
host fixture calls an independent origin-scoped service token, and requires a
scoped worker token without exposing the signing secret. Remote Docker contexts
are refused before credentials/resources, with a no-network preflight regression
test. Updated offline smoke passed with the actual sandboxed CLI/compiler and
synthetic model responses. OpenAI Docs/model pricing was rechecked on 2026-09-11;
the existing three-request $1.6192368 reservation envelope was unchanged. After
the user authorized a fresh $2 total cap, one invocation on candidate `02bb2c6`
passed using the configured key only through the isolated budget gateway. Three
requests retained $1.6192368 in conservative reservations; returned usage gave a
$0.00241242 model-token cost upper bound, not an invoice. The actual sandboxed CLI
changed the requested source heading, trusted compilation produced matching HTML,
initial bytes remained unchanged, and scoped credentials/isolation passed. Evidence:
`/tmp/pagewright-provider-smoke-6pt8lu`, job `ca76d708-e592-474d-8a21-8c79f057b4e6`,
observed `2026-09-11T19:23:28.194Z`. Seven spending/preflight tests passed again.
No second paid invocation was needed. Exact-project/job cleanup queries confirmed
no remaining containers/networks; the temporary key was removed and `.env` unchanged.

Firefox build 1509 completed the edit/publish/rollback and two-account phases but
timed out in the generated-page origin check. A small local navigation diagnostic
also behaved inconsistently; its exact cause is unresolved. An explicit matching
Chromium option preserves all application/security assertions and uncovered a real
cache defect: the browser reused old HTML after a preview switch. Firefox's
cache-disabled harness had masked it. The generated-content edge now sets
`Cache-Control: no-store` on pages/assets/errors, hides upstream validators/cache
policy and strips conditional validators before proxying. Mutable live/preview
URLs no longer invite browser freshness reuse. This trades cache efficiency for
correct publication; pre-existing fresh cache entries may require hard refresh.
No application security headers, worker capabilities or isolation were weakened.

Final post-fix Chromium acceptance with normal caching passed source inheritance,
active-build refresh, first preview before live, independent pointers/assets,
expected third-build failure, publish/rollback, two-user site/job/version/download/
deployment isolation, actual generated-page CSP enforcement, direct internal API/
Redis and scoped-worker misuse rejection, closed signup, operator-provisioned UI
login and zero-AI rejection. Evidence: `/tmp/pagewright-browser-FrDJQX`. Final
full race-enabled integration passed delivered-email and conditional-hosting
regressions plus readiness degradation/recovery. Focused rendered expiry/re-auth,
retry/reset and five-viewport accessibility checks, seven spending/preflight tests,
four browser-provider tests, focused vet, syntax and whitespace checks also passed.
The failed Firefox/initial Chromium traces remain documented alongside the success.

Existing app/API/live/preview pilot hosts passed read-only trusted HTTPS, TLS-ready,
security-header and exact-origin checks; no deployment or remote fault injection
was done. The cache fix is not installed on the pilot: a reviewed edge rollout is
still required. All generated test stacks/workers/data were removed, with private
fixture evidence and image/browser caches retained. No remote configuration/CA
changes, real email or push occurred. M4.9's public failure-recovery/full-browser
gates stay open. M4.13's local scenario acceptance is complete, not public release
sign-off. Next: M4.14 operator-runbook/release-state work.

M4.12 completed (2026-09-11), implementation `395a95f`: serving skip resolution
and meaningful state/concurrency acceptance. The remaining serving defaults test
was skipped because it depended on ambient environment and expected an obsolete
empty storage URL. It now checks the actual localhost default and uses `t.Setenv`
to isolate and restore every configuration input; custom configuration is likewise
restored safely. Existing cleanup tests already cover real archives, live/preview
pins, deterministic retention, pending stages, fail-closed receipts and concurrent
publication/deletion. No serving test skips remain.

New gateway tests interleave 320 conversation creations/reads across 32 goroutines,
assert prompt identity and cross-owner/site denial before provider access, and
retain context after two failed retries. The provider is synthetic and in-process;
no paid calls occur. Worker tests interleave status mutations, JSON snapshots,
cancel callback replacement and 800 cancellation requests, requiring coherent
step/progress fields and allowing the callback to reenter status mutation without
deadlocking. Direct real-Redis lock tests require exactly one of 32 concurrent
acquirers to win, a monotonic successor fence, and rejection of concurrent stale
release/renew attempts while the successor renews. Cleanup targets only unique
test lease/fence keys, never a database flush. Existing job reservation/replay,
queue claim, terminal-state and durable dispatch/fencing tests also ran under race
instrumentation. WebSockets remain retired: the actual `/ws` route uses the tested
501/no-upgrade/no-credential-reflection handler, not an enabled shared-state hub.

Verification passed: six-module `make test-all`; new `make test-race-state` with
three uncached race-enabled runs and bounded test timeouts; full disposable
`make test-integration` (five service test suites with `-race`, PostgreSQL/Redis,
real service APIs and dependency failure/recovery probes); ten shuffled config
runs with nondefault ambient values; focused vet; actionlint and whitespace checks.
The new local race target is wired into the Go CI job; hosted CI has not been run.
The race detector instruments test binaries and their in-process code, not the
separate service images. Distributed lease assertions complement race detection;
neither proves all possible schedules or conversation-cache expiry/persistence.
See [test commands and coverage boundaries](docs/TESTING.md#stateconcurrency-acceptance-m412).

The generated integration project `pagewright-test-1789151913-244787` was removed
with its containers/network and ephemeral data. No production code changes were
needed. No worker-isolation changes, remote pilot actions, private environment
changes, paid provider requests or push occurred. M4.9's remaining public security/
failure-recovery and full-browser gates remain open. Next implementation item:
**M4.13**, the consolidated release-scenario acceptance run.

M4.11 completed (2026-09-11), implementation `64ef049`: coordinated offline
backup and fresh-target recovery. See the [operator runbook](docs/BACKUP_RESTORE.md).
The trusted local Docker operator CLI requires an acknowledged maintenance window,
idle PostgreSQL/Redis work and stopped application writers. It dumps PostgreSQL
and preserves the entire cold Redis data directory together with immutable
artifacts, published trees/live-preview symlinks, private deployment receipts,
generated Nginx and serving configuration. A private completion manifest records
checksums, sizes, source volumes, exact service image IDs and an operator-attested
revision; it is not an authentication signature or an encryption mechanism.

Restore validates the whole bundle before contacting its target, requires a
different explicit project and exact images/revision, and rejects shared source
volumes, existing database objects and nonempty data volumes. There is no force,
clean or in-place restore. The recovery overlay removes public ports and the
manager socket, blocks gateway/manager startup and disables Docker volume copy-up.
Archive helpers are short-lived, networkless, resource-bounded and unprivileged
containers with only necessary file capabilities; they are reaped on failure
without deleting volumes. Partial restores remain stopped for inspection and must
be retried into another fresh project. Worker isolation is unchanged.

Verification passed: 11 Python guard tests, final `make test-backup-restore`,
six-module `make test-all`, fixture `go vet`, actionlint and whitespace checks.
The drill published explicit synthetic v1/v2 artifacts through real deployment
APIs, backed up state, removed the original test volumes, and restored into a
second disposable project. It checked owners/password hashes, all three initial/
v1/v2 database and storage versions, cross-owner denial, exact artifact checksums,
private serving receipts, allowance reservations and Redis job/commit/fence state.
Real edge HTTP responses retained distinct live/preview HTML, CSS, JS and nested
pages; a subsequent deployment advanced the sequence. A second restore correctly
refused populated state. The actual recovery overlay's no-port/no-dispatch/no-socket
and nocopy guards also passed. The new drill is wired into CI; hosted CI has not
been run by this change.

During development, Docker's default copy-up correctly triggered the nonempty
target guard; fresh nocopy mounts fixed setup without weakening restore safety.
The extended fixture initially omitted the bootstrap version from its expected
database count; the final check requires all three distinct identities and passed.
Final private fixture evidence: `/tmp/pagewright-backup-drill-3ca4znbe`. Generated
test containers/volumes were removed; the fixture backup remains available locally.
No real user data, private environment files, paid provider calls, remote pilot
changes or push were involved. This is maintenance-window recovery, not live
backup/PITR/HA or a production restore. Encrypted off-host retention, separate host
secrets/TLS recovery, old-host fencing and post-snapshot spending reconciliation
remain explicit operator responsibilities. No remote backup schedule is installed.
M4.9's remaining public security/failure-recovery and full-browser gates stay open.
Next implementation item: **M4.12**.

M4.10 completed (2026-09-11), implementation `ab8306a`: bounded runtime work and
dependency readiness. Gateway foreground database and provider/storage/manager/
serving calls inherit request cancellation, including streamed artifact bodies.
Request-local views preserve shared pools and HTTP client deadlines. Public
gateway work has a 55s context budget, its two possible provider calls each have
a 25s HTTP limit, and the browser build request allows 60s. Internal request and
transport budgets are explicit; PostgreSQL caps open/idle connections at 10/5,
and manager Redis clients have bounded pools/I/O with automatic retries disabled.
Worker execution limits, isolation, credentials and provider reservations are
unchanged. See [runtime limits and readiness](docs/RUNTIME_LIMITS.md).

Cancellation never supplies a rollback receipt. Durable job identities and
deployment recovery remain authoritative; interrupted serving preparation keeps
pending/activating intent and cannot replace a partial downloaded destination.
Confirmed site-toggle persistence and failed password-reset token invalidation
use bounded five-second cleanup contexts even after a disconnect. Existing
accepted-build outcome persistence retains its detached deadline.

Anonymous exact `GET /ready` reports only generic, non-cacheable readiness within
two seconds: gateway checks PostgreSQL and internal services; manager checks Redis
and its Docker daemon; storage checks its artifact directory; serving checks
directories, storage, local Nginx and absence of a pending Nginx recovery journal.
The graph is acyclic and makes no paid provider calls. Compose health checks use
readiness; `/health` is liveness. Directory checks do not claim free space or
writability, and readiness is not an end-to-end publication/sandbox/TLS test.

Verification passed: `make test-all`; final race-enabled `make test-integration`;
focused gateway and serving race tests; `go vet ./...` in all four changed Go
service modules; 48 UI contract tests, lint and production build; shared auth/
runtime conformance, shell syntax and whitespace checks. Integration proves pool
wait/query cancellation, reset-token cleanup after cancellation, pending intent
preservation, and readiness degradation/recovery under real disposable Redis and
storage outages while dependent liveness remains 200. Tests also preserve the
Nginx recovery-journal guard and show Docker readiness only pings the daemon.
During validation, cancellation fixtures were corrected to consume POST bodies
and to separate a five-second setup wait from the unchanged one-second cancellation
assertion. Repeated local/container race checks and the final full suite passed.

The uniquely named test stacks were removed; existing application data was not
used. No paid provider requests, key reads, remote changes or push occurred.
These settings require rebuilt/recreated services before they apply to the pilot.
M4.9's remaining public security/failure-recovery and full-browser gates remain
open; M4.10 does not close them. Next implementation item: **M4.11**.

M4.9 subsequent public acceptance and offline artifact tool (2026-09-10),
implementation `2e7201d`; **still not complete**. Dedicated Certbot passed
restricted startup, production app/API issuance, restricted reconciliation,
staging renewal simulation and explicit Nginx validation/reload. Both pilot
timers are enabled/scheduled. The operator created `m49-check.pagewright.io`;
its live/preview certificate became ready. An authorized real AI build completed,
was previewed and published. Public HTML/CSS/JS over trusted HTTPS passed;
preview initially left live 404. The operator confirmed HTTP-to-HTTPS redirect,
dashboard persistence and sign-out/sign-in recovery. Read-only checks confirmed
the apex GitHub redirect and blog response at the known host IP (one local apex
DNS lookup failed, so this does not claim that lookup passed).

The approved $10 reservation cap has 900 cents reserved and zero active provider
slots. Actual provider billing was not measured. No allowance increase, reset or
additional provider request was made for the offline work. The new offline tools
compile a fixed source edit using the existing trusted compiler/static checks,
then explicitly import an operator artifact through immutable manifest-last
storage routines. No AI job, callback, lease, fence, browser result or gateway
build-history entry is fabricated; no new production endpoint or worker bypass
was introduced. See [operator trust boundary](docs/OFFLINE_ACCEPTANCE.md).

Worker/compiler/artifact and importer/storage race tests plus vet passed. A real
network-disabled non-root compiler run first rejected an incorrect local asset
reference; nothing was imported. The corrected run passed all four static gates.
Import and identical re-import passed on a disposable copy of the committed base
before importing only `operator-m49-v2-aqicfd2p` into test site
`50dbf226-5975-4cde-b5e3-1a01d5bb7dd0`. Base version
`669e8078-3483-44d6-a2f9-084f349c52c9` stayed byte-identical. Both deployment pointers
and public headings still select version 1; the version-2 asset remains 404 on
both hosts before activation. Bundle evidence is retained privately on the host
at `/home/codex/pagewright-offline-AQiCFD2p/work/bundle-fixed`. This is an operator
import, not a second end-to-end AI/manager build. Production service images,
container isolation, secrets, signup, DNS, TLS configuration and live content
were not changed by this tool run.

Subsequent operator browser acceptance passed: version 2 preview and its asset,
live remaining on version 1, version 2 promotion to live, then version 1 rollback
while preview retains version 2. Independent read-only HTTPS checks confirmed
the final distinct headings, the version-2 asset returning 404 on live and its
expected content on preview. PostgreSQL confirms live
`669e8078-3483-44d6-a2f9-084f349c52c9` and preview `operator-m49-v2-aqicfd2p`.
Reservations remain 900 cents with zero active provider slots. Intermediate
version-2 live rendering was confirmed by the operator, not independently
observed by the agent. These manual browser checks do not claim a successful
rerun of the previously failing full automated browser suite. Remaining:
public security/failure-recovery and full-browser acceptance gaps. M4.9 remains
unchecked; no push.

M4.9 remote progress and Certbot compatibility fix (2026-09-09), `e62f568`:
the operator installed the reviewed pilot, prepared root-only application secrets
with AI allowance zero, and started the loopback-only stack successfully. The
operator confirmed the worker AppArmor profile in enforce mode, Nginx validation,
and no conflicting app/API host declarations. Let's Encrypt staging issued the
app/API certificate. Production issuance failed before Certbot logging; a
version-only probe under identical service restrictions reproduced Snap's
inability to create `/root/snap/certbot/5893`. Public HTTPS is still pending.
The fix selects `/opt/pagewright-certbot/bin/certbot` explicitly for both issuance
and renewal, without altering service restrictions or the host's Snap/cron.
Twelve local controller tests and whitespace checks passed. Dedicated Certbot
installation, restricted on-host execution, production issuance, renewal and
public/full-browser acceptance remain unverified. No further ACME attempts or
service activation were performed by the compatibility change. M4.9 stays open.
The historical no-remote-write/no-CA statements below describe the original
implementation session, not the subsequent operator deployment.

M4.9 HTTP-01 implementation (2026-09-09) in `6892ced`; **not complete**.
The operator explicitly selected individual Let's Encrypt certificates with HTTP
validation, without Namecheap API access or DNS delegation. This supersedes the
earlier wildcard certificate proposal below. The current
[HTTP-01 runbook](docs/PILOT_HTTP01.md) provides reviewed installation, staging,
production issuance, renewal, acceptance and narrowly scoped rollback steps.

The host-root controller reads initialized FQDNs from the fixed local pilot
PostgreSQL container, accepts only canonical platform site names, and never issues
from arbitrary incoming Host headers or aliases. One certificate covers app/api;
one per site covers its live and preview names. Host-owned challenge files and
dedicated staging/production Certbot directories stay outside application containers
and the existing apex/blog lineage. Persistent attempt limits and mutual exclusion
bound retries. Atomic Nginx replacement, validation/reload, rollback journals and
real trusted HTTPS/vhost probes gate a ten-minute readiness lease. Failed recovery
blocks writes; expiry/missing state withholds links and returns 503/Retry-After
before fresh deployment mutations. UI renders HTTPS provisioning guidance.

The pilot overlay uses exact HTTPS app/API/hosting addresses, closed signup and
loopback-only published ports; only the non-secret readiness directory is mounted
read-only into gateway. No worker sandbox relaxation, certificate keys, host Nginx
control or new Docker socket access is given to application containers. Existing
gateway source-IP throttling remains conservative behind the proxy (shared
unauthenticated bucket); broader proxy trust was not silently enabled. The dedicated
renewal timer uses the controller lock and validates Nginx before reload; it does
not modify the existing Certbot cron. Installation/state paths are outside
Server-Tools site discovery, and shared-vhost conflict review remains mandatory.

Verification passed: ten controller tests (including real OpenSSL name checks,
persisted backoff, rollback/recovery, read-only default and both-host probe gating);
real-nginx TLS with local test certificates (routing, challenge isolation, headers,
pending/unknown handshake rejection, separate apex/blog fixture preservation);
dummy-only Compose port/mount/configuration checks; systemd verification with
installation permissions; six-module package baseline; final gateway race/vet;
full isolated race-enabled integration including pending TLS denial before upstream
effects; UI contracts/lint/build and rendered provisioning/session/reset recovery.
One pre-existing serving `TestLoadConfigDefaults` skip remains.

Full-browser acceptance remains unresolved. Repeated pinned Firefox 146.0.1 runs
timed out at different hosted-page navigations. A separate tiny local HTML server
reproduced receipt of `200 text/html` followed by an unchanged `about:blank` tab,
without PageWright services. Fresh contexts/processes and a response/DOM helper
did not resolve the suite. All ineffective harness changes were removed; no
security headers or baseline assertions were weakened. Do not report the full
browser journey as passed for this implementation. Evidence includes
`/tmp/pagewright-m49-http-browser.log`, `-browser-final.log`, `-browser-isolated.log`,
`-browser-verified.log`, `-browser-processes.log` (same prefix), and
`/tmp/pagewright-browser-JsViiA`. Passing logs include
`/tmp/pagewright-m49-http-packages.log`,
`/tmp/pagewright-m49-http-integration-final.log` and
`/tmp/pagewright-m49-http-draft.log`.

Read-only server preflight found Python 3.13.5, OpenSSL 3.5.5, Certbot 5.8.0,
Nginx 1.26.3 and Compose 5.1.3, the expected conf.d include, free loopback upstream
ports, and absent pilot checkout/state/config targets. No remote files/services
were changed. Pending operator gates: reviewed file transfer/installation, private
pilot environment and ACME email, staging issuance, explicitly approved production
issuance/deployment, renewal dry-run/hook checks, shared-host coexistence and public
HTTPS/full-browser acceptance. No CA requests, paid calls, private .env reads/edits
or push occurred; disposable test stacks/data were removed. M4.9 remains open and
remaining M4 gates still block admitting remote testers.

Earlier M4.9 namespace progress (2026-09-09) in `8745082`; **not complete**.
The wildcard certificate proposal in this historical entry is superseded above.
The operator approved moving previews from `preview.<site>.pagewright.io` to
`<site>.preview.pagewright.io`. Gateway deployment/site responses, UI destination
validation, generated nginx hosts, collision checks and browser/integration
fixtures now share this layout. Live URLs and artifact/deployment identities are
unchanged. Exact generated legacy/v2 nginx files migrate to v3 transactionally on
Preview activation, preserving paths, live aliases and enabled/disabled state.
Custom old configurations are rejected without replacement; reload failure uses
the existing rollback/recovery protocol. Old preview bookmarks are not redirected.
Rebuild gateway/serving/UI together and reactivate previews before admitting users;
startup does not automatically rewrite saved configs. Worker isolation is unchanged.

[Pilot DNS/TLS runbook](docs/adr/0027-architecture-decisions-pilot-https.md) records local equivalents, host coexistence,
the operator-owned apex/www Certbot lineage and remaining production gates. The
operator updated DNS; both authoritative servers resolve app/api and arbitrary live
names to `135.181.209.167`, and the approved new preview layout was also verified.
Keep an explicit `*.preview` record so ACME validation nodes do not break inherited
wildcard resolution. Proposed dedicated pilot certificate names are
`*.pagewright.io` and `*.preview.pagewright.io`; the existing apex/www certificate
does not cover them. DNS provider/API capability, automated DNS-01 issuance and
renewal, separate Nginx integration compatible with Server-Tools, loopback-only
pilot upstreams, proxy/throttling review and public HTTPS acceptance remain open.
Do not regenerate shared Server-Tools sites or alter the blog/apex redirect.
Custom domain creation and alias mutation remain disabled.

Verification passed: six-module Go package baseline; gateway/serving race/vet;
final full isolated race-enabled integration including compiler/storage/actual
nginx roundtrip, migration and reload recovery; UI contracts/lint/build; JS syntax
and whitespace checks; startup/recovery smoke; and full production browser journey
with sequential edits, preview/live independence, failed build, rollback,
ownership/origin/internal-access/operator-login probes. One pre-existing
`TestLoadConfigDefaults` skip remains. The first package/integration runs exposed
a new test retrying after both forward and rollback reload failures without
supervisor recovery; corrected the test, retained the safeguard, and reran
successfully. Browser evidence: `/tmp/pagewright-browser-krNEws`; logs:
`/tmp/pagewright-m49-packages.log`, `/tmp/pagewright-m49-integration-final.log`,
`/tmp/pagewright-m49-browser.log`, `/tmp/pagewright-m49-smoke.log`.
Disposable stacks/data were removed. No paid calls, private .env reads/changes,
agent DNS mutations, certificate issuance, remote deployment or push occurred.
Next: finish M4.9 operator/TLS gates; remaining M4 gates still block remote testers.

M4.8 completed locally (2026-09-08) in `55a6074`.
[Password reset](docs/PASSWORD_RESET.md) records SMTP setup, upgrade requirements,
reset semantics, session recovery and remaining operator acceptance. SMTP supports
mandatory STARTTLS and implicit TLS, certificate/name verification, optional paired
credentials, an eight-second deadline and cancellation; it never falls back to
plaintext. The reset URL is a fixed application-allowlisted `/reset-password` URL,
HTTPS except explicit loopback development. New links use fragments and the UI
scrubs the current history entry after capturing the token in memory. Request Host
and forwarding headers never select the destination. Empty SMTP settings explicitly
disable reset with 503; partial/malformed settings stop startup before DB work.

The gateway generates 256-bit random reset secrets and stores only SHA-256 digests.
An account row lock serializes consumers; token use, password update and sibling
invalidation commit together. Expiry uses database wall-clock time at consumption,
and periodic cleanup removes expired rows. Ordinary password changes invalidate
pending reset links too. SMTP failure invalidates its token and emits only a fixed
operational message. Configured forgot-password responses do not distinguish
eligible/missing/passwordless accounts or delivery failure. Per-account database
minute-bucket throttling supplements existing auth/IP/global limits. JSON/no-store
responses and generic used/expired/invalid errors replace inconsistent responses.

Registration, operator CLI, reset and password change consistently require at least
eight Unicode characters and at most 72 UTF-8 bytes; UI validation matches. Existing
passwords remain usable for login. This preserves the MVP minimum, not a claim of
a stronger password standard. Reset does not immediately revoke existing JWTs;
their configured lifetime (default 15 minutes) still applies. Actual expired JWTs
are rejected before handlers. Rendered recovery verifies same-owner draft/retry/
clarification preservation after expiry and login, without automatic resubmission;
server-side builds remain independent of the browser session.

Verification passed: six-module package baseline; gateway race/vet and final focused
SMTP/startup/session tests; full isolated race-enabled integration including final
forced-update rollback; UI contracts/lint/build; configuration/whitespace checks;
startup/recovery smoke; rendered reset and draft-recovery regression; and the full
production browser journey plus ownership/origin/internal-access/operator-login
probes. SMTP wire tests use actual local verified TLS/STARTTLS dialogues and reject
untrusted certificates/plaintext. Handler integration separately captures sender
calls and proves hashed storage, throttling, exactly one winner among eight
concurrent reset requests, expiry/replay, sibling invalidation, delivery failure,
old-password rejection and new-password login. A forced DB failure proves password
and token rollback together. One pre-existing serving configuration skip remains.
Final evidence: `/tmp/pagewright-browser-MT19Yq`,
`/tmp/pagewright-m48-recovery-browser.log`,
`/tmp/pagewright-m48-integration-final.log`, `/tmp/pagewright-m48-smoke.log`.

Rebuild gateway/UI; worker image and schema remain unchanged. Previously issued
plaintext UUID links are intentionally unusable after upgrade; request a new link.
SMTP is synchronous, without a durable retry/outbox; DATA acknowledgement is relay
acceptance, not proof of inbox delivery. Timing is not constant across account
existence. Production SMTP service/credentials, verified sender and real inbox
acceptance are still operator setup before remote testers; the service/sender choice
was requested, not assumed. No real recipient or provider was contacted. No paid
calls, private .env reads/changes, DNS changes, remote deployment or push occurred.
Disposable stacks/data were removed; local evidence/build caches remain. Next:
**M4.9**. Remaining M4 gates still block admitting remote testers.

M4.7 completed (2026-09-08) in `9f3a6e0`.
[Ownership security](docs/adr/0026-architecture-decisions-ownership-security.md) records the endpoint matrix,
regression coverage and public-preview distinction. The audit found existing
owner checks before active site operations and database filtering by both current
site owner and submission owner before public job polling responses. There is no
browser event broadcaster to retrofit: `/ws` remains retired and never upgrades.
Future event delivery must retain owner/site filtering before sending data.
No runtime authorization defect or production behavior change was required.

The new PostgreSQL integration matrix uses two real accounts and JWT middleware
to check anonymous/foreign access to site detail, enable/disable, builds, job
history/detail, versions, downloads, deployment status and live/preview activation.
Both directions are covered; disabled deletion/aliases return 501 for authenticated
accounts and 401 anonymously. Upstream traps assert zero storage/serving/manager
calls after denial. Complete site/job rows remain unchanged and no foreign
deployment intent is persisted. Site-list totals/rows and job IDs are owner/site
scoped; legitimate owner polling still works without private prompt disclosure.

Real-stack browser acceptance logs in as the journey owner, registers a second
synthetic account and creates its starter site through public APIs. It verifies
foreign rejection, empty/owned-only lists, cross-site job/version substitution,
successful owner artifact download and unchanged owner site/history/version/
deployment snapshots. Missing versions under the caller's own site return the
existing 500 response and may leave a failed own-site deployment intent; they cannot
read foreign artifacts or move live/preview pointers. Deletion stays disabled,
not enabled merely to test it. Generated live and preview HTML are public; private
management endpoints do not provide confidential previews.

Verification passed: six-module package baseline, full isolated race-enabled
integration (including a final rerun with complete job-row equality), gateway vet,
JavaScript syntax/whitespace checks and the complete browser build/edit/refresh/
preview/publish/failure/rollback journey with ownership, origin/CSP, internal-access
and closed-signup/operator-login/disabled-AI probes. One pre-existing serving
configuration skip remains for M4.12. The first browser run stopped on a new test
expecting null instead of omitted empty pointer fields; the corrected assertion and
full rerun passed without application changes. Final browser evidence:
`/tmp/pagewright-browser-pk9mXB`; logs:
`/tmp/pagewright-m47-browser-final.log` and
`/tmp/pagewright-m47-integration-final.log`.

No service/worker rebuild, schema migration, paid provider call, private .env or DNS
change, remote deployment or push occurred. Disposable stacks/data were removed;
local evidence/build caches remain. This is regression acceptance of supported
routes, not a general penetration test or a proof against stolen credentials,
operator compromise or future ownership-transfer races. Next: **M4.8**. Remaining
M4 gates still block admitting remote testers.

M4.6 completed (2026-09-07) in `82c293f`.
[Origin security](docs/ORIGIN_SECURITY.md) records configuration, upgrade steps,
header policies and remaining browser trust boundaries. Gateway validates
`PAGEWRIGHT_APP_ORIGINS` before DB initialization: explicit HTTP(S) origins only,
with generated/preview namespace overlap rejected except reserved infrastructure
hosts. The outer policy covers the entire router, including errors and retired
sockets, and rejects foreign, opaque, empty and duplicate origins before handler
effects. Exact allowed origins receive non-credentialed CORS/preflight responses.
No-Origin CLI requests retain normal authentication; Origin is not identity.
WebSockets remain disabled (501 for allowed/no origin; 403 for denied origins).

The UI build validates its API origin before generating nginx CSP directives.
Self-only application scripts and exact API connections are separate from the
generated-content edge's local/inline script policy and same-origin connections.
Generated pages cannot embed frames, submit forms or fetch the application API
under this policy. UI/edge headers include embedding denial, nosniff, no-referrer,
opener isolation, origin-agent clustering and denied device permissions. The edge
replaces upstream headers, including for old managed configs and errors; UI asset
and health locations preserve headers despite nginx inheritance rules.

Verification passed: package baseline, gateway race/vet (including final focused
startup/middleware race tests), full isolated race-enabled integration, UI
contracts/lint/production build, Compose/auth-copy checks, recovery smoke and full
real-service browser acceptance. The browser verifies build/edit/refresh,
preview/live independence, expected failed build and rollback, exact CORS and
retired sockets, headers on assets/errors, allowed UI API access and an actual
generated-page `connect-src` violation. Internal credential-misuse probes and
operator-provisioned login/closed signup/disabled-AI checks also pass. One
pre-existing serving configuration skip remains. Final browser evidence:
`/tmp/pagewright-browser-LGiAG3`; final log:
`/tmp/pagewright-m46-browser-final-acceptance.log`.

Initial browser reruns hit Firefox lifecycle wait timeouts despite successful
page/asset responses. Hosted checks now use response/DOM and explicit rendered
readiness; independent phases use fresh browser processes. A new ambiguous heading
selector was corrected to assert the expected journey heading. The final complete
run passed with cached Playwright 1.58.2 and Firefox 146.0.1 (revision 1509), without
weakening any security policy. The harness pins the API port across recreation and
configures the exact disposable UI origin; provider spend was $0.

Rebuild gateway/UI together and reload the hosting edge; retain the existing
`m4.5` worker image. Audit legacy host/alias collisions before upgrading; there is
no automatic data migration. Generated HTML/JS remain active untrusted content,
and sibling origins can share a cookie site: parent-domain session cookies and
document.domain relaxation remain unsupported. TLS/HSTS and production DNS are
still M4.9 gates. No private .env reads/changes, paid calls, DNS/redirect changes,
remote deployment or push occurred. Disposable stacks/data were removed; local
evidence/build caches remain. Next: **M4.7**. Remaining M4 gates still block remote
testers.

M4.5 completed (2026-09-07) in `954ab9a`.
[Resource limits](docs/adr/0025-architecture-decisions-resource-limits.md) records the fixed MVP budgets,
publication behavior, verification and upgrade requirements. General JSON handlers
now consume the entire bounded representation before strict decoding, rejecting
unknown fields, additional values and oversized whitespace suffixes without trusting
Content-Length. The general ceiling is 1 MiB; existing 4 KiB endpoint and 4 MiB
private-metadata limits remain in force. Auth/build, manager callbacks and legacy
serving/storage log requests use the bounded decoder.

Storage applies the 64 MiB archive upload cap inside the handler, including bootstrap
and direct-handler use, with 413 on overflow and no immutable publication. Gateway
artifact streams and version-list responses are bounded; worker gzip packing stops
when its compressed-output budget is exhausted. Worker/serving retain identical
validated extraction policy: 256 MiB expansion including headers/padding/trailers,
10,000 entries, 32 MiB individual files and 64 KiB archive metadata. Archive inputs
must be regular files. Existing canonical-path, duplicate/conflict, link/type and
hidden-payload rejection remains intact, including source-only bootstrap support.

Compiler content/theme trees and final output have entry/total/per-file budgets.
Site/theme JSON reads are limited to 64 KiB before allocation/parsing. Template,
Markdown and component buffers, assembled pages and atomic output writes stop at
32 MiB. Growing page output is checked during compilation and the final stage is
validated before publication. Failed writes preserve prior destinations and discard
temporary output. Asset copies remain bounded by validated source trees; temporary
staging can exceed the final-output allowance. These budgets supplement existing
worker memory/CPU/disk/process limits and timeouts rather than replacing them.

Verification passed: six-module package baseline; six-module race/vet; full isolated
race-enabled service integration; Compose/auth-copy/archive-policy consistency;
fresh startup and abrupt restart/recreation smoke; and the full browser journey
through build, refresh, preview, publish, failure and rollback, plus internal-access
misuse probes and operator login. Browser evidence: `/tmp/pagewright-browser-sirbIv`.
Focused final worker tests also pass for device rejection and compressed-output
boundaries. Tests exercise malformed JSON/metadata, oversized complete bodies and
suffixes, archive bombs/count limits, traversal, links/devices/FIFOs, compiler/asset
escapes, sparse oversized trees and preservation/cleanup after failed writes.
One pre-existing serving configuration skip remains; no acceptance rerun was needed
for a functional failure in this milestone.

Rebuild `pagewright-worker:m4.5`, update explicit image pins and upgrade services
together. Review oversized legacy inputs; do not truncate or rename stored data.
Private quiescent service-owned volumes, operator trust and sandbox isolation remain
assumptions. HTML/JS are active generated content, not sanitized application code;
origin isolation/security headers remain M4.6. No private .env reads/changes, paid
calls, schema/data migrations, DNS changes, remote deployment or push occurred.
Disposable stacks/data were removed; local evidence/build caches remain.
Next: **M4.6**. Remaining M4 gates still block admitting remote testers.

M4.4 completed (2026-09-07) in `da73a4d`.
[Identifier security](docs/adr/0024-architecture-decisions-identifier-security.md) records canonical naming,
reserved labels, bounded identities, filesystem assumptions and upgrade notes.
The user confirms ownership of `pagewright.io`. The apex redirect and live DNS
are unchanged; M4.9 will configure the chosen owned namespace and both live and
preview routing/certificates. Local defaults remain unchanged, and neither domain
ownership verification nor TLS/origin isolation is claimed by this milestone.

Gateway creation retains lowercase/trim normalization and exactly one DNS label
below the configured namespace. Infrastructure names and internationalized site
labels are rejected, with matching UI checks. Serving/nginx independently require
canonical lowercase DNS labels, a dotted hostname and room for the preview prefix;
they reject malformed labels, directive injection and reserved nginx filenames.
Syntactically valid legacy domains remain available to the authenticated control
plane without automatic renaming or assuming ownership from a hostname alone.

Opaque site/version IDs are limited to 200 ASCII characters, leaving room for
artifact extensions and timestamped event filenames. Manager rejects invalid IDs
before queue reservation; workers validate launch identities before work; transport
clients, storage handlers/backend and serving operations enforce the bound.
Versions remain case-sensitive, and `initial` retains its bootstrap semantics.
Nginx-interpolated paths must be canonical safe absolute paths; hosting startup
rejects invalid configuration before initializing files. Storage checks containment
and existing symlinks before reads, mkdirs and immutable writes. Serving checks real
ancestors before receipt access and rejects nonregular receipts. Legacy artifact
downloads use unique temporary files instead of request-derived filenames.

Verification passed: six-module package baseline; five-service race/vet;
isolated race-enabled service integration; UI contracts, lint and production build;
Compose configuration/auth-copy checks; fresh startup and abrupt restart/recreation
smoke; and full browser build/refresh/preview/publish/failure/rollback, internal
credential-misuse probes and operator login. Final browser evidence:
`/tmp/pagewright-browser-pYE06Z`. New rejection tests prove no queue/dependency
effects, outside-file modification or config/receipt creation for invalid input.
One pre-existing serving configuration skip remains. Initial tests exposed the
intentional new identifier boundary and a malformed-URL test fixture; both were
updated. An initial integration slashless-redirect assertion returned 404; a full
rerun passed without changing routing, so its transient cause remains unconfirmed.
The first browser run completed deployment/rollback but raced independent history
and version fetches in its final assertion. An explicit version-list wait fixed
the harness, and the full rerun passed. Focused final serving/worker race tests pass.

Upgrade services together, rebuild `pagewright-worker:m4.4`, update old explicit
image pins and review any legacy IDs longer than 200 bytes. Do not rename identities
or delete volumes as an upgrade shortcut. These checks assume private operator-owned
volumes and no concurrent external mutation; they do not replace hostile-local-user
race-proof filesystem APIs. Archive/type/size/compiler containment remains M4.5.
No private .env reads/changes, paid calls, schema migration, domain routing changes,
remote deployment or push occurred. Disposable stacks/data were removed; local
traces/build caches remain. Next: **M4.5**. Remaining M4 gates still block testers.

M4.3 completed (2026-09-07) in `8bc3619`.
[Internal authentication](docs/INTERNAL_AUTH.md) records trust boundaries, worker
capabilities, required configuration, coordinated rotation and upgrade steps.
Supported root Compose now publishes only gateway, UI and hosting nginx. Database,
Redis, manager, storage, serving API, themes and optional worker ports are private;
the disposable smoke overlay uses random loopback-only mappings. Redis requires a
separate password without changing its AOF durability or resetting queue data.

Gateway, manager, storage and serving validate an explicit service credential at
startup. Production internal handlers authenticate reads and writes except exact
GET health checks. Internal clients pin credentials to their configured origin
and retain redirect refusal. These four services form one trusted control plane:
this is not per-service authorization or mTLS. Identical dependency-free auth
implementations/tests are checked for drift in package and integration workflows.

Each Docker worker receives a signed 17-minute capability binding job, site, source,
target, lock and fence. It permits only its source archive read, target artifact
writes and own-job callback/readback; it cannot bootstrap sites, enumerate versions,
read private metadata, invoke serving or authorize write commits. Verified identity
must match callback bodies and artifact attempt headers, and existing authoritative
fencing/terminal checks remain. Stale attempts cannot read replacement lock details.
Source reads may remain available until expiry; writes remain fenced. Worker status
binds container loopback. Workers never receive the master or Redis password;
sandbox isolation, resource limits and unprivileged operation remain unchanged.
The separate budget-limited provider credential remains shared, not job-scoped.

Verification passed: six-module package baseline; gateway/manager/storage/serving/
worker race and vet; full isolated race-enabled service integration; negative
configuration and auth-copy checks; authenticated startup/crash/recreation smoke;
and the full real-service browser journey including operator login. Direct probes
using actual disposable worker capabilities reject anonymous internal APIs/Redis,
cross-job/cross-version misuse, tampering, bootstrap writes and worker control-plane
operations, while permitting own-job/source reads. Browser evidence:
`/tmp/pagewright-browser-7MIiaX`. One pre-existing serving configuration skip remains.
An old integration callback fixture was updated to authenticate; the rerun passed.
An earlier smoke run overlapped new required Redis configuration, failed and was
explicitly cleaned up; the final complete smoke passed. Final storage tests also
verify that a signed capability cannot substitute job, lock or fence headers.

Disposable stacks/data were removed. No paid calls, private .env reads, schema
changes, live credential rotations, remote deployment or push occurred. Existing
deployments must configure distinct service/Redis secrets, rebuild the `m4.3` worker
image and replace old explicit image pins; drain workers for coordinated master
rotation and preserve volumes. In-host HTTP and Docker-administrator trust remain
explicit boundaries. Next: **M4.4**, identifier validation and path containment.
Remaining M4 gates still prohibit admitting remote testers.

M4.2 completed (2026-09-07) in `544901a`.
[Configuration security](docs/CONFIGURATION_SECURITY.md) records required secrets,
single-source PostgreSQL wiring, existing-volume rotation and log boundaries.
Root and legacy gateway-only Compose reject missing database/JWT secrets. The
gateway no longer supplies a fallback signing key and rejects invalid/missing
critical settings before database access or migrations. Validation errors name
keys/categories without reproducing private values. Required service URLs cannot
contain user credentials; invalid port/lifetime strings cannot silently default.

Root PostgreSQL and gateway receive one password with fixed bundled database/user
identity. URL encoding occurs in Go, preserving reserved punctuation. Root Compose
no longer honors a separate `PAGEWRIGHT_DATABASE_URL` override; intentional external
database deployments require a reviewed topology override. Direct processes retain
explicit PostgreSQL URL support with validated credentials and sslmode, without
query-level connection/credential overrides. No live configuration was changed.

Routine logs no longer reveal reset tokens/accounts, legacy worker prompts,
Kubernetes-stub job environments or callback URLs. Gateway/operator connection and
database errors and manager initialization/reconciliation withhold private raw
diagnostics. Private manifests/execution logs remain diagnostic records and must
not be published; this does not erase historic secret exposure or implement reset
email delivery. The legacy Kubernetes implementation remains an unsupported stub.

Verification passed: six-module package baseline; gateway/manager race and vet;
full isolated race-enabled integration, including actual reset-token creation with
log/response leak checks; entrypoint subprocess rejection before database I/O;
read-only root/legacy Compose missing-secret and credential-wiring checks; URL
punctuation/query-override tests; and startup/crash/recreation smoke using an actual
punctuation-containing PostgreSQL password. The full real-service browser journey
also passed: two sequential sandboxed source edits, preview/live independence,
failed build, rollback, closed registration, operator provisioning/login and
disabled-AI guidance. Final browser evidence: `/tmp/pagewright-browser-njSlhF`.
One pre-existing serving configuration test remains skipped. The Compose test
fixture was adjusted for configuration rendering's dollar escaping; actual URL
punctuation is independently exercised against PostgreSQL in startup smoke.

No private .env reads, paid provider calls, schema changes, existing-volume edits,
credential rotations, remote deployment or push occurred. Disposable containers,
networks and volumes were removed; traces/build caches remain local. Before an
existing deployment upgrades, back up and coordinate database-role password and
JWT rotation: changing environment variables alone does not rotate an initialized
PostgreSQL role, and a new JWT secret invalidates sessions. Do not delete volumes
as a migration shortcut. Next: **M4.3**, internal exposure/authentication and scoped
callbacks. Remaining M4 gates still prohibit a remote pilot release.

M4.1 completed (2026-09-07) in `cbb5380`.
[Pilot limits and provisioning](docs/PILOT_LIMITS.md) records configuration,
conservative charging semantics, pricing sources, upgrade and fail-closed recovery.
Operator provisioning is the selected access model: public signup defaults to
403, the registration UI explains operator access, and the existing account CLI
supports password stdin. Explicit development signup is only for disposable tests.

Migration 011 adds PostgreSQL admission, rate-counter and provider-reservation
tables. Admission precedes clarification/instruction calls and defaults to ten
attempts per owner/day, five per site/day, one active build per owner and two
globally. Failed/unclear attempts count. Exact retries reuse their attempt and
committed submission recovery remains available without fresh quota or credit.
Preparing and uncertain durable work continues occupying capacity after restart.
Fixed-minute throttles cover global, peer-IP/auth and authenticated-owner traffic;
forwarding headers cannot select a new peer bucket. Database failures fail closed.

Both gateway LLM calls and the pinned worker CLI use an internal, unpublished
gateway provider listener. Only the gateway receives the real key. Workers use a
separate internal token with reviewed text models, bounded output/request/response
sizes and no hosted tools or remote input/history. The pinned CLI's deferred
`additional_tools` definitions are validated recursively as local tools. Every
upstream attempt permanently reserves 100 cents before network I/O against the
configured lifetime allowance, default zero. Increasing the total grants only the
increment; failures/restarts never refund it. Verified terminal responses release
concurrency; uncertain or failed outcomes require operator review. OpenAI Docs
informed the conservative pricing envelope; this is not actual billed usage or a
cap on unrelated key use. Review prices before enabling/replenishing allowance.

Verification passed:

- `make test-all`: all six Go modules; one pre-existing serving configuration skip.
- Gateway `go test -race ./...` and `go vet ./...`, plus final focused HTTP retry/
  allowance-error regressions. Admission rejection cannot reach provider/dispatch.
- `make test-integration`: full isolated race-enabled service suites, including
  concurrent quota/budget/rate admission, restart persistence and no-refund checks.
- UI contracts, zero-warning lint and production build; ten offline provider tests;
  existing rendered draft-expiry/re-authentication/publishing regression.
- `make smoke-stack`: production startup, crash/recreation, durable history and
  missing-evidence recovery. Updated its capabilities expectation for signup mode.
- Full real-service browser journey through the new proxy: two actual sandboxed
  source edits, independent preview/live targets, failed build and rollback.
  Gateway restart into closed mode, operator CLI provisioning, real UI login and
  disabled-AI 429 guidance also passed. Final evidence:
  `/tmp/pagewright-browser-pLbBlZ` (cached Playwright 1.58.2 / Firefox revision 1509).

During verification, corrected PostgreSQL UUID/text parameter typing, compatibility
with the pinned CLI's local deferred definitions, and an ambiguous browser error
selector. Temporary fixture diagnostics were removed before acceptance/commit.
Disposable containers/networks/volumes were removed; test evidence/build caches
remain local. No paid provider calls, private-key reads, remote-host changes or
push occurred. Existing application volumes/accounts were not migrated by tests.

M4.1 does not close the remote-pilot release gate. Shared internal credentials
are not job-scoped, internal APIs still need authorization/private ports, and
origins/TLS/reset-email/backups remain outstanding. Orphan preparing attempts and
uncertain provider slots deliberately need operator recovery; all replicas must
share the same policy. Next: **M4.2**. Do not expose this stack to remote testers yet.

Carry boundary protections into M1/M2 as those interfaces are implemented; complete this gate before remote access. Add explicit origin policies, internal request authentication, per-job callback credentials, request/archive limits, path and hostname validation, worker isolation, per-user usage limits and secret validation. Disable open signup or require invitations. Deliver reset email without logging tokens; make tokens single-use and handle session expiry. Configure HTTPS for app and hosted sites, private infrastructure ports, persistent Redis state, backups, restore instructions and useful correlated error logs.

**Exit:** two accounts cannot access each other's sites, jobs, artifacts or updates; unauthenticated callers cannot mutate internal services; invalid/oversized archives and paths fail safely; a backup restore and bounded AI failure test succeed. All MVP release checks below pass.

Dependency order: **M0 → M1 → M2 → M3 → M4**. Security work belongs alongside each relevant change, with M4 verifying the combined deployment. Rough total: **14–24 focused development days**, subject to M1 findings and the real CLI integration.

## Release acceptance scenarios

Automate the deterministic scenarios against actual service interfaces and browser UI. Keep the real-provider smoke test separate, explicitly invoked and cost-bounded.

1. From empty disposable volumes, start the stack, provision an account through the operator CLI and sign in. Verify public signup is closed (development signup is only an explicit fixture exception). Create a unique platform subdomain with the starter theme; duplicate/invalid names return useful errors.
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
