# Preview activation and hosting URLs (M3.4–M3.6)

Selecting **Preview in New Tab** calls the owner-checked gateway deployment API
with `{ "target": "preview" }` for the selected immutable build. Gateway downloads
and stages the artifact through serving, activates its preview pointer, ensures
nginx host routing exists, and saves the preview selection before returning
`{ "status": "deployed", "target": "preview", "version_id": "…", "url": "…" }`.
Failures do not return a success URL. An activation/database/reload error can mean
partial activation; the UI says to retry the same version, not that nothing changed.

The browser validates target, version and URL (HTTP/S, `preview.<site-fqdn>` hostname,
root `/` path, no credentials/query/fragment). Only then does it attempt
to open the tab with `noopener,noreferrer`. Async popups may be blocked; a normal
**Open preview** link remains available. The button is guarded while pending;
closing the modal suppresses a late tab opening but does not cancel deployment.

Returned URLs use gateway startup configuration, never request Host/forwarded
headers: `PAGEWRIGHT_HOSTING_SCHEME=http` and `PAGEWRIGHT_HOSTING_PORT=8084` by
default. Configure `https`/`443` behind a TLS proxy, or match your exposed hosting
port. Standard ports are omitted. Site create/list/detail responses expose
`live_url` and `preview_url` using the same policy as deployment responses.
Dashboard and Chat validate and use these URLs; undeployed, disabled or unavailable
destinations are disabled buttons. Chat refreshes its links after successful deployment.

Both `<site-fqdn>` and `preview.<site-fqdn>` must resolve to the hosting endpoint.
For local development, map e.g. `demo.pagewright.io` and
`preview.demo.pagewright.io` to the Docker host. With default settings the URLs are
`http://demo.pagewright.io:8084/` and `http://preview.demo.pagewright.io:8084/`.
For HTTPS, provide certificates covering **both** names; a `*.pagewright.io`
certificate does not cover `preview.demo.pagewright.io`. No DNS or TLS provisioning
is performed automatically. Hosting scheme/port settings advertise an existing
endpoint; they do not configure TLS or change the Docker port mapping.

Preview is a separate virtual host rooted at the preview artifact's `public/`
output. Live aliases route only to live. Root-relative navigation, nested pages,
theme assets and page-local images therefore stay on the selected host, with no
HTML rewriting or recompilation. Directory redirects are relative, retaining the
external scheme/port. `/preview/{build_id}` is not a version-viewing endpoint.
Promotion selects the same immutable artifact for live; it does not rebuild it.

First preview creates missing site routing even when no live symlink exists.
Existing v2 config is retained byte-for-byte, including aliases and disabled-site
policy, and reloaded. Preview does not create a live symlink. Nil arguments to
`UpdateSiteVersions` now mean “leave that pointer unchanged,” so preview cannot
clear live and publishing cannot clear preview. Gateway checks database write
errors. This narrow prerequisite is brought forward from M3.7; distributed
activation/DB reconciliation and concurrent deployment policy are now implemented
by the [M3.7 durable deployment protocol](DEPLOYMENT_RECOVERY.md).

## Existing-installation upgrade

Rebuild/recreate gateway, serving and UI together. Re-activate Preview for each
site to transactionally migrate the old generated path-based nginx config; this
preserves aliases and disabled status. Existing files are **not** rewritten at
startup. An unrecognized/custom legacy config fails migration and requires operator
review; no guessed configuration replaces it. Reload failure rolls back migration
under the M3.5 journal/recovery protocol.

The first DNS label `preview` is reserved for hosting previews, for both site names
and aliases; site names are limited to 245 characters so the derived name fits DNS.
New collisions are rejected. Existing conflicting nginx host claims block activation
before changing the config. Audit old sites/aliases in this namespace before upgrading
and resolve them explicitly; no existing site or alias is silently renamed/deleted.
Old bookmarked `/preview/` URLs are no longer hosting routes after migration (they
may still be ordinary content routes if a site defines such a page).

## Verified boundary and remaining work

The deterministic integration journey uses the real worker/compiler, immutable
storage and actual nginx: first preview before publish serves compiled HTML while
live returns 404; later preview leaves the live artifact/pointer unchanged. Both
hosts serve byte-identical artifact assets and nested routes; promotion preserves
the selected archive and preview HTML. Unit
and HTTP contract tests cover deployment ordering, owner denial, failed artifact
or activation responses, URL validation, deferred navigation, modal closure and
existing nginx policy preservation. No manual HTML seeding or database edits are
used for the first-preview journey.

At M3.4 handoff, production root Compose still lacked reload coordination. M3.5
now supplies the [supervised hosting lifecycle](HOSTING_LIFECYCLE.md), config
validation, acknowledged reloads, rollback and restart recovery. Integration uses
that production supervisor and fixed public proxy. Routing is confirmed before
changing an artifact pointer; unavailable reloads fail without opening a success URL.
M3.7 reconciles durable intent/receipts and preserves the opposite pointer; it does
not promise a compensating rollback. Atomic symlink replacement remains M3.8.
Full rendered-browser acceptance remains M3.12. No private preview access
control or safe-generated-content claim is made; pilot hardening remains M4.
