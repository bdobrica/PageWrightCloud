# Deterministic compiled-artifact acceptance

Run `make test-integration` from the repository root. Docker is required; no AI
key, external provider, host ports or application `.env` are used. The existing
integration command (also used by CI) builds the real compiler and trusted starter
theme into the test image, and runs the race-enabled service suites.

`TestCompiledArtifactRoundTrip` performs this sequence:

1. Register through the real gateway auth handler, then create a fresh starter
   site through its authenticated site route. PostgreSQL and storage are real;
   there are no direct database inserts/updates or manually seeded public files
   in this scenario (the suite only creates/migrates its isolated schema).
2. Submit an authenticated build with an idempotency key. A local deterministic
   instruction-provider response replaces the paid provider. The actual manager
   API reserves the job and Redis lock and returns the canonical launch snapshot.
3. Launch the integration-only worker test entrypoint with that snapshot. It
   calls the production `runJob`: fetch/unpack bootstrap, patch instructions,
   execute, pack/validate, upload archive/log/manifest and report completion.
   The fixture executor recognizes one exact prompt, edits Markdown, and invokes
   the actual `pagewrightc` CLI. It does not write HTML itself.
4. Read completion through the manager API and list/download through gateway
   routes. Check compiled layout, preserved source, changed HTML and immutable
   source/output bytes after a conflicting PUT.
5. Deploy through the gateway version route, which calls real serving deployment
   and activation routes. Fetch nginx HTML using the site's Host header and
   compare it and every public asset byte-for-byte with the archive. Private
   source, instructions, metadata and runtime paths must return 404. Rejected
   source-only deployment must leave the compiled live HTML unchanged.

The gateway uses production handlers and auth middleware on a test HTTP server;
manager, Redis, storage, PostgreSQL and serving/nginx run in isolated containers.
Serving and nginx are deliberately co-located in the test container so the real
serving reload command reaches nginx; its hostname hash size supports the long,
UUID-based fixture domains. The test creates no serving files directly.
The dedicated Compose project removes its containers and ephemeral data on exit;
it does not use application volumes. Built test images/caches may remain.

## What this does not prove

The test harness uses the manager's integration-only `test-manual` spawner.
M2.1's real Docker create/start is tested separately by `make test-docker-spawner`.
The executor is excluded by the `integration` build tag and only built by
`tests/Dockerfile`; production images do not contain it. Production AI execution,
trusted compiler integration, successive edits, resource isolation, callback
recovery and queue dispatch remain M2. The existing worker manifest truthfully
retains `checks_passed: false`; this fixture is not a production validation gate.

The test-only nginx topology does not repair root Compose's separate nginx reload
problem. Browser publishing/preview, production topology, atomic activation and
DB/serving reconciliation remain M3; internal authentication and remote-pilot
hardening remain M4. This is M1 contract acceptance, not a working AI MVP or a
paid-provider smoke test.
