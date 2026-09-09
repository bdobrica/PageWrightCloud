# PageWright pilot: HTTP-01 certificates and host Nginx

M4.9 remains open until operator installation, staging/production issuance,
renewal and public acceptance pass. Local tests do not imply remote deployment.
This replaces the wildcard/DNS-01 proposal: **no Namecheap API or DNS credentials**.
The operator confirmed the requested DNS records, including `*.preview`.

## Architecture and limits

Host Nginx retains ports 80/443. The pilot Compose overlay publishes only
127.0.0.1:3000 (UI), :8085 (gateway), and :8084 (generated content).
Use the fixed project `pagewright-pilot`; infrastructure ports stay private and
worker isolation is unchanged. A host-root controller reads only initialized
site FQDNs from that project's PostgreSQL container. It validates one lowercase
label under pagewright.io, excluding app/api/www/preview. It never takes issuance
requests from incoming Host headers or accepts arbitrary aliases.

Certbot uses HTTP webroot validation. The challenge directory belongs to the
host, never generated content. One `pw-platform` certificate covers app/api;
each `pw-site-<label>` certificate covers `<label>.pagewright.io` and
`<label>.preview.pagewright.io`. Existing apex/www and blog certificates and
their cron remain untouched. Dedicated production and staging Certbot directories
live under `/var/lib/pagewright-tls`; application containers see only its public
readiness directory, mounted read-only, never certificates or keys.

Only exact registered hosts get challenge routes. Pending HTTPS and unknown
pagewright.io subdomains reject the TLS handshake; pending HTTP returns 503,
unknown HTTP returns 404, and ready HTTP redirects to its fixed HTTPS hostname.
The regex fallback does not change exact existing apex/www/blog vhosts. Audit
existing subdomain conflicts before installation: another exact host declaration
can override the pilot configuration. No blanket wildcard application origins.

