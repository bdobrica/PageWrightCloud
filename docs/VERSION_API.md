# Version and serving contracts (M1.9)

Gateway sends these serving JSON bodies:

| Serving request | Body |
| --- | --- |
| POST /sites/{fqdn}/artifacts | `{"site_id":"site-id","version":"artifact-id"}` |
| POST /sites/{fqdn}/activate | `{"version":"artifact-id"}` |
| POST /sites/{fqdn}/preview | `{"version":"artifact-id"}` |

`version_id` remains the gateway URL parameter, not a serving JSON field.
Deployment accepts 200/201; activation/preview require 200. Redirects are not
followed; trailing base-URL slashes are normalized. Failures propagate without
automatic write retries.

## Owner-checked listing

`GET /sites/{fqdn}/versions?page=1&page_size=25` returns:

```json
{
  "data": [{
    "id": "artifact-id",
    "site_id": "site-id",
    "build_id": "artifact-id",
    "status": "completed",
    "created_at": "2026-09-06T12:00:00Z"
  }],
  "page": 1,
  "page_size": 25,
  "total_count": 1,
  "total_pages": 1
}
```

Storage's manifest-backed listing remains authoritative. Gateway maps `timestamp`
to UTC RFC3339(`Nano`) `created_at`, supplies the owned site's identity and exposes
only these five fields. `id` is the artifact/build ID within this site, **not a
database version-row UUID or job ID**. Records require valid unique artifact IDs,
nonzero timestamps and `completed` status; semantic violations return 502.
Storage request/decoding failures remain 500. Authorization precedes storage access.

Sort order is newest first, with ascending build IDs breaking timestamp ties.
Page sizes are 1–100; invalid sizes use the configured default (validated, with
fallback 25). Invalid/nonpositive pages default to 1. Out-of-range pages return
`data: []` without multiplication overflow. Empty collections have zero total pages.

The UI validates version fields, timestamps and pagination at runtime. It exposes
load failures, clears prior selections/lists and ignores late responses after
site changes. Private metadata, logs and prompts are never forwarded.

This is a committed-artifact list, not pending/running/failed job history.
DB/storage reconciliation remains M3.1. A completed initial source archive is
not thereby compiled, previewable or publishable.

## Chat and verification

The shared route is `/chat/:fqdn`. Site cards build links from encoded FQDNs;
Chat reads the matching parameter for build/version API calls. An executable
router-matching test checks the relationship.

Gateway tests cover exact serving bodies/statuses/redirects and normalization.
HTTP integration uses real PostgreSQL/storage for initial-version listing,
authorization, empty/huge pages and upstream failures. UI contracts cover parsing
and routing; lint/build and startup/recreation check the affected applications.

This does not implement UI preview activation, correct hosting URLs, atomic
activation, preservation of the other DB live/preview pointer, or compensation
after serving/DB disagreement. Those remain M3 gates. M1.10 adds the deterministic
compiled-artifact service round trip; no browser/AI publishing journey is claimed.
