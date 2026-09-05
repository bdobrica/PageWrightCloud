# Artifact transport (M1.3)

Worker, gateway and serving use the same storage representation:

| Operation | Storage HTTP contract |
| --- | --- |
| Upload | `PUT /sites/{site_id}/artifacts/{build_id}`, raw archive body, `Content-Type: application/gzip`; success `201` |
| Download | `GET /sites/{site_id}/artifacts/{build_id}`; success `200`, `Content-Type: application/gzip` |

`build_id` here is the artifact version ID, not the job execution ID. Both path
identifiers match `[A-Za-z0-9][A-Za-z0-9._-]{0,254}`. Clients reject invalid IDs
before sending a request and do not follow redirects. Trailing slashes on the
configured storage base URL are normalized.

The gzip file is the HTTP representation, not HTTP compression. Uploads must
have no `Content-Encoding` (or only `identity`); downloads explicitly request
`Accept-Encoding: identity` and reject other encodings. Storage rejects multipart
and other upload media types with `415` before writing anything. No multipart
decoder or compatibility route is provided: deploy the clients and storage
update together.

Worker uploads stream directly from an open file. Gateway's authenticated
download handler streams from storage; worker and serving download into unique
sibling temporary files and rename only after successful copying and closing.
Failed downloads leave an existing destination intact and remove the temporary
file. Storage and gateway abort HTTP responses on mid-stream read errors rather
than ending a partial body successfully. Archive packing checks writer closes;
worker and serving extraction consume the gzip trailer to detect truncation and
checksum failures, including errors after the tar end marker.

## Verification

`make test-integration` runs worker first. It packs the shared
[fixture](../tests/fixtures/artifact-transport.json), uploads to real storage,
downloads and unpacks it, and saves the original compressed bytes in a temporary
test-run directory. Gateway and serving then fetch that same stored object and
compare exact bytes, not just equivalent decompressed content. Checks cover the
complete editable fixture and the deployed public subset, including UTF-8 text,
nested assets and binary bytes.
Serving uses its real artifact deployment/extraction manager, without nginx
activation. Gateway additionally tests its authenticated download handler and an
interrupted upstream response. Local HTTP unit tests exercise invalid headers,
identifiers, redirects and interrupted transfers.

## Boundaries

Storage is deliberately an opaque byte store: `application/gzip` is a transport
contract, not server-side archive validation. Existing malformed objects are not
repaired. A successful upload does not imply a complete, publishable version.
M1.4 adds [manifest/private-log persistence and commit visibility](VERSION_METADATA.md);
M1.5 adds [immutable writes and disabled deletion](IMMUTABLE_VERSIONS.md).
The existing `/sites/{site_id}/logs` endpoint stores event records, not the
worker's private per-version log payload. Gateway deletion is disabled (`501`);
storage has no DELETE endpoint (`405`).

M1.7 adds [archive layout validation](ARCHIVE_LAYOUT.md), extraction limits and
isolated staging: rejected extraction does not leave partial destination files.
Only public files enter the serving cache. This does not establish internal-service
authorization, safe HTML generation, nginx activation or an end-to-end AI build.
