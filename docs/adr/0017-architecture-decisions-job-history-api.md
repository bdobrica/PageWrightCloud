# Owner-scoped build history (M3.1)

ADR 0017 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


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
M3.2 adds the bounded polling behavior below; M3.3 removes the socket transport.
Clarification/input drafts and session-expiry recovery remain M3.10.

Acceptance: gateway integration tests exercise fresh-handler reads for every
lifecycle state, owner/site isolation at HTTP and SQL boundaries, pagination,
public-field allowlisting, unavailable upstreams and conservative late artifacts.
The deterministic worker round trip verifies that a version appears only after
verified manager observation. UI contract tests, type checking, lint and production
build cover the client/history wiring; actual browser acceptance remains M3.12.

## Bounded UI polling (M3.2)

The current history page is the polling scope (up to 25 jobs). The UI resolves
the owner-checked site's ID, loads a validated page and retrieves only its
pending/running jobs by exact ID. Every update must match site, job, source and
target identity. Older timestamps and running-to-pending regressions are ignored;
terminal outcomes are never revised. Other pages are checked when opened, not
crawled in the background. New submissions reset history to page one, including
uncertain/rejected submissions. Builds submitted in another tab require a history
refresh to discover them; there is no indefinite discovery timer for an idle page.

Checks use a single sequential request chain, with waits of 2, 4, 8, then at most
15 seconds between rounds. Each site/history/job request has a 10-second transport
timeout. Polling stops when all loaded jobs are terminal, after 60 scheduled rounds,
after 15 minutes of elapsed time, or after five consecutive failed rounds. The
elapsed budget is checked before starting further job requests; a request already
in flight may finish within its transport timeout. Hidden/suspended tabs never
catch up by launching overlapping requests. 401/403/404 pause immediately; normal
401 session handling still redirects to login. Pausing never cancels a build or
turns missing evidence into a failure. Manual refresh/resume starts a new budget.

Transient failures retain observed job states and diagnostic codes. Failed builds
display a safe code explanation and their job ID for operator investigation;
private worker/provider messages remain private. Navigating pages/sites, retrying
or unmounting aborts requests and clears the scheduled timer. Late responses from
disposed instances are ignored. Chat is keyed by site so an old submission's
response cannot alter the next site's conversation or history.

Completed observations refresh the version sidebar without restarting history
polling. Completed history on initial load also refreshes it to cover a race with
gateway reconciliation. Chat submission messages identify themselves as submission
acknowledgments and direct users to current history status. Unscoped legacy socket
messages no longer update Chat; M3.3 removes the unused connection entirely.
Polling changes no gateway/manager recovery or dispatch behavior.

`npm run test:contracts` includes deterministic scheduler/request tests for all
four lifecycle states, backoff/round/elapsed/error bounds, completion notifications,
initial/outage recovery, retained failure details, identity and monotonicity checks,
non-overlap, aborts and stale-response suppression. These exercise the actual
controller used by React; full rendered-browser acceptance remains M3.12.

## WebSockets disabled (M3.3)

The UI has no WebSocket hook, socket constructor, token-bearing socket URL or
reconnect timer. Socket URL configuration/build arguments were removed from the
supported Compose stack and examples; existing private `.env` values are ignored
and were not edited. Rebuild/reload the UI to retire older cached clients.

Gateway no longer starts a hub or includes the old upgrader/broadcast pumps.
`/ws` returns a public, cache-disabled HTTP 501 JSON response directing clients to
polling. It does not inspect or echo query/header credentials and cannot upgrade,
regardless of Origin or bearer authentication. Generic CORS preflight handling
still returns 200 for OPTIONS; this does not enable an upgrade. Old clients may
continue attempting reconnects until refreshed; the server never accepts them.

There is deliberately no feature flag to re-enable the old implementation.
Before introducing a new socket transport, require tested browser-compatible
authentication without reusable bearer tokens in URLs, strict Origin checks,
owner/site-authorized subscriptions and delivery, an actual durable job-event
publisher with reconnect/resynchronization semantics, and stable client/server
cleanup (single reconnect timer, bounded backoff, cancellation and disconnect on
navigation/logout). Tests must cover cross-owner/site denial, auth expiry,
duplicate/stale events, missed events and terminal monotonicity. Polling remains
authoritative until this gate is met. CORS hardening for other APIs remains M4.

Regression tests reject handshake requests with/without bearer credentials and
with same/foreign/missing Origins, assert no upgrade headers or credential echo,
and scan UI sources/build wiring for accidental socket reintroduction. Existing
polling lifecycle/cleanup tests remain in the same UI acceptance command.
