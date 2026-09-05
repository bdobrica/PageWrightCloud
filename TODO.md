# TODO — working MVP

Updated: 2026-09-05. Strategy, code evidence, scope and acceptance scenarios: [PLAN.md](PLAN.md).

This replaces the March security-first two-week schedule with a dependency-ordered MVP backlog. Existing components are acknowledged below; a checked component does not imply an integrated feature. All unfinished MVP work is unchecked. Longer-term ideas are retained in the deferred backlog.

## Verified starting point

- [x] Inspect the UI, gateway, manager, worker, storage, serving, compiler and root deployment wiring.
- [x] Confirm auth/site/version handlers, dashboard/chat/profile/reset pages and Docker configuration exist.
- [x] Run available untagged Go tests in all six modules: passed; compiler has no test files and some worker/serving tests are skipped.
- [x] Compile the bundled sample with the starter theme: home/about/contact pages and assets generated.
- [x] Run UI production build: passed with a CSS unexpected-`}` warning.
- [x] Run UI lint: baseline is **17 errors and 1 warning**, not passing.
- [x] Validate Compose syntax and inspect project containers: config valid with missing OAuth/LLM variables; no project containers running.
- [x] Create an evidence-backed MVP plan with milestone exit criteria.
- [ ] Demonstrate a complete real AI build, preview, publication and rollback. **Not currently working or verified.**

## M0 — Reproducible development and checks

Exit: a fresh checkout builds, initializes and restarts predictably; baseline checks run in CI.

- [x] M0.1 Document compatible Go/Node/Docker prerequisites from module files, lockfiles and Docker images; verify clean UI dependency installation and all selected image builds. Implementation: `8f404df`; clean local/container `npm ci` and UI build, all seven selected images passed.
- [ ] M0.2 Fix UI lint errors in auth state initialization, socket reconnection, typed error handling and component exports; resolve the hook dependency warning and CSS build warning.
- [ ] M0.3 Standardize unit/integration commands and prerequisites. Include manager integration tests in the root target; provision a dedicated test DB, Redis and the actual services needed by HTTP integration tests.
- [ ] M0.4 Use one migration source with transactional version tracking; reconcile embedded gateway migrations, SQL files and the stub DB helper. Test empty DB startup and restart/upgrade of an existing DB.
- [ ] M0.5 Simplify the single-host Compose setup to named-volume filesystem storage; remove the unnecessary privileged NFS service from this path. Separate host-port overrides from stable internal ports/healthchecks.
- [ ] M0.6 Add CI for Go package tests, UI build/lint and compiler fixture build. Record skipped tests explicitly; add integration/browser checks as M1–M3 land.
- [ ] M0.7 Update README startup/status claims and distinguish seeded hosting diagnostics from the full MVP smoke test.

## M1 — Contracts, initial source and artifacts

Depends on M0. Exit: a deterministic build traverses actual gateway/manager/storage/serving interfaces and produces hosted HTML without manual file seeding.

- [ ] M1.1 Define canonical job request/result/status schemas: `prompt`, `source_version`, `target_version`, `job_id`, errors and owner/site association. Align gateway, manager, worker and UI with HTTP contract tests.
- [ ] M1.2 Persist the job-to-target-version mapping before dispatch; stop using job IDs as artifact IDs. Handle failed enqueue/version DB writes and duplicate submissions explicitly.
- [ ] M1.3 Align all three storage clients with server routes; stream raw gzip bytes or implement one explicit multipart decoder. Verify uploaded/downloaded bytes and archive extraction match.
- [ ] M1.4 Implement and test manifest and private log persistence using agreed endpoints. Make complete versions visible only after required artifact/metadata writes succeed.
- [ ] M1.5 Enforce immutable version writes and safe temporary-file/atomic commit behavior. Implement version deletion, including metadata cleanup and active-version protection, or disable deletion in MVP UI/API.
- [ ] M1.6 Bootstrap each new site with valid `content/site.json` and home content; explicitly map the UI template to bundled `starter`. Surface initialization failures and make retries safe.
- [ ] M1.7 Define and validate the archive layout (`content/`, `public/`, manifest); keep execution instructions/logs/secrets out of public output and preserve source for future edits.
- [ ] M1.8 Add compiler fixtures for starter rendering, home discovery, MDX/component errors, navigation, assets and malformed configuration. Test traversal, escaping and symlink boundaries.
- [ ] M1.9 Fix serving payloads (`version` versus `version_id`) and normalize gateway version responses to UI fields, statuses and timestamps.
- [ ] M1.10 Add a deterministic executor fixture used only for tests; round-trip a compiled artifact through the real service routes.

