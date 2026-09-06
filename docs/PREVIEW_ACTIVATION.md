# Preview activation (M3.4)

Selecting **Preview in New Tab** calls the owner-checked gateway deployment API
with `{ "target": "preview" }` for the selected immutable build. Gateway downloads
and stages the artifact through serving, activates its preview pointer, ensures
nginx host routing exists, and saves the preview selection before returning
`{ "status": "deployed", "target": "preview", "version_id": "…", "url": "…" }`.
Failures do not return a success URL. An activation/database/reload error can mean
partial activation; the UI says to retry the same version, not that nothing changed.

The browser validates target, version and URL (HTTP/S, matching site hostname,
expected `/preview/` path, no credentials/query/fragment). Only then does it attempt
to open the tab with `noopener,noreferrer`. Async popups may be blocked; a normal
**Open preview** link remains available. The button is guarded while pending;
closing the modal suppresses a late tab opening but does not cancel deployment.

Returned URLs use gateway startup configuration, never request Host/forwarded
headers: `PAGEWRIGHT_HOSTING_SCHEME=http` and `PAGEWRIGHT_HOSTING_PORT=8084` by
default. Configure `https`/`443` behind a TLS proxy, or match your exposed hosting
port. Standard ports are omitted. Site DNS must resolve to the hosting endpoint.
This minimal deployment URL support is brought forward from M3.6; existing Chat
and dashboard links and preview asset/base-path behavior remain M3.6 work.

First preview creates missing site routing even when no live symlink exists.
Existing config is retained byte-for-byte, including aliases and disabled-site
policy, and reloaded. Preview does not create a live symlink. Nil arguments to
`UpdateSiteVersions` now mean “leave that pointer unchanged,” so preview cannot
clear live and publishing cannot clear preview. Gateway checks database write
errors. This narrow prerequisite is brought forward from M3.7; distributed
activation/DB reconciliation and concurrent deployment policy remain there.

## Verified boundary and remaining work

The deterministic integration journey uses the real worker/compiler, immutable
storage and actual nginx: first preview before publish serves compiled HTML while
live returns 404; later preview leaves the live artifact/pointer unchanged. Unit
and HTTP contract tests cover deployment ordering, owner denial, failed artifact
or activation responses, URL validation, deferred navigation, modal closure and
existing nginx policy preservation. No manual HTML seeding or database edits are
used for the first-preview journey.

The integration topology co-locates nginx with serving. **The production root
Compose reload lifecycle is not fixed by M3.4**: its separate nginx container still
requires M3.5's supervised/config-validated/controlled reload and rollback design.
An unavailable reload correctly fails activation instead of opening a success URL.
Atomic symlink replacement/config staging and full rollback remain M3.5/M3.8;
preview nested links/assets and configured URLs across all UI entry points remain
M3.6. Full rendered-browser acceptance remains M3.12. No private preview access
control or safe-generated-content claim is made; pilot hardening remains M4.