Each run permits at most two issuance attempts, ten attempts per 24 hours, and
one attempt per certificate per hour, persisted before calling Certbot. This is
a conservative pilot throttle, not a substitute for CA limits. Up to 500 sites
are accepted; larger inventories fail closed and need capacity review. Certificate
issuance is triggered at site initialization, not every build or publish.
The timer retries failures; inspect the private Certbot logs for diagnostics.
New certificates share Let's Encrypt's registered-domain rate limits; use staging
for tests. See [rate limits](https://letsencrypt.org/docs/rate-limits/).

Config replacement is atomic and journaled. `nginx -t` and reload gate installation;
failed reload restores the old file, and failed recovery retains the journal and
blocks subsequent writes. The next run recovers before trying again. Root-owned
state/config directories must not be writable by application users. Do not edit
generated pilot config manually or run two controllers.

Readiness requires both real, trusted TLS handshakes and vhost probes on loopback.
A ten-minute readiness lease fails closed when the controller stops. Gateway
withholds live/preview links and returns 503 with Retry-After before deployment
mutation while provisioning; UI asks the user to refresh. Building can continue.
This checks TLS routing, not upstream application health. Existing direct URLs
may remain reachable during controller failure. HSTS is host-only, one day,
without includeSubDomains or preload. Existing upstream security headers survive.

Gateway deliberately still ignores forwarded client IPs: unauthenticated clients
share the proxy-source throttle bucket (including the existing 10/minute auth
limit). This is conservative for a small closed pilot but needs explicitly scoped
proxy trust before larger use; do not globally trust X-Forwarded-For.

## Operator preflight — do not install blindly

Read-only host checks on 2026-09-09 found Python 3.13.5, OpenSSL 3.5.5,
Certbot 5.8.0, Nginx 1.26.3 (`/usr/sbin/nginx`) and Compose 5.1.3. The expected
conf.d include exists and ports 3000/8084/8085 were free. The pilot checkout,
state directory and dedicated Nginx config did not exist. This is not a complete
shared-vhost collision audit or deployment approval; recheck immediately before
installation. No server files were changed by these checks.

1. Use an independently reviewed checkout at `/opt/pagewright-pilot`, outside
   Server-Tools' `/mnt/www/<domain>/<subdomain>` discovery tree. Do not regenerate
   all Server-Tools sites; their container IP allocation/configuration may change.
2. Verify Python 3.8+, OpenSSL with `x509 -checkhost`, Certbot with webroot support,
   Nginx supporting `ssl_reject_handshake` (1.19.4+), systemd, Docker and Compose
   2.24.4+ (`!override`). Confirm `/etc/nginx/conf.d/*.conf` is included inside
   `http {}` and there is no existing `/etc/nginx/conf.d/pagewright-pilot.conf`.
   The dedicated file must survive Server-Tools regeneration.
3. Confirm host ports 3000/8084/8085 are free and external 80/443 are reachable.
   Check A/AAAA resolution of app/api and a fresh live/preview name; do not leave
   an AAAA record pointing to an unreachable server. Record current apex redirect
   and blog HTTPS behavior for comparison. Do not open extra public ports.
4. Prepare a private `.env.pilot` with production-strength database, Redis, service
   and JWT secrets; closed signup and zero AI allowance initially. Do not copy test
   credentials or paste secrets in chat. Preserve the reviewed worker AppArmor
   profile and build its existing production image. Real SMTP/provider activation
   is a separate operator gate, not required for certificate validation.

## Installation sequence — operator-run after preflight review

Run from `/opt/pagewright-pilot`. These commands install only dedicated files;
first confirm targets do not contain existing operator files. Retain a private
backup of host Nginx configuration before proceeding. No general sudo access is
needed for the coding agent.

```sh
sudo install -d -m 0755 /var/lib/pagewright-tls/public /var/lib/pagewright-tls/webroot
sudo install -d -m 0755 /usr/local/libexec
sudo install -o root -g root -m 0755 scripts/pilot_tls.py /usr/local/libexec/pagewright-pilot-tls.py
sudo install -o root -g root -m 0644 deploy/pagewright-tls.service deploy/pagewright-tls.timer deploy/pagewright-tls-renew.service deploy/pagewright-tls-renew.timer /etc/systemd/system/
sudoedit /etc/pagewright-tls.env
sudo chmod 0600 /etc/pagewright-tls.env
```

The environment file needs just `ACME_EMAIL=your-operator-email`; no DNS key.
Do not enable timers yet. The files above do not themselves change Nginx routing.
Validate the installed units with `sudo systemd-analyze verify` and their four
absolute filenames, then `sudo systemctl daemon-reload`.

After separately approving deployment, build/start the private application stack:

```sh
docker compose --env-file .env.pilot -p pagewright-pilot -f docker-compose.yaml -f docker-compose.pilot.yaml up -d --build
sudo python3 /usr/local/libexec/pagewright-pilot-tls.py
```

The second command is a read-only inventory check. Verify every printed hostname
belongs to the pilot. Before starting generated builds, also build the production
worker image required by the deployment; see the worker sandbox runbook.

**Staging first.** With your actual operator email substituted, this command
installs dedicated challenge/pending routes and requests untrusted staging
certificates in a separate directory. It never installs staging certificates into
public HTTPS configuration. This is an external action requiring approval:

```sh
sudo python3 /usr/local/libexec/pagewright-pilot-tls.py --apply --email your-operator-email
```

Check private `/var/lib/pagewright-tls/staging/logs` and certificate results; the
controller can finish successfully with pending failures, so exit status alone
is not issuance acceptance. After staging passes, approve production issuance:

```sh
sudo systemctl start pagewright-tls.service
sudo journalctl -u pagewright-tls.service --no-pager -n 30
```

That unit explicitly uses `--production`. Verify app/api HTTPS, then enable the
reconciliation timer. Provision a closed-pilot test account, create a new site,
and check the subsequent timer run provisions both exact site names:

```sh
sudo systemctl enable --now pagewright-tls.timer
```

For renewal, test only the dedicated production directory (not the existing cron
or `/etc/letsencrypt` lineage). Run after the controller is idle; Certbot locking
and the shared host lock prevent overlap with the installed renewal unit:

```sh
sudo flock -w 300 /var/lib/pagewright-tls/controller.lock certbot renew --dry-run --config-dir /var/lib/pagewright-tls/production/config --work-dir /var/lib/pagewright-tls/production/work --logs-dir /var/lib/pagewright-tls/production/logs
sudo nginx -t
sudo systemctl reload nginx
sudo systemctl enable --now pagewright-tls-renew.timer
```

The dry run tests renewal authorization; the two following commands separately
exercise the validation/reload hook. The installed renewal service reloads Nginx
only after a successful renewal. Confirm both timers are scheduled. Monitor
expiry/failures; do not assume a successful initial issuance proves renewal.
See [Certbot webroot and renewal](https://eff-certbot.readthedocs.io/en/stable/using.html).

## Public acceptance and rollback

Verify real certificate chains/SANs for app, API, live and preview; redirects;
new-site provisioning/failed issuance recovery; preview before publish, independent
live/preview assets and rollback; application/generated-content origin boundaries;
unsupported aliases; expired/missing readiness gating; private upstream ports;
renewal; and unchanged apex/blog behavior. Keep signup closed. The local runtime
suite models TLS using self-signed test certificates and mock upstreams; it does
not prove public ACME reachability or the actual shared-server configuration.

If installation fails, stop/disable both dedicated timers and stop their services.
Wait for their processes to exit. Preserve logs and `/var/lib/pagewright-tls` for
diagnosis (never delete certificates/volumes to retry). Move only the dedicated
`/etc/nginx/conf.d/pagewright-pilot.conf` out of the include directory to a reviewed
backup path, run `nginx -t`, and reload only if validation succeeds. Do not restore
the entire shared Nginx tree over unrelated edits. Stop the pilot Compose project
without `--volumes` if required. Existing blog/apex config and certificates were
never modified. A retained `previous-config.json` needs review before reinstalling;
do not discard an unresolved journal or let a restarted timer undo the rollback.

## Local development and automated tests

The base Compose stack keeps HTTP defaults; TLS readiness is opt-in through the
pilot overlay. Local equivalents remain `http://demo.pagewright.io:8084/` and
`http://demo.preview.pagewright.io:8084/` with local hosts/DNS entries. No public
DNS changes are made by tests.

```sh
python3 -B -m unittest discover -s scripts -p test_pilot_tls.py
python3 -B scripts/test_pilot_tls_runtime.py
python3 -B scripts/test_pilot_compose.py
make test-integration
```

The runtime test creates its own nginx container and test certificates, then
removes that container. No CA calls, paid provider, privileged containers or
production data are involved. Rebuild gateway/UI for readiness support; serving
must already include the v3 [preview namespace migration](PREVIEW_ACTIVATION.md).

Current verification limit (2026-09-09): the full pinned Firefox 146.0.1 browser
journey timed out repeatedly on hosted navigation. A separate plain HTML server
reproduced a `200 text/html` response with the tab staying at `about:blank`.
Fresh contexts/processes did not resolve the acceptance failures; attempted
harness workarounds were removed. Security headers and the baseline harness are
unchanged. Full-browser acceptance is not claimed. The focused rendered
provisioning/session-recovery checks and real-nginx TLS tests passed, but public
browser acceptance still requires a working browser environment and a successful
full run before M4.9 can be completed.
