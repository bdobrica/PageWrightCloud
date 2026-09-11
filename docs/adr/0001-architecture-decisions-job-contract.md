# Job wire contract — M1.1 / M1.2 / M2.7

ADR 0001 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


This is the canonical contract for the selected gateway, manager, worker and UI.
Go types remain local to their independently built service modules; real HTTP
contract tests exercise their interoperability instead of introducing a shared
module/build dependency. UI parsers validate responses at runtime.

## Identity and lifecycle

| Field | Meaning / authority |
| --- | --- |
| `job_id` | Execution identity; gateway allocates/persists it before dispatch. Manager can allocate it for direct callers that omit it. Never an artifact ID. |
| `site_id` | Stable site identity, not its FQDN. Gateway resolves it from the URL. |
| `owner_id` | User identity derived from the authenticated, owner-checked site's `user_id`; the browser cannot supply it. |
| `prompt` | Worker instructions generated from the user's message/clarification. |
| `source_version` | Immutable input artifact identity. Required, nonblank. |
| `target_version` | Output artifact identity, distinct in purpose from `job_id`. |
| `status` | Exactly `pending`, `running`, `completed`, or `failed`. No `queued` or `success` aliases. |

The intended lifecycle is pending → running → completed or failed. M1.1 validates
vocabulary, callback outcomes and identities; M1.2 adds durable pre-dispatch mapping
and submission deduplication. M2.2 adds durable dispatch; M2.7 adds
[lease renewal and fenced storage/result commits](0015-architecture-decisions-fenced-commits.md).
Callback recovery remains M2.8/M2.9; terminal callback duplicates return 409.
Manager marks a job running while recording irreversible dispatch intent;
that status alone does not prove a worker ran.

## Gateway: browser build submission

`POST /sites/{fqdn}/build`, bearer-authenticated, accepts one JSON object:

M1.2 also requires a nonzero UUID `Idempotency-Key` header. Retain it for retries;
see [durable submissions](0002-architecture-decisions-build-submissions.md) for replay and uncertainty semantics.

```json
{"message":"Change the homepage title","conversation_id":"optional-existing-conversation"}
```

`message` is required and nonblank. Omit `conversation_id` for a new request.
Unknown properties and trailing JSON are rejected with HTTP 400. In particular,
the browser cannot inject owner, site, source, target or job IDs. Attachments are
not part of this JSON contract; the preexisting multipart UI path remains
unsupported and is scheduled for disabling in M3.9.

HTTP 200 is one of two mutually exclusive shapes:

```json
{"question":"Which title?","conversation_id":"conversation-1"}
```

```json
{
  "job_id":"job-1",
  "site_id":"site-1",
  "owner_id":"user-1",
  "source_version":"version-1",
  "target_version":"version-2",
  "status":"running"
}
```

The accepted shape contains all six fields; a failed accepted snapshot also
requires nonblank `error_message` (absent/empty otherwise). It excludes prompt,
internal lease tokens and clarification fields. A lock conflict becomes HTTP 409.
Unrelated gateway errors retain `{ "error": "HTTP status text", "message": "details" }`.
Submission errors also carry `submission_state`, `job_id` and `target_version`;
HTTP 503 can mean uncertainty, not definite rejection.

## Manager: creation and job snapshots

`POST /jobs` accepts:

```json
{
  "site_id":"site-1",
  "owner_id":"user-1",
  "prompt":"Set the homepage title to Welcome.",
  "source_version":"version-1",
  "target_version":"version-2"
}
```

The first four fields are required, nonblank strings. If `job_id` is omitted,
manager allocates it and an omitted/empty `target_version`. With an explicit
`job_id`, it must be a canonical nonzero UUID and requires a nonblank target
distinct from it (including UUID aliases). Identical explicit-ID submissions
replay with HTTP 200; changed input returns 409. Gateway always sends both IDs
from its committed mapping. Legacy `user_text`,
`base_build_id`, `requested_action` and `metadata` are rejected, not silently ignored.

For legacy direct callers, empty/null `job_id` is treated as omission; the gateway
never uses that allocation path. Explicit nonempty IDs must satisfy the UUID rules.

HTTP 201 creation, HTTP 200 submission replay/`GET /jobs/{job_id}`, and successful callbacks return
the same snapshot:

```json
{
  "job_id":"job-1",
  "site_id":"site-1",
  "owner_id":"user-1",
  "prompt":"Set the homepage title to Welcome.",
  "source_version":"version-1",
  "target_version":"version-2",
  "status":"completed",
  "created_at":"2026-09-05T12:00:00Z",
  "updated_at":"2026-09-05T12:00:05Z",
  "result":"Homepage title updated.",
  "manifest_path":"/sites/site-1/artifacts/version-2/manifest"
}
```

