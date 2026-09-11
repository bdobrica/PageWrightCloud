# Request, archive and compiler limits (M4.5)

ADR 0025 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


Limits are fixed MVP policy, not knobs that disable validation. Rebuild services
and the worker (`docker compose build worker`), and replace old worker image pins
with `pagewright-worker:m4.5`. Existing data is neither rewritten nor deleted;
oversized legacy inputs require operator review, not automatic truncation.

| Representation | Limit |
| --- | --- |
| General JSON requests (auth/build, manager callbacks, legacy serving/log operations) | 1 MiB |
| Site creation and deployment intent requests | Existing 4 KiB limit retained |
| Private execution logs and commit metadata uploads | Existing 4 MiB limit retained |
| Compressed archive uploads, downloads and worker packing | 64 MiB |
| Archive decompression including headers, padding and trailer drain | 256 MiB |
| Archive entries including directories | 10,000 |
| Individual archive file | 32 MiB |
| Archive manifest/site configuration; compiler site/theme JSON | 64 KiB |
| Each compiler content/theme tree and final output tree | 10,000 entries, 256 MiB file bytes, 32 MiB per file |
| Individual template/Markdown/component rendering and assembled page | 32 MiB |
| Gateway storage version-list response | 4 MiB |

MiB/KiB are binary units. Count limits include the compiler tree root. Input tree
limits apply independently to content and theme. Final output is validated before
publication; page generation checks the growing stage after each page. Asset copies
are bounded by validated source trees and bounded individual writes. Temporary disk
use can exceed the final-output allowance while a last file or asset copy is staged.
Worker container memory/CPU/disk/process limits and timeouts remain required; these
representation limits are not a proof of constant-time parsing or a host-wide quota.

## Request handling and publication

The general JSON decoder reads the entire bounded body before decoding, rejects
unknown fields and additional JSON values, and counts trailing whitespace against
the budget. It does not rely on Content-Length or accept a valid prefix of an
oversized body. Existing smaller endpoint limits still apply. Invalid general JSON
returns 400; oversized archive/metadata uploads return 413. Empty/null requests
remain subject to each endpoint's required-field validation.

Storage applies the archive cap inside its upload handler, including initial-source
bootstrap and direct handler use. Overflow prevents immutable publication and cleans
the temporary upload. Storage retains opaque gzip bytes: it does not unpack uploads.
Worker and serving validate their full layout before publishing extracted data.
Gateway streaming reads are capped too, so an oversized upstream stream fails rather
than appearing to complete successfully. Failed compiler writes preserve an existing
destination; compilation uses an unpublished stage removed on error.

## Archive and compiler security

Worker/serving share an identical archive policy, checked by integration: canonical
relative paths under allowed roots, no traversal, duplicate/conflicting paths,
symlinks, hardlinks, devices, FIFOs, unsupported tar metadata or hidden trailing
payload. Only regular files/directories are materialized with controlled modes.
Archive inputs themselves must be regular files, not symlinks. Private/source files
are not copied to the hosting root. Existing source-only bootstrap compatibility
remains, but source-only archives cannot be deployed. Gzip packing now stops at the
compressed-output limit before consuming unlimited staging space.

Compiler input paths/trees reject symlinks, special files, output/input overlap and
asset traversal before compiling. Metadata is bounded before allocation and parsing;
existing schema/token checks reject malformed configuration and CSS token injection.
Template/Markdown/MDX buffers and atomic file writes enforce output limits. Rendered
HTML/JS are still active generated content, not sanitized trusted application code:
origin separation/security headers remain M4.6 and later release work.

Private quiescent service-owned volumes and the existing worker sandbox remain
assumptions. This does not grant the worker privileged access, introduce a validation
bypass, change DNS, or replace storage retention/host-level capacity monitoring.

## Verification

`make test-all` exercises archive expansion bombs, compressed size/count limits,
malformed manifests, traversal, links/devices/FIFOs, compiler/asset escape attempts,
JSON suffix/size limits and unchanged destinations on failure. New compiler tree
tests use sparse files rather than allocating hundreds of MiB of fixture contents.
`make test-integration`, race/vet, startup/recovery smoke and the disposable
[browser journey](../BROWSER_ACCEPTANCE.md) check that accepted workloads still build,
preview, publish and roll back through the authenticated stack with a fake provider.
