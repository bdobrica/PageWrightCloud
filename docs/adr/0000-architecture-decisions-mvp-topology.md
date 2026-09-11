# MVP topology and implementation principles

ADR 0000 · Status: accepted baseline, recorded from the original MVP plan.

## Context

The initial assessment found partially connected Go services and a React UI.
The MVP needed a complete single-host edit/preview/publish path without a service
rewrite. This record preserves the original seven principles; future-tense
milestone references describe the planning baseline, not current release status.
See [release acceptance](../RELEASE_ACCEPTANCE.md) for current evidence.

## Decisions

1. **Keep identifiers explicit.** `job_id` identifies an execution; `target_version` identifies its immutable artifact. Persist their association with site and owner before dispatch. Use `pending → running → completed | failed` consistently, with bounded retries and idempotent terminal updates. Reject stale worker results using job identity and fencing/attempt information.
2. **Keep editable source with each version.** Use an archive containing `content/`, generated `public/` when available, and safe layout metadata. Keep source-version provenance, theme/compiler versions and check results in the private build manifest; trusted compiler/check integration remains M2. Execution logs are private metadata. Publish only `public/`; never expose source, prompts, instruction files or secrets through nginx. Render from a trusted bundled theme outside the AI-writable directory. Preserve the same artifact when promoting preview to live.
3. **Define the editing base.** Default the next edit to the latest completed draft, then the live version, then the bootstrapped source. Surface the selected base in the UI. A failed job must not become the next base; this avoids losing consecutive unpublished edits.
4. **Start with reliable polling.** Gateway exposes owner-checked job status and persisted history; UI polls while a build is active and recovers after refresh. Disable the broken socket path in the MVP until it supports browser authentication, owner filtering, stable reconnection and the same event schema. WebSockets are optional, durable state is required.
5. **Use one local deployment path.** Prefer a named volume for the existing filesystem storage backend on a single host; the privileged NFS server is unnecessary for this topology. Give the selected worker image an explicit tag that matches manager configuration. Define the Docker network, credentials and resource limits used by spawned jobs. The manager's Docker authority must remain inaccessible to worker processes.
6. **Make serving own its nginx lifecycle.** For the MVP, run the serving controller and nginx together under a documented supervisor, with config validation and controlled reload. This removes the current cross-container reload gap. Return explicit live and preview URLs from the gateway, including local scheme/port. Use a preview hostname/origin strategy that supports promoting the same artifact; test links and assets under both hosts. Keep generated-site origins separate from the application origin, especially if cookies are introduced.
7. **Bound AI execution.** Pin and verify a real supported non-interactive CLI at implementation time; verify authentication, instruction discovery, filesystem restrictions, timeout and cancellation with that installed version. Use a deterministic fake executor only in tests. Constrain runtime, concurrency, output size and per-user build usage before allowing paid requests from testers.

## Consequences

The single-host topology and trusted control plane simplify operation but do not
provide multi-host HA. Immutable source/artifacts and durable execution/deployment
identities make retry and rollback verifiable, at the cost of retained storage
and conservative quarantine when evidence is missing. Polling avoids a second
event-delivery protocol. Privileged or unrestricted workers are not a fallback
for incompatible hosts. See [operations](../OPERATIONS.md) for recovery rather
than treating these principles as live repair commands.
