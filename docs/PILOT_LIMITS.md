# Pilot access and usage limits (M4.1)

The supported root Compose deployment now defaults to operator-provisioned
accounts and **no paid AI calls**. Existing accounts and data remain intact.
This does not make the stack safe for remote testers: M4.2–M4.14 remain release
gates, particularly internal-service authentication, private ports, origins and TLS.

## Account provisioning

First configure the [required PostgreSQL/JWT secrets](CONFIGURATION_SECURITY.md).
Existing database volumes require coordinated password rotation, not deletion.

Start the gateway so its migrations have completed. Create an account through
the existing operator CLI; prefer stdin so the password is not in process
arguments or shell history:

```sh
read -r -s -p 'New account password: ' pilot_password
printf '%s\n' "$pilot_password" | docker compose exec -T gateway ./users create -email tester@example.com -password-stdin
unset pilot_password
```

Provisioned passwords must contain 8–72 bytes. Existing login/password hashes are
unchanged. Public `POST /auth/register` returns a no-store 403 before reading or
creating account data. The registration page checks public capabilities and
shows operator-contact guidance, never a fallback signup form on lookup failure.
OAuth remains unavailable. `PAGEWRIGHT_SIGNUP_MODE=development` is an explicit
exception for disposable local tests; **never use it for a pilot deployment**.

## Configuration and charging semantics

| Setting | Default | Meaning |
| --- | --- | --- |
| `PAGEWRIGHT_USER_DAILY_BUILDS` | 10 | New build attempts per authenticated owner per UTC day |
| `PAGEWRIGHT_SITE_DAILY_BUILDS` | 5 | New attempts per owned site per UTC day |
| `PAGEWRIGHT_ACTIVE_BUILDS` | 2 | Global admission/provider concurrency; also root manager dispatch concurrency |
| `PAGEWRIGHT_AI_ALLOWANCE_CENTS` | 0 | Lifetime conservative provider allowance; zero disables AI |
| `PAGEWRIGHT_PROVIDER_TOKEN` | empty | Separate random internal credential, at least 32 characters when enabling AI |
| `PAGEWRIGHT_LLM_KEY` | empty | Real provider credential, held only by the gateway |

Attempts are reserved before any clarification/instruction call. Failed and
unclear attempts count; a clarification answer with a new idempotency key is a
new attempt. Exact retries do not consume another daily slot. Conflicting input
under the same key is rejected. An existing durable submission is looked up
first, so recovering it does not need another allowance or regenerate a prompt.
One owner may have only one preparing/active build at a time. Global admission
counts both preparing attempts and pending/running durable submissions, including
uncertain dispatches. Brief double-counting during dispatch is conservative.

PostgreSQL transactions/advisory locks serialize admission across gateway
processes. Database time determines quota windows. Errors in usage checks return
503 without starting work; quota/concurrency failures return 429 with retry
guidance. Preparing attempts remain occupied after a crash, rather than assuming
the provider or worker stopped. Reservations and quota history survive restart.

Request throttling uses durable fixed-minute windows: 600 requests globally,
120 per authenticated owner, 120 per peer IP, and 10 authentication requests per
peer IP. Health and OPTIONS are exempt. Only the connection's peer address is
used, not spoofable forwarding headers. Users behind one proxy/NAT share its IP
limit until explicit trusted-proxy handling is added. A minute boundary can admit
two adjacent windows' bursts; this is not a sliding-window limiter. Expired IP
counter rows are pruned; attempts and spending history are retained.

## Provider boundary and budget

Gateway clarification/instruction calls and installed worker CLI requests use
the internal listener on gateway port 8087, not a public API route. Root Compose
does not publish this port. Workers/manager receive only the internal token;
legacy `PAGEWRIGHT_WORKER_LLM_KEY`, `_URL` and `_MODEL` no longer select direct
provider access in root Compose. The worker model is the previously accepted
`gpt-5.6-luna`; gateway text classification remains `gpt-3.5-turbo`.

