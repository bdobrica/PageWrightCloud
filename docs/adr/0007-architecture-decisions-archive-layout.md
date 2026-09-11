# Version archive layout (M1.7)

ADR 0007 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


Storage retains immutable raw tar.gz bytes. Worker packing/unpacking and serving
deployment enforce the same versioned layout; storage itself remains opaque.

```text
manifest.json              safe layout metadata, never served
content/site.json          editable source configuration
content/home/index.md      nonempty home source (.mdx also accepted)
content/...                additional editable source/assets
public/index.html          nonempty generated home, compiled layout only
public/...                 generated pages and assets
```

The in-archive manifest has exactly these supported fields:

```json
{"schema_version":1,"kind":"compiled","theme_id":"starter"}
```

`kind: "source"` requires no `public/`; `kind: "compiled"` requires a nonempty
`public/index.html`. Both require valid JSON configuration with a nonblank
`site_name` and nonempty home source. “Compiled” describes the file layout,
not proof of compiler execution, freshness, HTML safety or passed checks.
Worker `checks_passed` remains false until trusted build checks land in M2.

This small layout manifest is distinct from the private storage
[build manifest and execution log](0004-architecture-decisions-version-metadata.md). Prompts, source-version
provenance, timestamps, compiler/check results and logs belong in those private
sidecars, not public output. Worker file counts/sizes describe regular archive
entries, including the layout manifest, rather than the execution workspace.
Theme/compiler versions and actual output checks remain M2.4.

## Packing and validation

Packing uses an allowlist: only `content/`, `public/`, and a freshly generated
layout manifest enter the archive. Root runtime files such as `.codex/`, `.env`,
execution logs and theme code are excluded. Trusted theme code remains outside
the editable archive. Unsafe files inside the selected trees fail packing rather
than silently disappearing. Failed packing leaves an existing output archive intact.

Both consumers reject unknown roots, noncanonical/traversing/absolute paths,
backslashes, colons/control characters, duplicate entries, file/directory
conflicts, symbolic/hard links, special files and extended tar metadata.
Only regular files and directories are supported; permissions are normalized to
0644/0755. Dot-prefixed path components (including `.env`, `.git`, `.codex` and
`.well-known`), instruction/prompt files, log/secret/credential directories,
common private-key/credential filenames and log/key extensions are disallowed.
Public output additionally rejects source directories (`content`, `source`,
`src`), `site.json`, Markdown/MDX, source maps and nested manifests.
See the identical `allowedPath` implementations in worker/serving for the exact
conservative MVP filename policy.

Limits: 64 MiB compressed; 256 MiB expanded including tar headers/padding;
32 MiB per regular file; 64 KiB for configuration/layout JSON; 10,000 entries;
1,024 bytes per path. Extraction reads through the gzip checksum and rejects
truncation and nonzero data after the tar end marker.

This is a structural boundary, **not a general secret scanner or HTML sanitizer**.
A token copied into an otherwise allowed HTML/image/JSON file cannot be identified
by its filename. Credential isolation, trusted compilation/output checks, redaction,
origin separation and internal-service authentication remain M2/M4.

## Extraction, retry and compatibility

Workers validate in a fresh staging directory and publish only into an absent or
empty workspace. They retain editable source for subsequent edits; runtime
instructions are added afterward and are excluded from the next packed version.
Failed validation removes staging files without modifying the destination.

Serving validates the entire archive but extracts **only public files** into a
staging directory. It never writes source or either manifest into the web cache.
Successful deployment renames the stage into the version cache, with a private
`.archive-sha256` marker outside `public/`. Same-byte deployment retries succeed;
different bytes or an old, unverified cache entry fail without replacement.
Rejected new deployments leave no version directory and do not change live/preview
links. Atomic link activation and nginx lifecycle remain M3.

New starter bootstraps use revision 2 and include a source-layout manifest.
Persisted M1.6 revision-1 source-only archives remain acceptable to workers even
without that manifest; their next pack generates it. Stored reservation bytes
are never regenerated on retry. Serving rejects all source-only archives.
Other historical nonconforming archives/caches are rejected, not automatically
rewritten or deleted. Already-active legacy caches are not retroactively audited.

## Verification

Worker/serving unit tests exercise layout rejection, private-file exclusion,
source preservation across a second edit, normalized modes, checksum damage,
failed-pack/extraction preservation, deploy retry/conflict and public-root HTTP
404s for private paths. The isolated integration suite checks policy-file parity
and round-trips one text/binary archive through worker, actual storage, gateway
and serving: full source is retained for editing while only public files deploy.
These are archive/HTTP-root checks, not a real AI build or nginx/browser journey.
