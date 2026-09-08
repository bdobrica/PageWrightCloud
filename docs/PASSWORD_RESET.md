# Password reset and session recovery (M4.8)

## Operator setup

Rebuild gateway and UI. No worker image or schema migration is required. Configure
these values privately; `.env.example` contains empty, non-working placeholders:

```dotenv
PAGEWRIGHT_SMTP_ADDRESS=smtp.example.com:587
PAGEWRIGHT_SMTP_MODE=starttls
PAGEWRIGHT_SMTP_USERNAME=your-smtp-user
PAGEWRIGHT_SMTP_PASSWORD=your-smtp-password
PAGEWRIGHT_SMTP_FROM=reset@pagewright.io
PAGEWRIGHT_RESET_URL=https://app.pagewright.io/reset-password
```

Use the actual provider's hostname, port and verified sender. `tls` selects implicit
TLS (commonly port 465); `starttls` requires a successful STARTTLS upgrade before
credentials or message data are sent. Server certificates and names are verified;
there is no plaintext fallback or insecure certificate option. Username/password
must both be present or both absent (for a trusted TLS relay). SMTP delivery has an
eight-second total deadline and honors request cancellation. The runtime image
already includes system CA certificates. The implementation uses Go's
[SMTP client](https://pkg.go.dev/net/smtp) with mandatory TLS rather than its
opportunistic `SendMail` helper.

The reset URL must be the exact `/reset-password` path on an origin listed in
`PAGEWRIGHT_APP_ORIGINS`, without credentials, query or fragment. HTTPS is required
except for explicit localhost/127.0.0.1 development origins. Links never derive
from request Host or forwarding headers. M4.6 rejects generated-site origins from
the application allowlist. All-empty SMTP configuration disables reset email with
a uniform 503; partial/invalid configuration stops gateway startup before DB work.

Actual sender verification, provider credentials, SPF/DKIM/DMARC and inbox delivery
need operator/provider setup. No SMTP service, sender identity, DNS change or real
recipient has been assumed or contacted by this milestone. The user owns
pagewright.io, but ownership alone does not configure mail delivery. Before remote
testers, configure the real service, request a link for an operator-controlled
account, verify sender/link/inbox delivery, and exercise reset and login. Never paste
credentials or reset links into logs, issues or public traces. Production HTTPS/DNS
remain M4.9 gates.

## Reset contract

For configured delivery, eligible, missing and passwordless accounts get the same
200 JSON message. SMTP failures also return that generic message to avoid revealing
account existence; a fixed, secret-free operational log records failure. There is
no durable email outbox/retry queue: after a failure or lost email, retry after a
minute and investigate provider delivery. A successful SMTP DATA acknowledgement
means the relay accepted the message, not proof that a human received it. Timing
can still differ between existing and missing accounts; this is not a constant-time
account-enumeration guarantee. Existing email identity/case semantics are unchanged.

One request per account key per database minute bucket is allowed, including missing
accounts. Existing public auth throttles additionally allow ten auth requests per
source IP per minute, with global limits. Throttled responses use JSON 429 and
`Retry-After: 60`. Password reset attempts retain those public auth limits.

Reset secrets are 32 cryptographically random bytes, encoded as 64 hex characters.
Only SHA-256 digests are stored in PostgreSQL. Tokens expire after one hour; expiry
is checked using database wall-clock time when consuming the token. Under an account
row lock, token consumption, password change and invalidation of sibling links
commit atomically. Concurrent consumers cannot both succeed. Failed password updates
roll back token consumption. Ordinary authenticated password changes also invalidate
pending reset links. SMTP delivery failure invalidates its token; failures of that
invalidation are logged without secrets and the token still has its bounded expiry.
Expired records are pruned by the gateway's periodic cleanup.

Existing plaintext UUID links issued by the old incomplete implementation are no
longer accepted; request a fresh email after upgrading. Their old rows expire and
are removed by cleanup. No stored passwords/accounts are migrated or renamed.

New links carry the token in a URL fragment, not a query string, so the token is
not sent in the initial HTTP request or normal proxy access logs. The UI captures
it in memory and removes it from the current history entry. It also scrubs legacy
query links, but cannot undo an already logged legacy request. Reloading the reset
page loses the in-memory token; reopen the original email link. Tokens are sent
only in the reset POST body, never echoed in responses or routine logs.

## Passwords and sessions

Registration, operator provisioning, reset and authenticated password change share
the existing MVP minimum of eight Unicode characters and a maximum of 72 UTF-8
bytes, matching the storage algorithm's
[bcrypt byte limit](https://pkg.go.dev/golang.org/x/crypto/bcrypt#GenerateFromPassword).
The UI applies the same count/byte policy. Passwords are not trimmed or silently
truncated. This preserves the project's minimum rather than claiming a stronger
password-security standard. Existing passwords remain valid for login until changed.
Auth responses use JSON and no-store; used/expired/invalid reset tokens share the
same error. Reset success offers an explicit login link rather than auto-login.

Password reset does **not** immediately revoke already-issued JWTs. Existing sessions
remain usable until their configured expiry (default 15 minutes); do not increase
that lifetime casually or promise “sign out everywhere.” Server-side session
revocation would require additional session/version state. Reset links and JWTs are
separate credentials with separate lifetimes.

Actual expired JWTs are rejected before handlers. Existing UI recovery preserves
the same owner's unsent draft, retry identity and clarification across a 401 and
re-login, without resubmitting automatically. Server-side jobs continue and can be
polled again after login; changing account or explicit logout follows the existing
draft isolation/clearing rules. This milestone does not cancel active work.

## Verification

TLS SMTP tests exercise actual local SMTP dialogues for implicit TLS and STARTTLS,
message recipient/link content, untrusted-certificate rejection and refusal of
plaintext. PostgreSQL integration tests cover generic responses, token hashing,
account throttling, eight-way concurrent consumption (one winner), expiry/replay,
old/new password login, delivery failure, sibling invalidation and forced-update
rollback. SMTP integration uses a private test CA; no production trust override is
exposed. Handler integration captures messages at the sender interface, separately
from the SMTP wire tests; it is not a real-provider inbox test.

The rendered draft browser suite now includes reset throttling feedback, shared
password policy, fragment-token scrubbing/no URL leakage and the success login
link, alongside session-expiry/re-auth/draft recovery. The full disposable production
journey continues to cover real builds, previews, rollback and security boundaries.
Remaining M4 gates still block remote testers.
