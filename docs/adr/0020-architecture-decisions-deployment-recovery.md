# Deployment consistency and recovery (M3.7)

ADR 0020 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


Publishing/previewing is a durable operation, not an untracked pair of HTTP calls.
Migration 010 stores one current deployment intent per site in PostgreSQL. Its
global sequence increases for each new selection; an unresolved selection retains
its sequence on retry. Reservation locks the site row briefly, before external I/O.
Only one unresolved operation is admitted per site, across **both** targets and
across gateway instances. A competing selection returns HTTP 409; retry the pending
version/target or let recovery finish first. Different sites have independent DB
reservations, though the MVP serving writer serializes deployment work globally.

## Protocol

1. Gateway validates owner, target, version and configured URL, then commits the
   exact site/version/target/sequence intent before contacting serving.
2. `POST /sites/{fqdn}/deployment` persists the sequence in a private
   `.deployment.json` beside, never inside, the site's public output. Serving stages
   and validates the immutable artifact and confirms nginx routing, preserving
   existing aliases and enabled/disabled policy for **both** targets.
3. Serving syncs an `activating` receipt **before** changing the selected symlink.
   It then verifies the symlink and syncs the `completed` receipt. The opposite
   symlink is not touched. Pre-activation failure gets a durable `failed` receipt;
   failure after activation starts remains uncertain and cannot become a clean failure.
4. Gateway accepts only a bounded, identity-matching terminal receipt, and updates
   that intent plus **only** its selected DB pointer in one transaction. Conditional
   sequence matching prevents stale acknowledgments from overwriting newer state.
   Only a confirmed DB commit returns the browser's success URL.

Receipt files are atomically replaced and file/directory synced. The M3.5 hosting
supervisor's exclusive writer lock must remain enabled; multiple independent
serving processes must not share this volume. Once enrolled, a site rejects legacy
unfenced artifact/activation/deletion endpoints. Same-sequence identity conflicts,
older requests and attempts to bypass an unresolved sequence return 409. Replaying
a completed operation verifies its pointer and does not download or activate again.

## Recovery and operational visibility

Gateway scans at startup and every five seconds, taking at most 25 oldest pending
records per pass. Each attempt is bounded by a 30-second client deadline; attempted
records move to the back of the scan order. Recovery replays **only the persisted
identity**, never a guessed newer selection or compensating rollback. Serving
deduplicates replay using its on-disk receipt, including across process restarts.
An interrupted `activating` operation can restage the same immutable artifact and
finish activation; a subsequent storage/nginx outage leaves it uncertain.

The owner-authenticated, `Cache-Control: no-store` endpoint
`GET /sites/{fqdn}/deployment` returns `null` before first deployment, otherwise the
current `sequence`, `site_id`, `fqdn`, `version`, `target` and `status`:

| Status | Meaning and action |
| --- | --- |
| `pending` | Serving outcome or DB confirmation remains unresolved. Recovery is active. Retry the same version/target; do not delete intent/receipt records. |
| `completed` | A verified serving receipt and the selected DB pointer committed together. |
| `failed` | Serving durably confirmed failure before activation. Existing DB pointers remain unchanged; a new request can reserve a new sequence. |

An unavailable/malformed/mismatched receipt or a failed DB commit produces no
success URL. A lost HTTP response can still mean serving changed: inspect this
endpoint rather than treating the UI error as a rollback. The dashboard pointer
remains the last confirmed DB state while an operation is pending. UI presentation
of detailed recovery progress remains M3.10.

Corrupt receipts, a completed receipt whose symlink differs, or a serving sequence
ahead of restored PostgreSQL state fail closed and require operator review.
Missing receipt files after an incomplete volume restore cannot always be
distinguished from first enrollment: do not resume requests against such a restore.
Do not reset the sequence, delete receipts, or manually replay old activation
endpoints. Restore PostgreSQL, deployment receipts, nginx
configuration and artifacts as a coordinated set. Arbitrary disk loss, separately
rolled-back volumes and automatic repair of legacy pre-M3.7 pointer mismatches are
not solved by this protocol; backup/restore acceptance remains M4.11.

## Upgrade and remaining boundaries

Drain gateway deployment requests and upgrade gateway and serving together. Do not
run old unfenced gateways alongside the new protocol. Migration 010 does not infer
deployment history or alter existing pointers; a site's first new selection enrolls
it. Keep the PostgreSQL sequence and private serving receipts across recreation.
Existing preview DNS/config upgrade requirements remain in
[PREVIEW_ACTIVATION.md](0018-architecture-decisions-preview-activation.md).

Site deletion is refused once it has a deployment record (even a failed one), so a
delayed request cannot recreate content after its sequence tombstone is removed.
Deletion of never-enrolled sites is serialized with enrollment and stops on serving
failure. Coordinated deletion/tombstone retirement is not yet implemented; M3.9
should hide unsupported controls. M3.8 adds [atomic pointer replacement and safe
serving-cache retention](0021-architecture-decisions-atomic-activation.md): live, preview, receipt-pinned and
recently switched output is protected, and rollback can restage an evicted copy.
The pointer no longer has a remove/create gap. This is restart/retry reconciliation
and atomic pointer replacement, not instantaneous globally atomic publishing.
Internal service authentication and private preview access remain M4 work.

## Verification

Tests cover concurrent reservations, opposite-pointer preservation, an injected
PostgreSQL pointer-write failure and transaction rollback, restart-style recovery,
stale completion rejection, owner-only status, and deletion guards. Serving tests
cover concurrent identical requests, durable replay through handler recreation,
stale/conflicting sequences, a crash-state receipt after pointer activation,
outage during that recovery, pre-activation failure and corrupt receipts.

The deterministic real compiler/storage/hosting journey drops the serving response
**after real activation**, verifies the DB stays pending with its old selection,
starts a fresh gateway recovery instance, and verifies restored DB/serving agreement
without changing preview. No paid provider is required. Full rendered-browser
acceptance remains M3.12.
