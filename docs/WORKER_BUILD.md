# Trusted worker builds (M2.4)

The selected `pagewright-worker:m2.12` image packages `pagewrightc` **0.1.0**
and starter theme **1.0.0** at `/usr/local/bin/pagewrightc` and
`/opt/pagewright/themes/starter-1.0.0`. Both are root-owned, read-only to the
UID 1000 worker. Build from the repository root:

```sh
docker build -f pagewright/worker/Dockerfile -t pagewright-worker:m2.12 .
make test-worker-compiler
make test-worker-cli
make test-integration
```

After unpacking and installing trusted instructions, runner snapshots the site.
After the executor returns, it rejects changes outside `content/`, hidden paths,
symlinks, special/executable files and archive-limit violations. Assets belong
under `content/<page>/assets/` and compile to `public/assets/pages/<page>/`.
Reported changed paths come from the filesystem diff, not the model's list.

Accepted source is copied with digest checks to a fresh sibling directory outside
the agent's writable root. The compiler receives only that source, the installed
theme, and a fresh output directory, with a two-minute deadline and no inherited
provider credentials. The archive contains the same source that was compiled.
Compilation, static HTML/reference checks and archive validation must all succeed
before replacing workspace `public/` or starting an upload. Compiler/validation
failures preserve previous output. Fresh output cannot retain obsolete pages.
This local replacement is not live hosting activation; that remains M3.

The private execution manifest records compiler/theme versions, actual changed
paths and `checks_passed=true` only after these named checks pass:

- `allowed_source_changes`
- `trusted_compiler`
- `html_and_local_references`
- `archive_layout`

HTML checks require nonempty home output, UTF-8, tokenizable documents with
explicit html/body tags, and existing local `href`, `src` and `poster` targets.
They reject base overrides and unsupported URL schemes. They are not a browser,
HTML sanitizer, CSS/srcset validator, fragment validator or external-link probe.
`browser_checks_performed=false` explicitly marks console errors unmeasured;
the legacy zero count and empty screenshots do not claim browser success.

`PAGEWRIGHT_COMPILER_BINARY` and `PAGEWRIGHT_THEME_PATH` are trusted operator/test
overrides, never workspace inputs. The production defaults are version checked.
Full process cancellation/resource limits remain M2.6; this change preserves the
existing non-root, capability-free sandbox and introduces no privileged mode.
No paid-provider acceptance is implied by deterministic compilation tests.
See [M2.12](PROVIDER_SMOKE.md) for the separate authorized provider run.

The installed compiler suite runs without network or capabilities and verifies
trusted-input permissions, real builds, asset bytes, deleted/stale output,
forbidden edits and failure preservation. The service round trip uses a
source-only deterministic executor; production runner performs the compilation
and its persisted manifest is checked before deployment through serving/nginx.
