# Initial site source and retry (M1.6)

Authenticated `POST /sites` accepts `fqdn` and `template_id: "starter"`. The old
UI identifier `template-1` is explicitly mapped to `starter`; unsupported templates
are rejected before writes. Domains are trimmed/lowercased and checked for DNS
label syntax/length. This is not proof of domain ownership or alias uniqueness.

Creation reserves a new, disabled site with `initialization_status: "pending"`
and its exact bootstrap archive/manifest/log bytes in one PostgreSQL transaction.
Migration 008 adds this state and the `site_bootstraps` table. No storage upload
occurs unless the reservation commits.

The gateway then uploads artifact → private log → manifest through the canonical
storage API, using the reserved bytes. It atomically records the database
`initial` version as completed and the site as `ready` only after storage confirms
all writes. Neither live nor preview is set; no hosting enablement occurs.
Creation returns `201` with the site only after initialization is ready.

## Seed content

The gateway binary embeds versioned starter source:

- `content/site.json`: valid `site_name`, author and language configuration.
- `content/home/index.md`: discoverable home page for the bundled starter theme.

The archive is deterministic (fixed tar metadata and gzip header, stable file
ordering). The manifest identifies site, `build_id: "initial"`, creation time,
`template_id: "starter"`, `bootstrap_revision: 1` and `kind: "source"`.
`compiled` and `checks_passed` are explicitly false. Source is not fabricated
compiled output: no `public/`, credentials, execution instructions or theme code
are included. Trusted theme code remains bundled separately for the compiler.

New-site builds can safely use the existing `initial` fallback once ready. Builds
and enablement reject pending initialization before downstream calls. Selecting
the newest draft instead of live/bootstrap remains M2.5.

## Retry contract

The normalized FQDN is the durable creation identity. Repeating creation for the
same owner and starter template returns the same site/version, including when
requests race. Another owner or an incompatible existing/legacy site receives
`409`; no existing site is overwritten. Both accepted template identifiers refer
to the same canonical starter reservation.

Exact archive and JSON bytes remain in PostgreSQL after readiness. Retries do not
regenerate timestamps, repack source or reinterpret a newer embedded seed. This
preserves M1.5 byte identity across gateway restart, partial uploads, lost storage
acknowledgements and failed DB confirmation. Concurrent uploads are identical
idempotent writes, not competing replacements. Ready retries return the saved site
without uploading again; restored/lost storage volumes require a separate recovery
policy, not implicit replacement.

Reservation/transport/confirmation failure returns `503` with same-domain retry
guidance. A storage `409` is surfaced as a conflict requiring operator repair;
the gateway never overwrites immutable initial data. Pending sites remain visible
in the dashboard with **Resume Setup**, which pre-fills the creation form. Build
and enable controls do not proceed until setup completes.

## Compatibility and limits

Existing sites are labeled `legacy`; migration does not invent bootstrap bytes,
change live/preview references, or certify their historical source. Their existing
build behavior remains unchanged and may still lack an `initial` artifact.
Reusing a legacy domain through creation is rejected rather than silently repairing
or overwriting it. The low-level DB CreateSite helper likewise retains legacy
semantics; the supported creation workflow is the authenticated HTTP endpoint.

Storage commit and database readiness are not one distributed transaction. A
manifest can be committed while DB confirmation remains pending; retry reconciles
this safely. Cancellation/site deletion may leave unreferenced immutable storage
objects. Retention, tenant/internal-service hardening, complete archive validation,
browser build/publish behavior and real AI execution remain later milestones.

## Verification

Unit tests check deterministic archive bytes and the seed file/config set.
`make test-compiler-smoke` compiles both the existing three-page fixture and this
bootstrap source with bundled `starter`. Integration tests use real PostgreSQL
and storage for upload-phase failures, lost acknowledgement, failed final DB
transaction, same-domain concurrency, owner/template rejection and pending-build
gating. Migration tests cover upgrade from 007, legacy preservation, reservation
rollback and reopening the DB connection with unchanged saved bytes.

Startup/recreation smoke creates a real bootstrapped site through gateway, fetches
its initial archive/manifest, retries creation with the same identity, and verifies
state after container recreation. UI source contracts/lint/build cover starter
selection and recovery wiring; they are not a browser end-to-end test.
