# Identifier and filesystem boundaries (M4.4)

## Platform names

The operator owns `pagewright.io`. This milestone does not verify or change DNS,
the apex redirect, certificates, hosting configuration or private `.env` values.
Local defaults remain unchanged. At M4.9 configure `PAGEWRIGHT_SITE_DOMAIN` to the
chosen owned namespace (`pagewright.io` or a dedicated `sites.pagewright.io`) and
route live `<label>.<namespace>` and preview `preview.<label>.<namespace>` hosts.
An apex redirect can remain independent of those subdomains. TLS, origin and
generated-content isolation still require their own release gates.

Gateway creation lowercases and trims the supplied FQDN and configured namespace.
Exactly one ASCII DNS label is allowed below that namespace: 1–63 characters,
letters/digits with internal hyphens. Apex, arbitrary/nested domains, trailing dots,
wildcards, URLs, whitespace within names and injection syntax are rejected before
database writes. `preview`, `www`, `api`, `app`, `admin`, `auth`, `assets`, `cdn`,
`mail`, `status`, `support`, `ns1`, `ns2` and `xn--` labels are unavailable.
UI checks match the server but are not an authorization boundary.

Serving and nginx require canonical lowercase DNS labels, a dotted hostname and
at most 245 bytes (leaving room for `preview.`). They reject the preview prefix as
a primary site. Existing syntactically valid legacy domains remain addressable by
the authenticated control plane: serving does not claim DNS ownership or restrict
old sites to the creation namespace. Changing the namespace never renames data.

## IDs and paths

Site/version transport IDs are opaque and case-sensitive, not necessarily UUIDs:
`[A-Za-z0-9][A-Za-z0-9._-]{0,199}`. No trimming, decoding or path cleaning repairs
invalid IDs. Manager rejects invalid identities before queue reservation; worker
launch, storage API/backend and transport clients independently validate them.
`initial` remains the reserved bootstrap target. Generated UUIDs are unaffected.
Rebuild the worker with `docker compose build worker` and update old explicit image
pins to `pagewright-worker:m4.5`; upgrade services together to align validation.
Previously accepted IDs longer than 200 bytes require operator review before an
upgrade; do not silently rename existing artifacts or edit database identities.

Storage verifies lexical containment and rejects existing symlinks along artifact,
metadata and log paths before reads, mkdirs and immutable writes. Serving checks
real directory ancestors before receipt access and deployment operations; receipt
files must be regular files. Its deliberate live/preview symlinks retain the
existing constrained artifact-target validation. Legacy downloads use unique
temporary files instead of names built from request fields.

Paths interpolated into nginx must be absolute, canonical and drawn from the safe
ASCII path alphabet. Dot segments, duplicate separators, root-only paths and nginx
syntax/variable injection are rejected, including configured maintenance paths.
Startup validates configuration before hosting initialization. Nginx filenames
cannot target its reserved maintenance/health files through site operations.

These checks assume private operator-owned volumes with one supported writer and
no concurrent external filesystem mutation. They are not a hostile-local-user
race-proof filesystem API or protection against a Docker administrator. Do not
mount generated-source workspaces over storage/hosting roots. Archive expansion,
link/type/size limits and compiler budgets are recorded in [resource limits](RESOURCE_LIMITS.md); authentication
continues to be required as described in [internal authentication](INTERNAL_AUTH.md).

## Verification

Focused Go tests cover malformed/injected hostnames and nginx paths, reserved
platform names, wrong namespace suffixes, invalid IDs before queue/dependency use,
symlinked storage ancestors/final files, and serving receipt containment. Rejection
tests verify outside files remain unchanged and no config/receipt is created.
UI contracts cover reserved names and the owned namespace. Run `make test-all`,
`make test-integration` and the [browser acceptance](BROWSER_ACCEPTANCE.md) journey
to check normal authenticated workflows alongside these negative tests.
