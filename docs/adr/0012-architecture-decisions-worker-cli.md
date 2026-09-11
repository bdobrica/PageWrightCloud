# Worker CLI and sandbox contract (M2.3–M2.7)

ADR 0012 · Status: accepted implementation record.

This record preserves the milestone's design, contract and tradeoffs; dated
verification and future-work statements below are historical, not current release
status. For current procedures use the [operations index](../README.md); for
verified scope use [release acceptance](../RELEASE_ACCEPTANCE.md).


`pagewright-worker:m2.12` packages Codex CLI **0.153.4**, replacing the production
mock. The npm lockfile pins both the wrapper and native platform packages with
integrity hashes; the image checks the installed version. The worker invokes the
native executable directly so cancellation does not merely kill an npm launcher.
Deterministic executors remain integration-build-only.

## Invocation

The runner replaces `.codex/instructions.md` with its image-owned template and
explicitly supplies its text as `developer_instructions`. It does not assume
that Codex automatically discovers that filename. The template permits source
edits in `content/` and `assets/`, not generated output or trusted theme/compiler
files. Instructions are not validation: compiler integration remains M2.4.

The executor uses stdin for the prompt, fresh per-job CLI state outside the
archive, `--ephemeral`, `--skip-git-repo-check`, ignored user config/rules,
API-only authentication, approval policy `never`, and `workspace-write` sandboxing.
Command networking and extra temporary-directory write access are disabled.
These controls follow [non-interactive usage](https://developers.openai.com/codex/noninteractive)
and the [configuration reference](https://developers.openai.com/codex/config-reference).

The worker-only provider key becomes `CODEX_API_KEY` in the CLI process, using an
explicit Responses provider configuration for the chosen endpoint. The installed
version ignored the former `OPENAI_BASE_URL` setup. No operator login, manager
endpoints, job JSON or unrelated keys enter the CLI environment. Shell snapshots
are disabled and secret-name exclusions are explicit: installed-binary testing
showed that inheritance configuration alone was insufficient. The shell fixture
checks that the provider key is absent while the API fixture confirms bearer auth.

The [M2.6 isolation policy](../WORKER_ISOLATION.md) cancels execution at
the 1 MiB capture limit and adds outer PID/filesystem isolation and resource caps.
Exact-key redaction is not comprehensive secret detection. Cleanup/recovery and
per-job authorization remain separate concerns. An optional
`PAGEWRIGHT_WORKER_LLM_MODEL` manager setting becomes `PAGEWRIGHT_LLM_MODEL` in
the worker and an explicit CLI `--model` argument; empty retains the pinned CLI
default. [M2.12 provider acceptance](../PROVIDER_SMOKE.md) explicitly selects a model
behind a smoke-only budget gateway. A model being listed by the API does not
prove generation access for the configured key.

## Narrow runtime prerequisite brought forward from M2.6

On the tested WSL2 Docker host, Docker's default seccomp policy denied bubblewrap
namespace creation. The deprecated Landlock path was also incompatible with the
CLI's current permission profile; it is not enabled as a fallback.

Manager-created workers now run as UID/GID **1000:1000**, with all capabilities
dropped, no-new-privileges and a private UID-owned `/work` tmpfs. A pinned,
default-deny seccomp allowlist adds nested user-namespace creation and the mount
operations needed to construct the inner sandbox. The manager embeds the profile
and sends its JSON to Docker; no daemon-side profile path or worker socket mount
is needed. The kernel denies mounts by the outer capability-free user. See the
[audited delta and provenance](../../pagewright/manager/internal/spawner/docker/PROFILE_PROVENANCE.md).

This adds kernel attack surface for user namespaces; it is not protection against
kernel vulnerabilities. M2.6 adds a tool-only seccomp filter after sandbox setup,
blocking further namespace/mount operations. No privileged container, added
host capability, unconfined profile, global sysctl change or sandbox bypass is
used. A bounded local sandbox preflight fails **before any provider request** if
the host cannot enforce the required policy.

### AppArmor hosts

Docker's default AppArmor profile also denies mounts. An explicit worker-only
profile is supplied for AppArmor 4.x hosts, preserving the upstream proc/sys,
network-family and peer restrictions while permitting nested sandbox setup.
It is only safe with the non-root, zero-capability settings above. Review it and
load it on the Docker daemon host, then select the fixed profile name:

```sh
sudo apparmor_parser -r security/pagewright-worker-proc.apparmor
export PAGEWRIGHT_WORKER_APPARMOR_PROFILE=pagewright-worker-proc
make test-worker-cli
make test-worker-compiler
```

Set that variable in the manager's deployment environment as well. Unreviewed
profile names, including `unconfined`, are rejected. Missing profiles fail Docker
creation rather than falling back. The separate proc profile replaces Docker's
proc overmounts with explicit AppArmor denials; non-proc masks remain. The manager
requires the image's `proc-v1` compatibility label and resolves it to an immutable
image ID before creating the container. The runner verifies tool namespace denial
before contacting the provider. See the isolation document for the trust model.
The legacy `docker-compose.worker-apparmor.yaml` overlay does not implement this
Engine API proc policy. Build the optional worker image and let the manager launch
jobs; do not use a privileged/unconfined Compose workaround.

The WSL daemon does not enable AppArmor. The supplied Debian host passes the
complete installed CLI/compiler suite with the new enforcing profile and shared
manager launch policy; see [host evidence](../M2_6_HOST_ACCEPTANCE.md). CI requires
AppArmor and runs the same negative acceptance tests. Hosted CI has not been run
here. Other LSMs/kernel
restrictions require operator review and must fail closed, not be disabled.
Official guidance explains [Codex's Linux prerequisites](https://learn.chatgpt.com/docs/sandboxing).

## Verification

`make test-worker-cli` builds an acceptance target from the production runtime,
uses no external network or real credentials, and checks:

- Exact installed version, actual Responses request auth, instructions and prompt.
- A model-requested shell command really runs and writes inside the workspace.
- The shell cannot inherit the provider key.
- Outside-workspace writes and connections to a reachable parent loopback server
  are denied by the sandbox, not merely by the container's lack of internet.
- The outer process is UID 1000, has zero effective capabilities, retains seccomp
  and no-new-privileges, and cannot mount filesystems.
- Per-invocation runtime state is removed.

`make test-docker-spawner` additionally verifies actual manager-to-daemon launch,
container configuration, endpoint reachability and writable workspace. Package,
race/vet and full service integration cover preflight failure, argv/environment,
redaction/capture limits and existing deterministic build contracts. Two old
executor/parsing skips and the kill test were restored; two serving skips remain.
No paid API call or real AI edit was performed. Test containers/data are disposable;
test images/build caches may remain. M2.7 adds [fenced commits](0015-architecture-decisions-fenced-commits.md)
without changing the pinned CLI or sandbox policy. M2.8 is the next milestone.
