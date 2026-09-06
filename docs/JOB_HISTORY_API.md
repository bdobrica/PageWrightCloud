# Owner-scoped build history (M3.1)

Authenticated gateway routes:

- `GET /sites/{fqdn}/jobs?page=1&page_size=25`: newest-first durable submissions,
  including reserved/uncertain and rejected builds. Page size is 1–100; pages are
  positive integers up to 2147483647. Invalid pagination returns 400.
- `GET /sites/{fqdn}/jobs/{job_id}`: one durable submission in that site.
  Unknown, malformed or cross-site job IDs return 404; another owner's site
  returns 403. Missing authentication returns 401.

Responses are `Cache-Control: no-store`. The list uses the existing
`data/page/page_size/total_count/total_pages` envelope, with `[]` for empty pages.
Within a request, count and rows share a read-only repeatable-read snapshot.
Order is creation time descending, then job ID descending. Separate offset pages
are not a frozen snapshot: a new build can shift subsequent pages.

Each item contains `job_id`, `site_id`, `source_version`, `target_version`,
`status`, `dispatch_state`, `created_at`, `updated_at`, and optional `error_code`
and `recovery_error`. Status is pending/running/completed/failed. Dispatch state
is ready/dispatching/accepted/rejected. Raw generated prompts, error messages,
private results/logs, request hashes/keys and runtime credentials are omitted.
Diagnostic codes use a fixed allowlist; unknown values become generic failure
or recovery-attention codes, rather than leaking arbitrary upstream text.
This is build history, not a persisted transcript of clarification messages.
The existing bounded-per-state lifecycle journal remains an operator/internal
record, not a raw public response.

Reads never enqueue or redispatch jobs and do not call manager or storage.
They remain available during upstream outages. They report the last PostgreSQL
observation, **not** a claim of instantaneous manager status. Existing M2.9
background recovery verifies identity, updates submission/version/journal in one
transaction, and retains immutable terminal outcomes. Its due-work delay and
bounded batches can make fresh results temporarily lag. Missing evidence remains
uncertain and is not turned into a new execution by a read or browser refresh.

The versions endpoint lists manifest-committed artifacts, excluding targets whose
durable submissions are not completed. Pending/running/failed builds are visible
in job history instead. Storage alone cannot revive a conservative failure;
manager completion alone cannot make an absent artifact available. Artifacts
without a durable submission retain legacy listing behavior. This is a read-time
intersection, not a storage deletion or a publishing authorization gate. Source
selection and deploy/download authorization are unchanged by M3.1.

Chat mounts a site-keyed history component and loads this endpoint after a browser
refresh, independently of its ephemeral message list. Pagination and manual retry
are available; stale responses after navigation/unmount are ignored. Submission
completion triggers a history refresh, including uncertain/rejected responses.
No automatic job polling was added (M3.2); the pre-existing socket remains M3.3.
Clarification/input drafts and session-expiry recovery remain M3.10.

Acceptance: gateway integration tests exercise fresh-handler reads for every
lifecycle state, owner/site isolation at HTTP and SQL boundaries, pagination,
public-field allowlisting, unavailable upstreams and conservative late artifacts.
The deterministic worker round trip verifies that a version appears only after
verified manager observation. UI contract tests, type checking, lint and production
build cover the client/history wiring; actual browser acceptance remains M3.12.
