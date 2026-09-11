# Text-only MVP capabilities

ADR 0022 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


This release is MVP-only. There is no environment switch that enables unfinished
features; supplying OAuth credentials does not enable Google sign-in.

## Supported path

Register/sign in with email and password, create a platform subdomain using the
starter template, send a text request, follow durable build history, preview,
publish and roll back completed versions. Existing enable/disable and download
actions remain available.

The gateway owns the creation namespace:

- Set `PAGEWRIGHT_SITE_DOMAIN` on the gateway (default `pagewright.dev`).
  The local-domain Compose override sets `pagewright.io`.
- Public `GET /capabilities` returns `mode: "mvp"`, `site_domain`, and false
  flags for attachments, custom domains, aliases, OAuth and site deletion.
  It is non-secret and uses `Cache-Control: no-store`.
- The UI fetches this domain with a timeout and retry control. Failed or malformed
  configuration disables creation; there is no build-time domain fallback.
  The old `VITE_PAGEWRIGHT_DEFAULT_DOMAIN` setting has been removed.
- Creation accepts one DNS label under that domain, normalizes case/outer
  whitespace, and rejects nested names, foreign suffixes and the base domain.
  Labels are 1–63 lowercase ASCII letters/digits with internal hyphens.
  `app`, `api`, `www` and `preview` are reserved.
- Configure DNS/hosts and, for the pilot, TLS separately for both the site and
  `<label>.preview.<namespace>`. This setting does not provision DNS, verify ownership of
  arbitrary domains or issue certificates.

## Disabled paths

| Action | Gateway behavior |
| --- | --- |
| Google login and callback | 501, no redirect, cookie, token exchange or account creation |
| List/add/delete aliases | Authenticated routes return 501 without database or serving calls |
| Delete a site | Authenticated route returns 501 for every site |
| Attachment/multipart build | Explicit non-JSON media types return 415 before storage/provider/manager calls |
| JSON attachment fields | Strict request decoding returns 400 for unknown fields such as files/attachments |
| Arbitrary-domain creation | 400 before reservation or bootstrap writes |

Disabled 501 responses are no-store. Authentication middleware still applies to
protected routes. UI attachment/alias components and their client paths are removed;
Google and site-delete controls are absent. Text input is required; submissions
still use the existing idempotency key and clarification protocol. JSON requests
without a Content-Type remain accepted for compatibility; an explicit type must
be `application/json` (parameters such as charset are allowed).

Deletion is conservatively unavailable even for undeployed sites: coordinated
database/storage/serving cleanup is not implemented. Existing database deployment
record and serving active-artifact/receipt guards remain intact. No deletion or
data migration is performed by this change.

## Upgrade and legacy sites

Upgrade gateway and UI together. Set the gateway domain to the platform namespace
previously used by the UI before resuming creation. Existing sites, aliases,
artifacts and deployments are preserved; owner-scoped reads/builds/deployments for
existing sites are not filtered by the new creation policy. Legacy aliases are not
newly verified or editable.

Resume Setup locks the original name. If it is outside the current namespace,
creation stops with an operator-recovery message. Restore its original supported
namespace configuration when appropriate; do not rename rows or delete retained
deployment evidence to bypass this policy. Arbitrary legacy custom-domain
bootstrap recovery requires a future, separately reviewed migration path.

This is an MVP product boundary, not the M4 security gate. Internal service
authentication, signup restrictions, reset-email delivery, comprehensive origin/
hostname validation and pilot TLS remain separately tracked. Keep the development
stack private.

## Verification

Handler tests use nil dependencies to demonstrate that disabled routes, invalid
names and attachment payloads cannot reach database/provider/storage/serving work.
UI contracts cover namespace parsing, invalid resume names, text-only submission
identity and removal of unsupported controls. Integration tests configure the
test namespace and retain real bootstrap/build/preview/publish/rollback coverage.
Production-Compose smoke checks capabilities and direct API rejection before and
after recreation, alongside persisted site/artifact checks.

These UI checks are contracts, source wiring, lint and build checks, not rendered
browser acceptance; that remains M3.12.
