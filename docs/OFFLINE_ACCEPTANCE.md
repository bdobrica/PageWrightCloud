# Offline operator acceptance artifact

This is a narrowly scoped **operator import**, not an AI job or a second
end-to-end manager/worker test. It creates a distinct version for public
preview/live/asset/rollback acceptance without provider calls. The production
worker, manager, gateway, service authentication and spending policy are unchanged.
Neither utility is installed in production images or exposed through an API.

## Trust and scope

The operator must verify the exact site ID, committed base version, owner approval,
current live/preview pointers and idle build state before using it. Restrict use
to the dedicated acceptance site. Do not run concurrently with site deletion,
builds or deployment tests. Use a new `operator-m49-...` target, never `initial`
or an existing application build identity. Record all IDs and retain evidence.

`acceptance-compile` runs offline as a non-root user with a read-only container
root, no capabilities, no network, bounded CPU/memory/PIDs and only the exported
base archive plus a fresh output directory. It uses the exact trusted compiler
and theme from the reviewed worker image, not tools from downloaded site source.
It unpacks through the existing safe archive parser, snapshots source, performs
one fixed homepage edit and adds a version-specific text asset. The original
worker `build.Compile` performs allowed-change checks, fresh source compilation,
HTML/reference checks and archive validation. Any failure emits no manifest.
The output directory must be new; retain failed attempts rather than overwriting.

`acceptance-artifact` requires operator-level storage access. Its export mode
reads only a committed source artifact; mount storage read-only for export.
Import accepts only a trusted compiler bundle and explicit matching site/base/
target arguments. It verifies the existing committed base, both archive hashes,
operator provenance and the named static checks. The artifact write is also
digest-bound at the immutable-write boundary. It uses the storage backend's
existing immutable writes and manifest-last visibility. Identical re-imports
are idempotent; conflicting bytes are rejected. Partial artifacts stay hidden
without a valid commit; do not delete partial data to retry blindly.

The bundle is **not signed**. Its checks/provenance are a trusted operator's
attestation, not proof against a malicious operator. Never use this command as
an untrusted upload endpoint. The importer does not independently recompile.
Only its short-lived, network-disabled container gets writable storage; no
provider credentials, service secrets or Docker socket go into either container.
The importer runs as the storage file owner to write the new immutable objects;
it is not a privileged container. Default Docker seccomp/AppArmor and
`no-new-privileges` remain enabled. The production worker sandbox is unchanged.

No job, lease, fence, callback or gateway build history is fabricated. The
manifest and private log explicitly record `operator-acceptance-v1`, zero provider
calls and no browser checks. The UI discovers the committed artifact through its
normal storage-backed Versions list. Import never changes a deployment pointer,
serving file, certificate, account, signup setting, AI allowance or reservation.
Preview/publish/rollback still go through the ordinary authenticated UI and
serving validation. The imported version can also become the next build's base;
that is why this tool is limited to the designated test site.

## Build and invocation contract

From reviewed sources on the Docker host with the reviewed `pagewright-worker:m4.5`
image already present:

```sh
docker build -f deploy/Dockerfile.acceptance-artifact -t pagewright-offline-acceptance:reviewed .
```

Inspect and use the resulting image ID, not a mutable tag, for execution. The
tools take these arguments **inside isolated containers** (not direct host
commands with arbitrary production mounts):

```text
acceptance-artifact --root /nfs --site SITE_ID --base BASE_VERSION --export
acceptance-compile --input /input.tar.gz --out /work/bundle --site SITE_ID --base BASE_VERSION --target operator-m49-UNIQUE
acceptance-artifact --root /nfs --bundle /bundle --site SITE_ID --base BASE_VERSION --target operator-m49-UNIQUE
```

Export goes to stdout; keep binary archive output out of operational logs.
Use read-only mounts for the base archive and compiler bundle. Allow only the
fresh work directory to be writable during compilation; never mount live storage
or the hosting volume into that container. Before importing, inspect the generated
manifest, inspect archive layout and test the bundle on a disposable storage copy.
Mount only the explicitly resolved pilot storage volume writable for import.
An import is an authorized persistent write to that site's artifact history.

After import, verify live and preview are unchanged, the existing base bytes
match, and provider reservations have not increased. Do not delete a committed
version as cleanup: it may be selected by preview/live or as a later build base.
Disposable containers can be removed by exact returned IDs; retain the bundle
and audit evidence privately until acceptance is complete.

## Browser sequence

1. Refresh Versions and select the `operator-m49-...` entry.
2. Preview it: version 2 and its linked asset must load over trusted HTTPS;
   live must still show version 1 and return 404 for the new asset.
3. Promote version 2 to live: both hosts and the new asset must work.
4. Promote the original version 1 to live: live rolls back, its new-asset URL
   returns 404, while preview retains version 2 and its asset.
5. Reload/sign in again and verify pointers and pages persist.

This proves deployment behavior, not AI editing or a second manager job.
Public security/failure-recovery checks and full browser acceptance remain
separate M4.9 gates. No claim of completion follows merely from import success.

## Local checks

```sh
cd pagewright/worker
go test -race ./cmd/acceptance-compile ./internal/build ./internal/artifact
go vet ./cmd/acceptance-compile
cd ../storage
go test -race ./cmd/acceptance-artifact ./internal/storage/nfs
go vet ./cmd/acceptance-artifact
```

Importer unit fixtures test trust-boundary checks and persistence, not real
compilation. The actual offline container run must additionally verify the real
compiler output before production import.
