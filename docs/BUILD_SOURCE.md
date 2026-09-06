# Default build source (M2.5)

New build submissions select their source from the owner-checked site's storage
version list, not the gateway's potentially stale `versions.status` rows:

1. Latest manifest-committed build other than `initial`.
2. The site's live version, if no completed build is listed.
3. Bootstrap (`initial`).

Ordering uses the immutable manifest creation timestamp descending, then build ID
ascending for ties, matching version history. A published version remains a valid
completed build; publishing or rolling back does not erase newer draft history.
Preview pointers do not affect selection. Incomplete uploads are excluded by
storage's manifest-last listing. A committed artifact remains usable if its
manager completion callback is lost; lifecycle reconciliation is separate work.

Storage errors or invalid list entries fail the new submission with HTTP 502,
before reservation or manager dispatch. They do not silently select older content.
Legacy live/bootstrap fallbacks retain their existing behavior; an absent archive
still fails worker fetch rather than creating an empty site.

Selection occurs after instruction generation, immediately before reservation.
The chosen `source_version` is persisted with the job and target version. Same-key
retries replay that base even after restart, a newer draft or a storage outage.
The existing manager single-active-job guard still controls concurrent requests;
selection is a snapshot, not an automatic rebase when a queued job starts.

The build response already includes `source_version`. Chat displays that exact
version when accepted and on completion, rather than guessing from live/preview.
Durable browser job history and reload recovery remain M3; this does not introduce
a manual base picker or paid-provider acceptance.

Verification: selector tests cover ordering, ties, bootstrap/live fallbacks and
invalid/unavailable storage. HTTP/database tests cover restart/retry base pinning
and no dispatch on lookup failure. The isolated service round trip performs two
source-only deterministic edits through the production worker compiler, checks
both edits in preserved source and generated HTML, and verifies the first draft
and live pointer stay unchanged before publishing.

```sh
make test-all
make test-integration
cd pagewright/ui
npm run test:contracts
npm run lint -- --max-warnings=0
npm run build
```
