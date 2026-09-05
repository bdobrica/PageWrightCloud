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
| Job submission | [Gateway client](pagewright/gateway/internal/clients/manager.go) sends `user_text` and `base_build_id`; [manager](pagewright/manager/internal/api/handler.go) requires `prompt` and uses `source_version` / `target_version`. | A normal build request is rejected for missing `prompt`. Agree one job contract and version identity. |
| Execution | [Docker spawner](pagewright/manager/internal/spawner/docker/docker.go) and Kubernetes spawner only log. [Worker Dockerfile](pagewright/worker/Dockerfile) installs a mock command. | An accepted job cannot execute the intended AI workflow. There are also two worker implementations/images; select `pagewright/worker` as the MVP runner. |
| First site | [CreateSite](pagewright/gateway/internal/handlers/sites.go) inserts a DB row only. [Build](pagewright/gateway/internal/handlers/build.go) falls back to `initial`; the worker always downloads a source artifact. UI selects `template-1`, while the supplied theme is `starter`. | Bootstrap a valid, versioned source and map the supported template explicitly. |
| Artifact transport | [Storage routes](pagewright/storage/internal/api/handler.go) use `/sites/{site_id}/artifacts/{build_id}`; gateway, worker and serving clients use `/artifacts/{site_id}/{version}`. Worker sends multipart bytes while storage writes the raw body. | Downloads fail; fixing paths alone still leaves malformed archives. Manifest/log endpoints expected by the worker and deletion expected by the gateway are missing. |
| Compilation | [Worker runner](pagewright/worker/cmd/runner/main.go) edits and repacks source without invoking the compiler; `ChecksPassed` is hard-coded. [Serving](pagewright/serving/internal/artifact/manager.go) requires an archive containing `public/`. | Integrate `pagewrightc`, generate `public/index.html`, and derive validation results from actual checks. |
| Version state | Gateway creates a version using the job ID, independently of manager's target version. [Version listing](pagewright/gateway/internal/handlers/versions.go) returns storage records that differ from UI types. | Use one artifact/version ID, persist the job mapping, reconcile completion, and normalize the UI response. |
| Live updates | [UI socket](pagewright/ui/src/hooks/useWebSocket.ts) sends a query token; [auth middleware](pagewright/gateway/internal/middleware/auth.go) accepts only a bearer header. [Hub](pagewright/gateway/internal/websocket/hub.go) has no caller publishing build results and no implemented ownership filter. UI expects `success`, manager emits `completed`. | Implement authenticated job retrieval first; polling can complete the MVP. Repair and authorize WebSockets before enabling them. |
| Deploy / preview | [Gateway serving client](pagewright/gateway/internal/clients/serving.go) sends `version_id`; [serving types](pagewright/serving/internal/types/types.go) require `version`. Preview UI only opens a URL and does not activate a preview. | Repair the payload, preview action, returned URLs, and preview assets/navigation. |
| Deployment consistency | [DB update](pagewright/gateway/internal/database/sites.go) sets both live and preview IDs, clearing one when the other changes. Serving removes the old symlink before creating the new one. | Preserve the other pointer, handle DB failures, and replace symlinks using an atomic rename. |
| Hosting | [Compose](docker-compose.yaml) separates serving and nginx, but serving executes local `nginx -s reload`. Preview activation does not create nginx config. | Make nginx configuration/reload part of a supported topology; first preview must work before first publish. |
| Job reliability | Manager queues and immediately spawns inside the HTTP handler; no queue consumer runs in server main. Lock renewal and worker timeout configuration are not wired into the lifecycle. Redis has no persistent volume in root Compose. | Add durable dispatch, bounded concurrency, lock renewal, timeout handling and restart recovery. |
| User-facing gaps | File uploads are multipart in the UI but build handler decodes JSON. Reset email is a TODO and reset tokens are logged. Site links assume HTTPS without the local port. | Ship only working controls, configure returned hosting URLs, and complete recovery before remote access. |
| Boundaries | Internal write APIs have no authentication and their ports are published. FQDNs reach filesystem/nginx paths without adequate validation. Wildcard HTTP/socket origins remain. | Enforce service authorization, validate identifiers, restrict exposure, and isolate worker credentials and generated content. |

The compiler is a useful trusted build component, but its presence and the worker's instruction file do not themselves enforce an execution security boundary. The worker currently runs a command with its inherited environment and has no integrated trusted-output validation.

## Verification baseline

Executed during this assessment:

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

Resolve the UI lint/CSS baseline; document tool versions supported by the lockfiles and images. Establish explicit commands for unit tests, integration prerequisites, UI checks, image builds and migrations. Exercise startup from fresh, disposable volumes and restart with existing data. Consolidate migration ownership: gateway startup currently embeds migrations separately from the SQL files, and the DB helper's `RunMigrations` is a stub.

**Exit:** a fresh checkout can build the UI and all selected service images, initialize the DB once, restart safely, and run documented checks. CI starts running these checks.

### M1 — Contracts, bootstrap and artifact round trip (3–5 days)

Repair job, storage, serving and UI contracts together, with tests exercising real HTTP handlers. Bootstrap a site using `starter`, record initial source, and define archive/manifest storage. Compile a deterministic edit fixture and round-trip its archive through storage and serving. Add version deletion support or disable the corresponding UI/API until implemented.

**Exit:** without an AI dependency, create a fresh site, submit the canonical job, obtain a valid immutable artifact and fetch its `public/index.html` through hosting. No manually seeded serving files or direct DB edits.

### M2 — Real worker and recoverable job lifecycle (4–7 days)

Implement the Docker spawner and a queue dispatcher with bounded concurrency. Build the selected real worker image with the compiler and trusted theme. Bootstrap/download source, execute a bounded edit, validate allowed changes, compile to `public/`, validate output, store the artifact and report completion. Add lease renewal, fencing enforcement, worker cleanup, callback retries and reconciliation for lost callbacks or manager restarts. Persist sufficient job/version state beyond Redis's current 24-hour job TTL.

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