## M2 — Real builds and recoverable jobs

Depends on M1. Exit: a real request changes site content; successive edits preserve changes; failures terminate visibly and leave live content intact.

- [ ] M2.1 Implement the Docker spawner and configure the intended network, explicit worker image tag, work directory, storage/manager endpoints and scoped credentials. Remove ambiguity with the manager's separate mock worker image.
- [ ] M2.2 Replace request-handler spawning with bounded queue dispatch; acknowledge jobs durably and recover interrupted dispatch without duplicate execution.
- [ ] M2.3 Package and pin a real non-interactive AI CLI in the selected worker image. Verify its installed authentication/instruction/sandbox behavior; keep the mock restricted to tests.
- [ ] M2.4 Package `pagewrightc` and a trusted versioned starter theme. After edits, validate allowed source changes, compile into `public/`, check HTML/assets and produce truthful manifest results.
- [ ] M2.5 Use latest completed draft → live → bootstrap as the default source selection. Expose that base to the user and verify two unpublished edits accumulate.
- [ ] M2.6 Enforce worker CPU/memory/process/runtime/output limits and cancellation. Isolate writable source from theme/compiler; do not expose the Docker socket, unrelated host files or management credentials to AI execution.
- [ ] M2.7 Renew site locks, enforce fencing/attempt identity at result/artifact commit, and reject stale or duplicate terminal updates. Test concurrent builds lasting longer than the original lock TTL.
- [ ] M2.8 Retry result delivery with bounds; reconcile artifacts and worker exits when callbacks are lost. Record failed/spawn-error/timeout states and release locks on every terminal path.
- [ ] M2.9 Persist Redis data and durable job/version history; reconcile pending/running jobs after restart and beyond Redis's current 24-hour job TTL.
- [ ] M2.10 Clean up exited/orphan containers and temporary data with a documented retention policy. Capture bounded, redacted logs correlated by site/job/version.
- [ ] M2.11 Repair skipped executor parsing/cancellation tests and test actual runner failures: invalid output, compiler error, upload failure, callback failure, timeout and restart.
- [ ] M2.12 Run one explicitly invoked, cost-bounded real-provider smoke test and verify the requested content changed in the resulting HTML.

## M3 — Browser workflow, preview and publication

Depends on M2. Exit: create → edit → refresh → preview → publish → edit → rollback works through the UI.

- [ ] M3.1 Add owner-checked gateway job retrieval and persistent build history; update version state from manager results. Recover pending/terminal status after gateway restart and browser refresh.
- [ ] M3.2 Poll active jobs from the UI with bounded backoff and consistent pending/running/completed/failed handling. Filter by current site/job and retain useful failure details.
- [ ] M3.3 Disable the broken WebSocket integration for the polling MVP. Before re-enabling it, implement browser-compatible auth, origin checks, owner filtering, event delivery and stable reconnect cleanup.
- [ ] M3.4 Make Preview call the deployment API before opening a returned preview URL. Provision host routing on first preview, including before first publish.
- [ ] M3.5 Implement the chosen serving/nginx lifecycle in Compose: supervised processes, configuration validation, controlled reload, rollback on reload failure and restart recovery.
- [ ] M3.6 Return configured live/preview URLs; remove hard-coded HTTPS/port assumptions and unsupported `/preview/{build_id}` links. Test nested pages/assets on both hosts and promotion of the exact same artifact.
- [ ] M3.7 Preserve the opposite live/preview DB pointer on updates and handle DB write errors. Reconcile DB and serving state if deployment only partially succeeds.
- [ ] M3.8 Replace remove-then-create symlinks with atomic replacement; stage extraction before activation. Prevent retention/deletion from removing active versions and test rollback.
- [ ] M3.9 Hide unsupported attachments, arbitrary domains/aliases and OAuth controls; disable unsupported backend actions in MVP mode. Keep the supported text-only request path explicit.
- [ ] M3.10 Preserve drafts across session expiry/re-authentication; show progress, retryable failures, empty states and meaningful version labels. Refresh dashboard deployment state after actions.
- [ ] M3.11 Verify keyboard access, focus handling and usable dashboard/chat/modals on mobile and desktop.
- [ ] M3.12 Add browser acceptance coverage for the full journey, two sequential edits, preview/live independence, refresh, build failure and rollback. No manual HTML seeding or DB edits in this suite.

