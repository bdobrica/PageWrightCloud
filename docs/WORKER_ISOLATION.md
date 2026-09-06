# Worker isolation (M2.6)

The local Docker daemon has no AppArmor; the supplied remote host passes the
full installed CLI/compiler suite with enforcing AppArmor and the narrowly scoped
proc-compatible launch policy. See [the host acceptance record](M2_6_HOST_ACCEPTANCE.md). No
privileged/unconfined fallback or host-wide sysctl change is introduced.

## Resource policy

Manager launch and the root Compose worker declare the same fixed ceilings:

| Resource | Ceiling |
| --- | --- |
| CPU | 1 CPU quota |
| RAM / swap | 1 GiB / none |
| Processes | 128 |
| Workspace | 256 MiB tmpfs |
| Container and CLI scratch | 64 MiB each |
| Shared memory | 16 MiB |
| File descriptors / core dumps | 1024 / disabled |
| CLI time / compilation | 10 minutes / 2 minutes |
| Job context / whole-runner watchdog | 15 minutes / 16 minutes |
| Captured CLI output | 1 MiB; overflow cancels execution |
| Downloaded compressed archive | 64 MiB |
| Container log retention | Two 10 MiB files |

Root filesystems are read-only; UID 1000, no-new-privileges and CapDrop ALL remain.
Docker init reaps children. Archive validation retains its 256 MiB expanded,
32 MiB per-file and 10,000-entry limits; multiple staged copies can exhaust the
workspace earlier and fail safely. RAM includes tmpfs use. CPU quota is a ceiling,
not reserved host capacity; size host capacity for the manager's concurrent-job
cap. Equal Docker memory/swap totals disable swap, per
[Docker's resource semantics](https://docs.docker.com/engine/containers/resource_constraints/).

## Execution and credential boundary

An outer bubblewrap namespace exposes only installed system/CLI files read-only,
private `/proc` and `/dev`, bounded scratch, the job site and fresh CLI state.
It does not mount runner files, sibling workspaces, the theme directory, Docker
socket or host user directories. The compiler binary is read-only; runner alone
compiles frozen source using the trusted theme outside the CLI namespace.

The provider key is intentionally available to the trusted CLI, not to generated
shell commands. The CLI has network access to contact its configured provider;
its inner workspace sandbox denies tool network access. The environment allowlist
excludes management job/lease credentials. Installed fake-API tests verify actual
shell denial of sibling management files, Docker socket, theme and provider-key
reads through `/proc`, not merely absence of an environment variable.
OpenAI's [sandbox documentation](https://learn.chatgpt.com/docs/sandboxing)
distinguishes enforcement from approval policy; these checks retain both the
inner sandbox and `approval_policy=never` rather than treating prompting as a
security boundary. No paid API calls are required for acceptance.

Process-group termination handles ordinary cancellation, and the outer PID
namespace removes detached/setsid descendants on cancellation **and normal CLI
exit**. Kill requests and SIGTERM/SIGINT cancel the job, including storage I/O
and compiler context; cancellation is checked before upload. `/kill` acknowledges
the request, not synchronous cleanup. A racing manifest commit can still make an
artifact durable; callback/reconciliation semantics remain M2.8/M2.9. The hard
watchdog exits 124 if cooperative cancellation cannot finish; recovery after
OOM/SIGKILL/hard timeout remains those lifecycle milestones.

Exact-key redaction remains a defense in depth, not general secret detection.
Logs remain private and bounded; arbitrary prompt/source content and transformed
secrets are not comprehensively sanitized. Internal HTTP authentication and
callback credentials remain M4. Unit and service fixtures bypass the outer CLI
namespace only in test code/integration-tag builds; installed CLI acceptance uses
the production namespace.

## Kernel-surface assessment

The existing three-rule container seccomp delta is unchanged. Trusted CLI sandbox
setup can access additional namespace/mount kernel code even though outer
capabilities are empty. An immutable PATH-selected bubblewrap wrapper stacks a
second seccomp filter after setup: tools cannot create/enter namespaces or use
mount/root-changing APIs. Ordinary threads remain allowed; clone3 returns ENOSYS
for clone fallback. Alternate ABIs fail closed. PID/filesystem isolation limits visibility and
process lifetime; cgroups limit consumption. None protects against a kernel bug.
The AppArmor profile allows namespace/mount setup but retains proc/sys restrictions;
it is not a narrowly allowlisted application syscall policy. Preserve the pinned
default-deny seccomp baseline, block setns/BPF/keyrings as before, and do not share
this profile with unrelated services. Use patched, dedicated worker hosts for
any remote pilot; stronger VM isolation needs a separate deployment decision.
This is a design assessment, not a kernel penetration test or remote-release gate.
See [profile provenance](../pagewright/manager/internal/spawner/docker/PROFILE_PROVENANCE.md).

## Proc compatibility and host acceptance

Docker's proc masks/read-only submounts prevented mounting private proc on the
tested Debian host. Selecting only `pagewright-worker-proc` makes the manager send
explicit Engine API `MaskedPaths` retaining `/sys/firmware` and
`/sys/devices/virtual/powercap`, and an empty (not null) `ReadonlyPaths`. The sysfs
mount remains read-only. AppArmor replaces protected proc path access denials.
Tools cannot remount those paths under an alias because their additional seccomp
filter denies mount and namespace operations. Trusted runner/CLI/bubblewrap remain
part of the trusted computing base; this is not equivalent confinement of a
compromised trusted CLI. Image provenance remains the operator's responsibility.

The manager rejects older images without the `io.pagewright.sandbox-policy=proc-v1`
label and launches by resolved image ID, avoiding a tag-change race. The worker
preflight requires actual namespace EPERM from a tool before provider contact.
The new profile is opt-in; empty/legacy profile settings retain Docker's defaults.
There is no automatic relaxation when sandbox startup fails.

On an AppArmor-enabled cgroup-v2 Docker host, from this checkout:

```sh
docker info --format '{{json .SecurityOptions}}'
sudo apparmor_parser -r security/pagewright-worker-proc.apparmor
export PAGEWRIGHT_WORKER_APPARMOR_PROFILE=pagewright-worker-proc
make test-worker-cli
make test-worker-compiler
make test-docker-spawner
```

The CLI suite verifies `pagewright-worker-proc (enforce)` and an AppArmor-specific
read denial of `/tmp/pagewright-apparmor-probe`, plus successful nested CLI
execution, credential/network denials, namespace teardown and cgroup limits.
CI now requires AppArmor rather than silently skipping this gate on its Ubuntu
runner. Remote AppArmor enforcement, protected proc access denials and full executor
acceptance pass. No successful hosted CI run is claimed. Go 1.24.10 is required
for the Engine API acceptance launcher. Containers are deleted by their returned
IDs after logs/inspection; no unrelated containers are selected for cleanup.

Manager/Compose/example defaults select the distinct `pagewright-worker:m2.8`
image tag. Build with `docker compose --env-file /dev/null --profile worker build worker`.
On the AppArmor host, also set the manager's profile variable as above. The legacy
manual Compose worker overlay cannot express this scoped Engine API policy;
use manager-launched workers. No production deployment or paid-provider call is
part of this acceptance. Loading the staged profile is runtime-only; persistent
installation/reboot handling remains an explicit administrator deployment step.