Every admitted upstream attempt permanently reserves **100 cents before network
I/O**. The allowance is a deliberately conservative reservation total, **not a
bill or actual usage estimate**. For example, 1000 cents admits at most ten
provider attempts, not ten builds. A build usually needs several provider calls.
No reservation is refunded for a small response, provider rejection, cancellation,
missing usage, or a crash. Increasing the configured lifetime total grants only
the difference; restart and date changes do not replenish it. All gateway
instances must use the same settings; drain/restart all writers when changing
policy. Never delete reservation history to replenish credit.

The proxy supports only reviewed text Chat Completions and Responses requests:
one Chat completion with at most 500 output tokens, or Luna Responses with at
most 8192 output tokens and standard service tier. It rejects unknown top-level
fields, server-side history/background work, remote/media input and hosted tools.
Local function/custom/shell definitions, including validated `additional_tools`
items from the pinned CLI, are allowed; execution still occurs
inside the unchanged worker sandbox. Requests are limited to 1 MiB, responses to
4 MiB, and upstream calls to 120 seconds, with no automatic retries or redirects.
Successful verified terminal responses release concurrency, but never money.
Uncertain or failed provider outcomes retain an active reservation for operator
review. Database failures fail closed. Shared internal credentials are not yet
job-scoped; M4.3 still owns scoped service authorization.

OpenAI Docs informed this policy (reviewed 2026-09-07). At the reviewed
[standard pricing](https://developers.openai.com/api/docs/pricing), Luna's maximum
long-context cache-write input price is $0.50/M and output is $1.80/M. Reserving
the full [1.05M-token context](https://developers.openai.com/api/docs/models/gpt-5.6-luna)
plus 8192 output tokens bounds that request at $0.5397456, below the $1 reservation;
text gpt-3.5-turbo is also below this reservation. This bound assumes those model
limits/prices and the official standard endpoint remain valid. Review pricing
before enabling/replenishing allowance or upgrading the policy. It is not a cap
on unrelated API-key usage, taxes, or provider changes. Configure a separate
provider-project [hard spend limit](https://developers.openai.com/api/docs/guides/spend-limits)
as defense in depth; its enforcement can lag tracked spending.

Default mode accepts only `https://api.openai.com/v1`. The development exception
permits local fixtures/custom endpoints for tests and is not a paid pilot mode.
No production credentials, balances, provider settings or remote hosts are
changed by the automated acceptance tests.

## Fail-closed recovery and rollout

Back up PostgreSQL before upgrading. Migration 011 adds admission, rate and
provider-reservation tables without changing existing accounts, artifacts or
deployments. Upgrade gateway, manager configuration and UI together; stop old
workers/direct-provider configurations before enabling credit. Do not run older
gateways against the new admission policy.

For an exhausted allowance, inspect the reservation total and raise the lifetime
configuration only with explicit spending approval. For occupied attempts or
provider reservations after an error/crash, stop dispatch and gateway provider
traffic, stop affected workers, and verify outstanding provider work has ended
before clearing **only the identified busy/active flags** through an operator
database session. Never decrement `reserved_cents`, delete reservation rows, or
mark uncertain builds completed. Provider-side uncertainty may require waiting
or provider investigation. Keeping a slot blocked is intentional if safe release
cannot be established. Existing build reconciliation still owns job outcomes.

Read-only inspection queries (operator database access only):

```sql
SELECT coalesce(sum(reserved_cents),0) AS lifetime_reserved_cents,
       count(*) FILTER (WHERE active) AS occupied_provider_slots
FROM pilot_provider_reservations;
SELECT id, created_at FROM pilot_provider_reservations WHERE active;
SELECT owner_id, site_id, request_key, created_at FROM pilot_attempts WHERE busy;
```

## Verification

Use `make test-integration` for PostgreSQL admission/spending/rate races and
restart-persistence checks; gateway package/race tests cover registration,
configuration, forwarding-header spoofing and the provider boundary. Run the
[browser acceptance suite](BROWSER_ACCEPTANCE.md) for the full real-service edit/
publish journey through the proxy with a local dummy provider, followed by a
gateway restart into closed mode, operator-provisioned UI login and disabled-AI
429 guidance. The fixture
allowance represents synthetic reservations, not paid API calls.
