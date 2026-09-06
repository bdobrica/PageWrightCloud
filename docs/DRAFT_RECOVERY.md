# Draft recovery and publication feedback (M3.10)

## Draft lifecycle

Chat saves text synchronously on each edit in browser session storage, under a
versioned key containing the account ID and site FQDN. It also saves the current
clarification question, original request and conversation ID. A saved draft is
restored on reload or returning to the site in the same tab; another account/site
does not load it. This is local convenience storage, not encrypted backup or a
durable server conversation. Closing the tab normally ends its lifetime; browser
session restoration may retain it.

Before sending, Chat saves the exact payload fingerprint and idempotency key.
After an uncertain response, reload or session expiry, Retry same request reuses
that identity. The text is locked while the submission is uncertain, and no
request is sent automatically after login. A confirmed response clears the sent
text/identity or stores the next clarification. An explicit server rejection
clears the retry identity. Discard draft requires confirmation and warns that an
already accepted build can still run; it does not cancel a build.

Protected-request 401 responses remember an allowlisted local destination and
clear the expired login. Re-authenticating as the same account returns there.
A different account goes to the dashboard without loading the previous account's
draft. Bad login credentials stay on the login form instead of triggering an
expiry redirect. Late 401s from replaced tokens do not clear the new session.
Cross-tab auth changes remount protected content; submission also checks the
current account before dispatch. No owner-check marker is sent to the gateway.

Explicit Logout clears this tab's saved drafts and return destination. Automatic
expiry deliberately does not. Session storage is accessible to scripts on the app
origin and is not a boundary against XSS or someone controlling the browser.
Use Logout on shared devices. Tokens/passwords are not copied into draft records.

Storage failures are visible and block sending before dispatch; text remains in
memory so it can be copied. Invalid saved records are not silently overwritten.
Retry loading saved draft can recover after storage is repaired, or the user can
explicitly discard the record. Input is limited to 50,000 characters.

The gateway's clarification cache is still in memory. If it was lost, the saved
question/original/answer remain available. After a failed clarification, Restart
clarification as a new request explicitly combines the original and answer with a
new identity; the user must first check build history. Overlong combined text is
not silently truncated. This is not server-side conversation persistence.

## Feedback and refresh

- Chat displays submission progress, uncertainty, storage errors and retry actions.
  Existing bounded build-history polling remains authoritative for accepted jobs.
- Versions have timestamps, exact IDs and independent Live/Preview labels based
  on the last confirmed site response. Completed-version lists have refresh/retry
  controls and older/newer pagination.
- Publishing has visible pending/error states. Failed preview/publish attempts
  refresh site state too: activation may have succeeded before confirmation was
  lost. Retrying the same selected version uses the existing deployment protocol.
- Dashboard fetches server state on entry, focus/visibility return, manual refresh
  and after enable/disable attempts. It does not infer success by flipping local
  booleans. Duplicate toggles are blocked while pending; failed reads retain the
  previous data with a stale-state warning and disable toggle controls.
- Read requests have 10-second timeouts; build/deploy/toggle calls have 30-second
  timeouts. A client timeout does not cancel an accepted server operation.
  Cleanup aborts list reads and suppresses stale/unmounted updates.

No backend protocol, schema, provider setup or deployment-recovery policy changed.
M3.11 retains the full keyboard/focus/mobile audit; M3.12 retains the complete
real-service browser journey.

## Verification

Run from `pagewright/ui`:

```bash
npm run test:contracts
npm run lint -- --max-warnings=0
npm run build
node test/draftBrowser.mjs /absolute/path/to/playwright/index.mjs /absolute/path/to/firefox
```

The focused rendered test uses an installed Playwright/Firefox pair, a loopback
static server and intercepted gateway fixtures. It exercises draft reload,
expiry, bad-credential login, same-account return, exact-key uncertain retries,
account isolation, clarification restoration, failed/successful publishing,
dashboard action/focus refresh, version retry, storage failure/repair and logout.
It requires browser/local-listener permissions but makes no provider calls.
The harness does not install browser dependencies or modify a running gateway.

Pure tests additionally cover malformed storage, safe return destinations,
stale-token 401s, draft deletion boundaries and independent version labels.
The production Compose smoke verifies bundle startup and retained data across
container recreation; it is separate from the mocked browser regression.