## M4 — Remote pilot release gate

Apply relevant protections while building M1/M2; all items below block admitting remote testers.

- [ ] M4.1 Require invitations or operator provisioning; implement per-user/site build quotas, request throttling and bounded AI spending/concurrency.
- [ ] M4.2 Remove default production secrets and fail startup for missing critical configuration. Wire PostgreSQL credentials consistently; redact reset tokens, credentials and private prompts from routine logs.
- [ ] M4.3 Restrict internal service/database/Redis ports; authenticate service writes and scoped worker callbacks. Test direct unauthenticated requests and cross-job credential misuse.
- [ ] M4.4 Validate/normalize platform subdomains, reserved names, site/version identifiers and path containment before filesystem or nginx use. Reject configuration injection and traversal.
- [ ] M4.5 Bound JSON bodies, archive bytes, decompressed size and file count; reject unsafe links/file types. Test compiler/assets/archive escape attempts and malformed metadata.
- [ ] M4.6 Replace wildcard HTTP/socket origin policies with configuration and tests. Keep application and generated-site origins separate; configure appropriate security headers.
- [ ] M4.7 Test cross-user access for site, job, version, download, preview/deploy and deletion endpoints; apply owner filtering before any event delivery.
- [ ] M4.8 Complete password-reset email delivery and single-use token consumption, expiry and throttling; normalize auth responses and apply a consistent password policy. Confirm session expiry/recovery preserves ongoing work.
- [ ] M4.9 Configure DNS and HTTPS for app, live and preview hosts; document local equivalents and disable unsupported domain aliases.
- [ ] M4.10 Add downstream request cancellation, appropriate server/client timeouts, DB pool limits and dependency readiness checks.
- [ ] M4.11 Document and exercise backups/restores of PostgreSQL, artifacts and deployment metadata in disposable infrastructure; verify recovery of the published site.
- [ ] M4.12 Resolve or replace skipped serving cleanup/config tests; run meaningful race checks on job/lock/conversation state and any enabled WebSocket hub.
- [ ] M4.13 Run all release scenarios in PLAN.md, including worker/manager restart, failed deployment, two-user isolation and real AI smoke. Record commands, results, limits and known issues.
- [ ] M4.14 Publish operator runbooks for failed jobs, credential rotation, disk pressure, backup restore and rollback; update README to the verified release state.

## Deferred backlog — not MVP blockers

Retained and grouped from the previous roadmap; revisit after pilot feedback.

- [ ] Compiler/themes: more themes, theme validation tooling, richer props validation, incremental builds, drafts, sitemap/RSS, SEO suggestions and marketplace.
- [ ] Editing UX: safe attachments, browser screenshots/preview checks, cloning, team collaboration, analytics and localization.
- [ ] Identity: Google/other OAuth, email verification for open signup, refresh sessions, 2FA and richer account administration.
- [ ] Domains: customer-domain ownership verification, alias lifecycle, automatic certificates, CDN integration and cache invalidation.
- [ ] API: search/filter/pagination extensions, bulk operations, versioning, OpenAPI, webhooks and external integrations; GraphQL only if a concrete need emerges.
- [ ] Orchestration: Kubernetes/Helm, priority queues, dead-letter tooling, job cancellation UI, pre-warmed pools and autoscaling; NATS/Kafka/RabbitMQ backends.
- [ ] Storage: S3-compatible/Azure/GCS/RustFS adapters, retention automation, deduplication, compression options, metadata search, multipart/resumable uploads, ETags and caching.
- [ ] Operations: metrics/Grafana, alerting, tracing/error tracking, load testing, container/dependency scanning, IaC, mTLS/secret-management tooling and multi-region recovery.
- [ ] Platform: billing, white-labeling, plugins/template extensions and expanded developer/operator documentation.

The old fixed two-week schedule and coverage-percentage targets are superseded by milestone acceptance checks. Features should move into the MVP only when necessary to pass those checks or in response to explicit product scope changes.
