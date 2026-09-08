# Cross-user access acceptance (M4.7)

The public gateway authenticates bearer tokens before protected handlers. Active
site handlers resolve the requested site and compare its owner to the authenticated
user before reading private state, invoking internal services or reserving work.
Request-supplied owner IDs are not trusted. Origin validation complements this
boundary but is not authentication.

## Endpoint contract

| Surface | Anonymous | Other account | Owner |
| --- | --- | --- | --- |
| Site detail, enable/disable, build, jobs, versions, download, deployment status, live/preview activation | 401 | 403 for a valid request to an existing foreign site | Normal operation and validation |
| Site list | 401 | Only the caller's own rows and totals | Only the caller's own rows and totals |
| Site/version deletion and aliases | 401 | 501, no lookup or mutation | 501, no lookup or mutation |
| Retired `/ws` with no/allowed Origin | 501 | 501 | 501; never upgrades |

Invalid input may be rejected before site lookup. Missing sites return 404; the
existing 403/404 distinction can reveal that a hostname exists, but does not expose
private IDs, history, artifact bytes or deployment state. Deletion remains disabled;
this milestone does not enable destructive workflows to test them. Disallowed
browser origins are rejected by the outer M4.6 policy before these contracts.

Job IDs are scoped to both owner and site in the database query before producing
public polling responses. A foreign job ID under the caller's own site returns 404.
Public job snapshots omit private prompts, raw failures and credentials. There is
no browser event subscription/broadcast transport: WebSockets were retired in M3.3.
Any future transport must apply the same owner/site filtering before delivery;
client-side filtering is not an authorization boundary. Internal provider SSE is
not a user job-event subscription and retains the separate M4.3 credential boundary.

Version IDs are local to a checked site. Supplying another site's version ID under
an owned site never selects the foreign artifact. Currently a missing artifact
download or failed deployment returns 500; deployment failure may persist a failed
intent for the caller's own site, but cannot move live/preview pointers to the foreign
version. A future error-contract improvement should preserve that isolation.

Generated live **and preview HTML are public** under the current static MVP. Private
management/download endpoints do not make preview URLs confidential. Do not publish
secrets into generated content. Public-preview authentication would be a separate
product/security change, not an ownership regression fix.

## Regression coverage

`make test-integration` runs `TestCrossUserEndpointMatrix` against private migrated
PostgreSQL schemas with two real accounts and authenticated middleware. It checks
both directions of the endpoint matrix, secret-free errors, scoped list counts and
job reads, unchanged site rows/history, no persisted foreign deployment intent and
zero storage/serving/manager calls on denial. Existing job-history tests additionally
exercise pending/running/failed/completed snapshots and same-owner wrong-site IDs.

The disposable [browser acceptance runner](BROWSER_ACCEPTANCE.md) additionally uses
the real production router and services after its successful build/rollback journey.
It logs in as the journey owner and registers a second synthetic account through
the public API, creates that account's starter site and checks foreign access,
cross-site job/version substitution, owner download success, disabled deletion,
retired sockets and unchanged owner site/history/version/deployment snapshots.
It uses genuine issued tokens, no injected sessions or direct SQL writes. The
deliberate cross-site build request must fail before provider work; no additional
AI request is needed. All test accounts/sites live only in the disposable stack.

No production authorization behavior, schema, worker image, DNS or private `.env`
changes are required for this milestone. Re-run these regressions whenever routes,
ownership rules, downloads, polling or event transports change. This is coverage of
the current supported routes, not a general penetration test or proof against
stolen credentials, trusted-operator compromise or future ownership-transfer races.
Remaining M4 gates still block remote testers.
