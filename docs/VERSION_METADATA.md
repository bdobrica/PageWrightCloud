# Version metadata and completion (M1.4)

M2.7 adds [attempt-bound digest reservations](FENCED_COMMITS.md) before immutable
publication. Non-bootstrap POST/PUT requests require the current worker attempt
header; manifest fencing must match. Identical object retries require a live
attempt, and terminal callback duplicates now return 409. The ordering below
remains; recovery of reserved-but-not-materialized bytes remains M2.8/M2.9.

Storage treats the manifest as the final commit record. M1.5 adds
[write-once publication and byte-identical retries](IMMUTABLE_VERSIONS.md). The worker performs:

`artifact PUT → private log POST → manifest POST → manager completed callback`

Each failure stops this sequence. Log persistence is required, including an empty
log. No successful callback is attempted after an artifact/log/manifest failure.
A lost manifest response may leave a committed storage version while the worker
reports failure; a lost callback may leave manager status uncertain. Storage
retries do not create duplicate listing entries. Cross-service reconciliation
remains M2; storage now enforces retry byte identity, not a distributed transaction.

## Internal HTTP contract

Prefix: `/sites/{site_id}/artifacts/{build_id}`, where `build_id` is the artifact
version ID, not the execution job ID. Same identifier rules as
[artifact transport](ARTIFACT_TRANSPORT.md).

| Method / suffix | Payload or result |
| --- | --- |
| `POST /logs` | JSON `{"content":"execution output"}`; content must be a string, may be empty; no other fields |
| `GET /logs` | The stored JSON, including exact escaped text/binary string content |
| `POST /manifest` | JSON object requiring matching `site_id`, `build_id`, and nonzero RFC3339 `created_at`; other compiler/worker fields are preserved |
| `GET /manifest` | Stored manifest, only while artifact and log prerequisites remain present |

POST requires `application/json`, absent/identity Content-Encoding, and at most
4 MiB of encoded JSON. Successful POST returns `201`; malformed payload/identity
returns `400`, unsupported representation `415`, oversized body `413`, missing
artifact/log at manifest commit `409`, and storage write failure `500`. Missing
metadata GET returns `404`; invalid manifests or unreadable files return `500`. Metadata
responses have `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

Manifests validate persistence identity, not compiler assertions. In particular,
`checks_passed` is not an enforced publishing gate; the runner's existing stubbed
checks are repaired by M2.4. This does not define the final archive manifest
layout (M1.7).

## Persistence and visibility

Sidecars live outside the archive at
`/nfs/sites/{site_id}/metadata/{build_id}/{execution,manifest}.json`. New metadata
directories use `0700`, files `0600`. Writes use unique temporary files and checked
write/sync/close before no-replace publication. Artifact close errors are also propagated.
`manifest.json` is written only after an artifact and private log file exist.

`GET /sites/{site_id}/versions` returns one entry per valid committed manifest,
newest `created_at` first, using `build_id`, `timestamp`, `action: "build"` and
`status: "completed"`. It never includes prompt or private execution output.
Partial, corrupt or missing commit records and missing prerequisites stay hidden. Raw artifact
GET remains available before commit for the existing transport/bootstrap flow;
this is listing completeness, not a download authorization gate.

Legacy `/sites/{site_id}/logs` event records are retained but no longer create
version-list entries. Existing artifact-only/event-only versions are hidden, not
deleted or automatically certified complete. Their bytes remain retrievable by
known ID; supply the real required metadata to commit them. No synthetic migration
or fabricated successful manifest is performed.

## Privacy and remaining limits

Private means separate from the downloadable archive/static public tree and
omitted from version listings. These endpoints remain on the existing internal
storage API, which currently has no service authentication. Do not expose the
storage port to untrusted networks; this is **not tenant authorization or secret
redaction**. No public gateway metadata route is added. Existing secrets already
inside an archive are not removed by this change (M1.7/M4).

M1.5 prevents replacement, syncs publication directories and disables version
deletion. Orphan partial uploads remain stored but unlisted; job fencing and
callback reconciliation remain separate. Manager callbacks still trust workers; no nginx publication
or real AI execution is proved here. Logging of runs that fail before reaching
the persistence phase and redaction/retention policies remain follow-up work.
Post-write integrity checks on artifact/log contents are not performed by storage.

## Acceptance checks

Storage tests exercise failed log/manifest writes, hidden partial versions,
retry without duplicate entries, reopening the backend, missing/corrupt records,
sorting, permissions, identity/schema/media/size errors and legacy event isolation.
Worker tests verify upload ordering and absence of a completed callback on each
persistence failure. The isolated integration harness persists actual worker
metadata and checks gateway visibility, while gateway/serving still compare the
unchanged archive bytes and extracted fixture contents. Startup smoke checks
cover metadata persistence across container recreation.
