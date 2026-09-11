# Compiler fixtures and boundaries (M1.8)

ADR 0008 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


Executable coverage lives in `pagewright/compiler/internal/compile/` and CLI
process tests in `cmd/pagewrightc/`. The committed
[starter fixture](../../pagewright/compiler/internal/compile/testdata/site/content)
uses the actual bundled theme, not a substitute renderer.

## Supported inputs and output

- Exactly one home page: root `index.md[x]` or `home/index.md[x]`. The worker
  archive contract specifically requires the latter. Both extensions in one
  directory, duplicate home pages and colliding normalized routes fail.
- Page directory segments match `[A-Za-z0-9][A-Za-z0-9_-]*`. Only an actual
  `home/` prefix is removed; `homestead/` remains `/homestead`.
- Grouping directories may omit an index. Pages attach to the nearest ancestor
  page, falling back to home. Navigation is sorted and uses all resolved titles
  before rendering. Asset directories are not discovered as pages.
- `site.json` requires a nonblank `site_name`, rejects unknown fields/type errors
  and trailing JSON, and defaults language to `en`. Theme configuration requires
  a name and tokens. An optional base URL is an HTTP(S) origin, not a path prefix.
- Theme tokens merge with site overrides. CSS token keys/values use a conservative
  string-only grammar; declaration delimiters, escapes, markup and URL functions
  are rejected. This supports starter colors, dimensions, fonts and shadows, not
  arbitrary user CSS. Token output is sorted for repeatable bytes.
- Theme assets go under `assets/`; page assets under `assets/pages/{page-id}/`.
  Tests compare exact text/binary bytes and reject unexpected output files.

Output must be absent or empty, disjoint from source and theme. Rendering happens
in a fresh sibling staging directory; publication occurs only after all phases
succeed. Failed rendering removes staging data and leaves existing output untouched.
Rebuild into a new directory rather than mixing fresh pages into old output.
This is not live/preview activation, durable storage commit or a worker build.

## Components and escaping

Both extensions support top-level `:::component Name` blocks terminated by
`:::`. This is constrained block syntax, not JSX or JavaScript execution.
Fenced code examples remain literal. Props accept JSON strings, arrays and objects
with string leaves; unquoted plain strings remain compatible. Malformed JSON-looking
values, numeric/boolean/null leaves, duplicate/empty prop names, missing names and
unclosed blocks fail. Scanner lines are limited to approximately 64 KiB.

Unknown components and template parse/execution failures produce build errors.
Diagnostics retain source context and numeric line/column formatting.
`YouTubeVideo` matches the documented name; `YoutubeVideo` remains an alias.

Fixtures check escaped site names/component text, filtered unsafe link schemes
and omitted raw Markdown scripts. Navigation, breadcrumbs and TOC attributes are
escaped. Formatted titles retain nested inline text; TOC anchors use Goldmark IDs,
including repeated headings separated by components. Themes/components remain
trusted templates, not user-provided code.

## Filesystem boundary

The CLI validates source/theme trees and rejects symlinks and special files.
Existing output path components cannot be symlinks. Page asset IDs cannot traverse
outside their output root. Writes use unique temporary files, checked closes and
rename, not predictable `.tmp` names. The inherited working-directory alias is
normalized; explicitly supplied symlink paths are rejected. Use physical absolute
paths or paths relative to the working directory.

Checks assume local Linux filesystems and inputs that are not modified concurrently.
They are not a sandbox against a process swapping links/files during compilation.
Worker credential isolation, resource limits, trusted-theme ownership, HTML/origin
policy, general secret detection and hosting activation remain M2–M4. These tests
do not replace the separate [archive/public-output policy](0007-architecture-decisions-archive-layout.md).

## Verification

```sh
make test-compiler
make test-compiler-smoke
cd pagewright/compiler
go test -race -count=1 -coverpkg=./internal/... ./...
```

Suites cover starter rendering, home/routes, nested navigation, component errors
and line numbers, escaping, deterministic tokens, exact assets, malformed config,
traversal, symlinks, existing-output preservation, late failure cleanup and CLI exit
codes. Root `make test-all` and existing CI run the suite. The smoke target also
builds the original three-page fixture and gateway bootstrap through the CLI.
No provider, Docker stack or browser is needed for these compiler suites.
M2.4 adds [production worker integration and static output checks](0013-architecture-decisions-worker-build.md).
Browser validation and broader production hardening remain future work.
