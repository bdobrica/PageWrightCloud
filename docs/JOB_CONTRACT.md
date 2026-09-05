# Job wire contract — M1.1

This is the canonical contract for the selected gateway, manager, worker and UI.
Go types remain local to their independently built service modules; real HTTP
contract tests exercise their interoperability instead of introducing a shared
module/build dependency. UI parsers validate responses at runtime.

## Identity and lifecycle

| Field | Meaning / authority |
| --- | --- |
| `job_id` | Execution identity; currently allocated by the manager. Never an artifact ID. |
| `site_id` | Stable site identity, not its FQDN. Gateway resolves it from the URL. |
| `owner_id` | User identity derived from the authenticated, owner-checked site's `user_id`; the browser cannot supply it. |
| `prompt` | Worker instructions generated from the user's message/clarification. |
| `source_version` | Immutable input artifact identity. Required, nonblank. |
| `target_version` | Output artifact identity, distinct in purpose from `job_id`. |
| `status` | Exactly `pending`, `running`, `completed`, or `failed`. No `queued` or `success` aliases. |

The intended lifecycle is pending → running → completed or failed. M1.1 validates
the vocabulary, callback outcomes and identity consistency, not durable dispatch,
terminal-state idempotency, fencing or recovery. Those remain M1.2/M2 work.
Manager currently marks a job running while calling its placeholder spawner;
that status alone does not prove a worker ran.

## Gateway: browser build submission

`POST /sites/{fqdn}/build`, bearer-authenticated, accepts one JSON object:

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

The accepted shape contains all six fields; it excludes prompt, internal lease
tokens and clarification fields. A manager lock conflict becomes HTTP 409.
Existing gateway errors retain `{ "error": "HTTP status text", "message": "details" }`;
this task does not change unrelated public API error codes.

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

The first four fields are required, nonblank strings. Omitted or empty
`target_version` asks the manager to allocate a UUID; a nonempty supplied target
must be nonblank. The manager allocates `job_id` separately. It does not currently
accept caller-supplied job IDs or provide deduplication. Legacy `user_text`,
`base_build_id`, `requested_action` and `metadata` are rejected, not silently ignored.

HTTP 201 creation, HTTP 200 `GET /jobs/{job_id}`, and successful callbacks return
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
  "manifest_path":"/artifacts/site-1/version-2/manifest"
}
```

All fields through `updated_at` are required; timestamps use RFC3339, including
optional fractional seconds. Optional string fields are `result` (a human-readable
summary, not an encoded JSON object), `error_message` and `manifest_path`.
`error_message` must be nonblank for failed snapshots and empty/absent otherwise.
`manifest_path` is allowed only for completed outcomes. Empty optional strings
serialize as absent. The manifest value is an opaque locator; accepting it does
not verify storage availability or settle M1.4's manifest endpoint.

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
  "error_message":"Compilation failed."
}
```

All five identity/version fields are required and nonblank. `status` must be
`completed` or `failed`; optional `result`, `error_message` and `manifest_path`
have the snapshot semantics above. `/jobs/{job_id}/status` uses the same payload
but also permits `running`. It remains available for the manager's legacy mock
worker. Both endpoints return the updated full snapshot, not an acknowledgment
with a different schema.

The route's `job_id` and all stored identity/version fields must match the body.
Mismatch returns HTTP 409 without updating the job or releasing its lock.
These are consistency checks, **not authentication**: internal endpoints are
still unauthenticated and must remain restricted to trusted local development.
Scoped callback credentials and attempt/fencing verification remain later work.

## Manager errors and compatibility

Manager job-handler errors have JSON content type and this shape:

```json
{"error":"invalid_request","message":"Human-readable validation details"}
```

| HTTP | `error` | Meaning |
| --- | --- | --- |
| 400 | `invalid_request` | Malformed/unknown/trailing JSON, missing fields, invalid status/outcome |
| 404 | `job_not_found` | Job lookup failed (backend does not yet distinguish missing from unavailable) |
| 409 | `job_busy` | Site lock acquisition failed |
| 409 | `job_conflict` | Callback route/identity/version mismatch |
| 500 | `internal_error` | Queue, update or spawn failure |

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
described in [TESTING.md](TESTING.md). CI includes the new checks.

M1.2 still must persist a job-to-target-version mapping before dispatch and fix
the gateway's legacy version-row write using `job_id`. M1.6 replaces the current
`initial` source placeholder with a real bootstrap artifact; M2.5 changes base
selection beyond the current live version. M3 adds owner-checked retrieval,
polling and browser recovery. This contract does not make the full pipeline work.
