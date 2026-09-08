# Application and generated-content boundaries (M4.6)

## Configuration and upgrade

Set `PAGEWRIGHT_APP_ORIGINS` to an exact comma-separated list of application UI
origins, e.g. `https://app.pagewright.io`. The local default is
`http://localhost:3000`; add `http://localhost:5173` explicitly for Vite development
if needed. Origins include scheme and non-default port, but no path, trailing slash,
credentials, query, fragment, wildcard or `null`. Use lowercase ASCII hostnames and
omit default ports as browsers do. IPv6 literals are not supported by this MVP
configuration parser. Invalid configuration stops gateway startup before DB access.

Gateway refuses origins in the generated namespace, except the reserved `app`,
`api`, `www` labels or namespace apex, which users cannot claim through MVP creation.
With `PAGEWRIGHT_SITE_DOMAIN=pagewright.io`, use `https://app.pagewright.io` for the
UI, `https://api.pagewright.io` for the gateway, and `<site>.pagewright.io` /
`<site>.preview.pagewright.io` for generated content. A dedicated
`sites.pagewright.io` namespace is also possible. Audit legacy deployments and aliases
for collisions with application hosts before upgrading; valid legacy records are
not automatically renamed. All application origins must be operator-controlled.

Rebuild the gateway and UI together. `VITE_PAGEWRIGHT_API_URL` is compiled into both
the UI API client and its CSP connection allowlist. It must be an absolute HTTP(S)
origin (an optional trailing slash is accepted), without credentials/path/query or
fragment. The build validates it before generating nginx directives. Restart the
hosting edge to load its changed configuration; it applies headers to old managed
site configs too, without changing activation receipts or nginx template migration.
No worker rebuild is necessary beyond the existing `m4.5` image.

The optional local-domain overlay explicitly permits `http://pagewright.io:3000`
and `http://localhost:3000`. It is a local routing diagnostic, not a production DNS
change. The user's ownership of `pagewright.io` is recorded, but this milestone does
not change its apex redirect, DNS, HTTPS or private `.env`. HSTS and HTTPS enforcement
remain M4.9 work; do not enable HSTS before certificate/routing acceptance.

## Gateway and sockets

The origin policy wraps the entire public router, including errors/unmatched paths,
before authentication, throttling or handler side effects. Supplied but unlisted,
empty, multiple or opaque origins get 403 with no allow-origin header. Request Host
and forwarding headers cannot expand the list. Allowed origins get an exact
allow-origin value, never a wildcard; `Vary: Origin` and no-store prevent shared
response confusion. Cookie credential sharing is not enabled.

Preflight permits reviewed GET/POST/PUT/DELETE methods and Content-Type,
Authorization and Idempotency-Key headers, without invoking application handlers.
Requests without Origin remain usable by CLI/internal clients, but retain all normal
authentication/ownership checks. Origin checks are browser protection, not identity:
non-browser callers can omit/forge Origin, so service tokens and user JWTs remain
required. WebSockets stay retired: allowed/no-origin requests receive 501, disallowed
origins 403, and neither response upgrades or accepts socket credentials.

## Response policies

| Surface | Enforced policy |
| --- | --- |
| Application UI, including assets and errors | Self-only scripts, self/inline styles, self/data images, self fonts; connections only to self and configured API; no embedding, objects or base-URL overrides; self-only forms |
| Gateway | Non-rendering CSP, no embedding, nosniff, no-referrer and no-store, including denied origins |
| Generated-content edge, including assets/errors | Local/inline scripts and styles; self/data images; same-origin connections; no frames, embedding, objects, base-URL overrides or form submissions |

UI/generated responses also use opener isolation, Origin-Agent-Cluster and a denied
camera/microphone/geolocation/payment/USB permissions policy. The edge replaces
upstream security headers so legacy templates cannot weaken its policy. The UI
repeats its security include in nginx locations that add caching/content headers:
nginx's default [header inheritance rule](https://nginx.org/en/docs/http/ngx_http_headers_module.html)
would otherwise omit parent security headers there.

Browser [CSP connections](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/connect-src)
and [same-origin boundaries](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Same-origin_policy)
complement gateway denial. Generated HTML/JS remain untrusted active content, not
sanitized application code. External embeds, API calls and form integrations are
not supported by this static MVP policy. Sibling subdomains are different origins
but can share a site/cookie domain: do not introduce parent-domain session cookies
or document.domain relaxation. Existing bearer tokens remain origin-local; this is
not proof against XSS, phishing, browser vulnerabilities or an operator routing two
services onto one origin. The supported root edge must remain the sole published
generated-content listener. Do not publish serving's internal management listener.

## Verification

Go tests exercise startup rejection, exact matches, scheme/port/suffix mismatches,
generated/preview/null/duplicate origins, forwarding-header spoofing and preflight
denial before handlers. UI tests validate CSP generation and configuration injection
rejection. The disposable browser journey verifies login/build/deployment under CSP,
exact CORS responses, retired sockets, headers on UI assets and generated errors,
allowed UI API access, and blocked generated API access. Existing recovery and
internal credential-misuse acceptance remains in place. Remaining M4 gates still
block remote testers.
