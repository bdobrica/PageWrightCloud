# Atomic activation and serving-cache retention (M3.8)

The selected live or preview symlink is replaced with a single same-filesystem
rename. Serving never unlinks the active pointer first. Compiled public output is
validated and extracted into a private staging directory, synced, then renamed to
its immutable version directory before activation. A temporary symlink is created
under the site root and renamed over the selected pointer; the site directory is
synced afterward. Normal completion/error paths remove their temporary directories.

Before cutover, serving validates that the new artifact/public paths are real
directories and the old pointer is absent or a canonical
`artifacts/<exact-version>/public` symlink. Regular files, directories and unexpected
symlinks are not destructively replaced. Descendant site/artifact directory symlinks
are rejected before deployment, activation, retention or whole-site deletion.

If preparing or renaming the pointer fails, the old output remains selected. An
error **after** rename does not prove failure: complete new output may already be
selected. The [M3.7 activating receipt](DEPLOYMENT_RECOVERY.md) retains uncertainty
and recovery retries the same fenced operation. There is no speculative rollback.
An explicit rollback is a new deployment of the desired older immutable artifact,
using a newer sequence and the same atomic activation path; preview remains unchanged
when rolling back live (and vice versa).

## Retention policy

`PAGEWRIGHT_MAX_VERSIONS_PER_SITE` (default 10) is a soft total serving-cache budget,
not a limit that can remove protected content. After a terminal serving receipt,
retention protects:

- The **exact** live and preview versions, never substring matches.
- The current deployment receipt's version in every recognized state, including
  pending/activating and unacknowledged completion.
- Artifacts created, activated or retired within the last minute, allowing a grace
  period for in-flight requests. Activation refreshes the old/new directory timestamps
  without modifying any artifact file or archive digest.
- A legacy staging call's explicitly supplied version while its cleanup runs.

Protected artifacts can exceed the configured limit. Zero keeps only protected or
grace-period entries; negative limits disable cleanup with an error. Other artifacts
are ordered newest first by directory modification time, with a deterministic name
tie-break, and the oldest excess entries are removed. Private `.deploy-*` staging
directories and other dot-prefixed entries are left alone. Unknown artifact entries,
malformed/dangling active pointers, unreadable/corrupt/unknown receipts or filesystem
errors fail closed rather than guessing which versions are safe to delete.

Deployment, pointer replacement, retention and deletion share the artifact manager
writer mutex; the M3.7 handler also serializes the complete fenced operation. Keep
the M3.5 supervisor's exclusive single-writer volume lock enabled. Independent
writers and concurrent operator filesystem edits are unsupported.

Retention removes only disposable **serving-cache copies**, never canonical storage
archives or gateway version history. Evicted versions can be downloaded and validated
again on rollback. A cleanup error after a durable activation receipt is logged as
deferred maintenance; it must not turn a successful activation into a false failure.
Cleanup currently runs on deployment/replay, not on an independent periodic timer.

## Deletion, upgrades and limits

Whole-site removal rejects either active pointer or any deployment receipt before
removing nginx routing or files. The guard and infrastructure-removal callback run
under the artifact writer lock. M3.7's gateway enrollment/deletion serialization and
enrolled-site deletion restriction remain; no destructive “force delete” endpoint
was added. Canonical version deletion remains disabled. Coordinated site deletion
and receipt/tombstone retirement still need a separate protocol; M3.9 should hide
unsupported controls.

Upgrade serving after draining deployments; retain M3.7 gateway/serving compatibility
and all receipt/sequence evidence. No schema change is needed. Existing cache entries
become eligible under the documented budget on the next cleanup; review the configured
limit before upgrading. Canonical storage must remain available for an evicted rollback.

Atomic rename guarantees pointer replacement, not a globally consistent multi-request
browser snapshot: HTML and later assets can span two deployments. The grace period
reduces immediate eviction races but is not a request-lifetime lease. Crash-left hidden
staging directories are retained for operator inspection rather than speculatively
deleted. POSIX rename/directory-sync semantics and the single-writer filesystem layout
are required. Arbitrary disk loss and independently restored volumes remain outside
this acceptance boundary; coordinated backup/restore remains M4.11.

## Verification

Deterministic tests replace the previously skipped cleanup test. Coverage includes
exact live/preview/receipt pins, zero budget, stable timestamp ties, grace-period
overflow, staging preservation, malformed evidence and path indirection rejection.
A continuous filesystem reader observes only complete old/new files through repeated
switches and cleanup. Injected rename failure preserves old output; an injected
post-rename error leaves complete new output visible to a recreated manager, followed
by explicit rollback. Serving tests evict an inactive cached version, re-download it
through a newer fenced rollback, preserve preview and reject stale requests.

The full deterministic compiler/storage/real-nginx integration also exercises
preview/live independence, promotion and a real rollback with lost-acknowledgment
reconciliation. These are filesystem/service tests, not rendered-browser acceptance
(M3.12), and require no paid provider.
