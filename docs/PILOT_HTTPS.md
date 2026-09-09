# Pilot DNS and HTTPS — M4.9 in progress

**Superseded design note:** the operator selected individual Let's Encrypt
certificates with HTTP-01, not wildcard/DNS-01 certificates. Use the current
[HTTP-01 installation and acceptance runbook](PILOT_HTTP01.md). The material below
records the earlier wildcard proposal only; do not follow its DNS/API setup steps.

The physical server at `135.181.209.167` can host the closed pilot alongside the
personal blog. PageWright uses only pagewright.io names. Do not change ublo.ro
routing, the apex/www redirect, or the existing apex certificate.

## Public hostname contract

| Purpose | Hostname | Required certificate name |
| --- | --- | --- |
| UI | `app.pagewright.io` | `*.pagewright.io` |
| API | `api.pagewright.io` | `*.pagewright.io` |
| Live site | `<site>.pagewright.io` | `*.pagewright.io` |
| Preview | `<site>.preview.pagewright.io` | `*.preview.pagewright.io` |

The two wildcard names can share one dedicated pilot certificate, separate from
the existing `pagewright.io` / `www.pagewright.io` certificate. A TLS wildcard
matches exactly one leftmost label, unlike DNS wildcard synthesis.
See [RFC 9525](https://www.rfc-editor.org/rfc/rfc9525.html#section-6.3).

## DNS

`app` and `api` have A records to `135.181.209.167`. `*` is a CNAME to
`www.pagewright.io`, which resolves to that IP. Both authoritative nameservers
were verified on 2026-09-08 for app/api and arbitrary live/old-preview names.
The approved `m49-probe.preview.pagewright.io` layout also resolved to the same
IP on both authoritative servers when checked on 2026-09-09 (local time).

Add an explicit `*.preview` CNAME to `www.pagewright.io` (or A to the same IP).
Do not rely on the apex wildcard for preview resolution: creating the DNS-01
validation record `_acme-challenge.preview` introduces an intervening DNS node
that can prevent that fallback. A successful wildcard-derived DNS answer alone
does not prove the explicit `*.preview` record exists; check the zone settings.
Verify a fresh `<site>.preview.pagewright.io`
name on both authoritative servers after adding the record and during validation.
Existing app/api/apex/www records remain unchanged.

## Certificate issuance and renewal — operator gate

The current certificate covers only apex/www. Host Nginx reads the Server-Tools
`.cert` symlinks, which point at `/etc/letsencrypt/live/pagewright.io/`.
The operator confirms Certbot renewal runs from cron. Leave this lineage intact.

Wildcard issuance requires DNS-01. Choose automated DNS updates using narrowly
scoped credentials or delegate only the ACME challenge names to a validation
zone; do not put a registrar-wide credential in PageWright's `.env` or containers.
Provider/API support is not yet confirmed. Manual TXT entry is not unattended
renewal, and cron alone cannot automate manual challenges.
See [Let's Encrypt DNS-01 guidance](https://letsencrypt.org/docs/challenge-types/#dns-01-challenge).

Before requesting production certificates, prepare provider-specific instructions,
test issuance against ACME staging, then obtain the separate pilot certificate.
Verify renewal with a dry run and a deploy hook that validates Nginx before reload.
Never publish private keys or paste credentials into chat.

## Reverse proxy and application — still pending

Keep existing host Nginx as the public 80/443 listener. Do not run a competing
public proxy or regenerate all Server-Tools sites. Prepare a dedicated pilot
configuration outside its generated per-site files and check coexistence before
an operator-approved reload. Keep pilot files outside the auto-discovered
`/mnt/www/<domain>/<subdomain>` tree.

The eventual pilot Compose configuration must bind UI, gateway and generated-site
edge ports to loopback only; infrastructure remains private. Route app to UI,
api to gateway and only canonical live/preview names to the generated edge.
Preserve Host and generated-content security headers, reject unknown hosts,
redirect HTTP to HTTPS, and serve only the dedicated pilot certificate on these
virtual hosts. Keep the worker sandbox and resource limits unchanged.

Gateway advertises `PAGEWRIGHT_HOSTING_SCHEME=https`,
`PAGEWRIGHT_HOSTING_PORT=443`, and `PAGEWRIGHT_SITE_DOMAIN=pagewright.io`.
Use exact `PAGEWRIGHT_APP_ORIGINS=https://app.pagewright.io`; build UI with
`VITE_PAGEWRIGHT_API_URL=https://api.pagewright.io`. These settings do not create
TLS listeners. Review trusted proxy/IP throttling behavior before remote access.

Public acceptance must verify certificate chains/SANs, redirects, API/UI access,
new-site live and preview routes, their isolation/security headers, denied aliases,
and unchanged apex/blog behavior. No public deployment or HTTPS acceptance is
implied by the local hostname change. M4.9 remains open until these gates pass.

## Local development and upgrade

Map `demo.pagewright.io` and `demo.preview.pagewright.io` to the local Docker host
in local DNS/hosts, not public DNS. Default URLs use HTTP on port 8084; the optional
local-domain overlay still uses `pagewright.io:3000` for UI and `:8085` for API.
The disposable browser suite uses `example.localhost` and loopback ephemeral ports;
it makes no public DNS changes or paid provider calls.

Rebuild gateway, serving and UI together and follow the
[conservative v2-to-v3 routing migration](PREVIEW_ACTIVATION.md#existing-installation-upgrade).
Custom domain creation and domain-alias mutation remain unsupported; this change
does not enable either feature.
