# Immutable versions and disabled deletion (M1.5)

ADR 0005 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


Artifact, private-log and manifest files are write-once from their first
publication, including partial versions and legacy files. M1.4's manifest-last
completion boundary is stable because prerequisites cannot be replaced through
storage APIs during or after commit.

| Request | Result |
| --- | --- |
| First complete upload | `201` |
| Same length and SHA-256 digest retry | `201`, existing file preserved |
| Different bytes at an existing path | `409`, original preserved |
| Manifest before artifact/private log | `409`, remains unlisted |
| Storage artifact DELETE | `405`, unsupported |
| Authenticated gateway version DELETE | `501`, disabled without storage/DB calls |

The UI has no version deletion action or client method. This uses M1.5's explicit
“disable deletion” option. All versions, including live/preview, are protected
from user-requested version deletion. Future deletion requires coordinated
active-version protection, metadata cleanup and retention rules. Existing site
deletion and serving-cache cleanup are separate workflows.

## Publication and concurrency

Supported deployment: Linux local filesystem/Docker named volume. A request
writes a unique sibling `.upload-*` file, checks copy/fsync/close, then publishes
using an atomic no-replace hard link. No check-then-overwrite rename or
process-local lock is used. Losing writers compare size and SHA-256 with the
existing regular file: identical retries succeed, others conflict. Timestamped
event files use the same write-once primitive.

The temporary name is removed and the containing directory and ancestors through
the storage root and its parent are synced. Manifest commit first syncs artifact
and log directories, including prerequisites published by a concurrent request
that has not yet acknowledged success. Unsupported hard-link/directory-fsync
operations fail rather than falling back to overwriting. Network filesystems and
object stores need separately verified backends.

An error or lost response after publication can leave a visible file despite
missing acknowledgement. Do not remove a winner to roll back that uncertainty.
Retry the original bytes to finish sync and receive success. This is not a
cross-service transaction; manager reconciliation remains M2. Hardware power-loss
durability has not been fault-injected.

## Compatibility and limits

Preserve original gzip and JSON for retries. Changed JSON whitespace/key order or
`created_at` conflicts; repacking may change gzip bytes too. Use a new target
version for changed output. Partial versions retain their write-once files and
stay unlisted until required files are supplied. Existing malformed legacy files
are neither replaced nor automatically certified complete.

Normal failure paths remove temporary files. Process death can leave hidden
`.upload-*` names; they are not version entries or API-addressable archive paths.
No online sweeping is performed because it could race a writer. Offline orphan
cleanup and retention need a later explicit maintenance policy.

The volume and its directories must be trusted. Guarantees cover cooperating API
writers, not manual filesystem mutation, hostile symlink replacement or a storage
administrator. Internal authentication, archive validation and publication safety
remain M4/M1.7/M3 respectively.

## Verification

Tests cover failed-copy cleanup, retries/conflicts before and after commit/reopen,
concurrent independent backend instances and competing subprocesses, HTTP conflict
preservation, and a disabled gateway handler with no usable DB/client. A UI source
contract guards against reintroducing deletion. Startup smoke verifies retries,
all three overwrite conflicts, both deletion responses and preservation of the
original artifact/metadata before and after container recreation. Shared archive
integration still checks worker/gateway/serving byte and extraction compatibility.