All fields through `updated_at` are required; timestamps use RFC3339, including
optional fractional seconds. Optional string fields are `result` (a human-readable
summary, not an encoded JSON object), `error_message` and `manifest_path`.
`error_message` must be nonblank for failed snapshots and empty/absent otherwise.
`manifest_path` is allowed only for completed outcomes. `error_code` optionally
records manager submission rejection (`job_busy` or `spawn_failed`); callbacks
clear it. Matching rejected submissions repeat the saved HTTP 409/502 error.
Empty optional strings
serialize as absent. Completed callbacks require the canonical manifest path
and an attempt-bound storage commit reservation; this is not an availability
guarantee after a later storage failure.

Internal manager/worker snapshots may additionally contain `lock_token`,
`fencing_token`, and `worker_id`. Gateway wire types deliberately do not retain
those fields. Response readers allow additive fields, but reject missing required
identity, invalid statuses and invalid outcome combinations. The selected worker
only accepts pending/running snapshots at launch; terminal jobs are not launchable.

## Worker callbacks

`POST /jobs/{job_id}/result` accepts only terminal outcomes:

```json
{
  "job_id":"job-1",
  "site_id":"site-1",
  "owner_id":"user-1",
  "source_version":"version-1",
  "target_version":"version-2",
  "status":"failed",
  "error_message":"Compilation failed.",
  "lock_token":"attempt-token-from-running-snapshot",
  "fencing_token":7
}
```

All five identity/version fields are required and nonblank. `status` must be
`completed` or `failed`; optional `result`, `error_message` and `manifest_path`
have the snapshot semantics above. `/jobs/{job_id}/status` uses the same payload
but also permits `running`. Unfenced legacy mock callbacks are rejected.
Both endpoints return the updated full snapshot, not an acknowledgment
with a different schema.

The route's `job_id` and all stored identity/version fields must match the body.
Mismatch returns HTTP 409 without updating the job or releasing its lock.
These are consistency checks, **not authentication**: internal endpoints are
still unauthenticated and must remain restricted to trusted local development.
Callbacks require nonblank `lock_token` and positive `fencing_token` matching the
stored attempt and live site lease. These are checked atomically at result commit;
completion also requires the manifest write reservation. Expired/superseded and
duplicate terminal callbacks return 409 without altering the result. Admission
retries remain idempotent. Scoped callback credentials remain M4.

## Manager errors and compatibility

Manager job-handler errors have JSON content type and this shape:

```json
{"error":"invalid_request","message":"Human-readable validation details"}
```

| HTTP | `error` | Meaning |
| --- | --- | --- |
| 400 | `invalid_request` | Malformed/unknown/trailing JSON, missing fields, invalid status/outcome |
| 404 | `job_not_found` | No recorded job; backend failure is distinct and returns 500 |
| 409 | `job_busy` | Site lock acquisition failed |
| 409 | `job_conflict` | Callback mismatch or conflicting reuse of a submission ID |
| 502 | `spawn_failed` | Persisted failure to start a worker |
| 500 | `internal_error` | Backend reservation, lookup or update failure |

Gateway's manager client preserves HTTP status/code/message in a typed error and
checks that successful responses match the requested identity. Unknown endpoint
and unsupported-method responses remain router behavior, outside this envelope.

This intentionally breaks obsolete job payloads. Rebuild gateway, manager,
worker and UI together. Old Redis job snapshots without owner/source associations
cannot be safely resumed by the new worker; recovery/migration is not implemented.
Do not delete development data automatically or assume existing jobs were upgraded.

## Acceptance evidence and boundaries

- Gateway HTTP client tests assert canonical request keys, response identity,
  status and structured errors, and reject legacy/inconsistent snapshots.
- Manager real-handler tests cover creation/retrieval, both callback endpoints,
  required fields, obsolete keys, invalid statuses, identity mismatches and errors.
- The isolated integration harness runs the actual authenticated gateway build
  handler with PostgreSQL and the real manager API. Only instruction-provider
  HTTP responses are faked; ownership checks, accepted identities, conflict and
  failure propagation are exercised.
- The worker's real HTTP callback sender round-trips completed/failed outcomes
  through the same manager service. No executor or paid provider is invoked.
- `npm run test:contracts` validates UI response parsers used by the API/socket
  paths. These are not browser, socket authentication or publishing tests.

Run `make test-all`, `make test-integration`, and UI contract tests/lint/build as
described in [TESTING.md](../TESTING.md). CI includes the new checks.

M1.2 persists the job-to-target-version mapping and version before dispatch;
see [submission state/retry rules](0002-architecture-decisions-build-submissions.md). M1.6 replaces the current
`initial` source placeholder with a real bootstrap artifact; M2.5 changes base
selection beyond the current live version. M3 adds owner-checked retrieval,
polling and browser recovery. This contract does not make the full pipeline work.
