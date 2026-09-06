# M2.6 host acceptance — 2026-09-06

Status: **host acceptance passed** with the separate `pagewright-worker-proc`
profile and guarded proc-compatible policy. The original failure and investigation
below are retained as history. Docker proc overmounts are replaced by explicit
AppArmor denials plus a tool-only namespace/mount seccomp filter; other container
protections remain. No privileged/unconfined mode or host-wide sysctl was used.

## Environment and scope

- Host: `ublo.ro`, Linux `6.12.86+deb13-amd64`, Docker `29.5.0`, cgroup v2.
- Docker reports AppArmor, seccomp and cgroup namespaces.
- The user loaded the worker profile as administrator. The SSH account has no
  sudo; no privileged container was used to obtain host access.
- Profile SHA-256: `1f45f906c8db66cafa3458f7fe89cdb629a5c433e6c2454b1c37dd8eb4fdb89f`.
- Staging: `/tmp/pagewright-m26-acceptance.FFCg96BO`. Only selected test sources
  were transferred, not `.env` files, credentials or Git history.
- Test containers use UID 1000, no capabilities, no-new-privileges, enforcing
  `pagewright-worker`, reviewed seccomp, read-only root and resource ceilings.
  Test execution has no network; image builds download pinned dependencies.

## Original-profile observations (superseded by final acceptance)

| Check | Result |
| --- | --- |
| Profile reports `pagewright-worker (enforce)` | Pass |
| Profile-specific read denial probe | Pass |
| CPU/memory/swap/PID ceilings | Pass |
| Installed inner Codex sandbox write/network/outer-mount denial | Pass |
| Installed compiler suite under worker AppArmor and seccomp | Pass |
| Production executor preflight / outer namespace | Fail before provider request |
| Outer namespace detached-child lifecycle | Cannot pass: child never starts |

The minimal bubblewrap reproduction, with the same restrictions, fails at:

```text
bwrap: Can't mount proc on /newroot/proc: Operation not permitted
```

The diagnostic container retains Docker's read-only `/proc/bus`, `/proc/fs`,
`/proc/irq`, `/proc/sys`, `/proc/sysrq-trigger`, and masked proc entries such as
`kcore`, `keys`, `interrupts` and `timer_list`. This is consistent with the
[upstream nested-proc mount problem](https://github.com/containers/bubblewrap/issues/284),
but privileged kernel audit logs were not available to the SSH account, so this
does not independently prove which kernel/LSM check rejected the mount.

A stricter empty `/proc` diagnostic did not work: installed CLI startup requires
`/proc/self/exe`. Binding the runner's proc tree would undermine the intended
process/credential boundary and was not adopted. No unmasked/systempaths-unconfined,
AppArmor-unconfined, seccomp-unconfined or privileged workaround was tried.

The lifecycle test was tightened to require a `started` marker from the actual
detached child before evaluating cleanup. Previously its cancellation case could
appear to pass when startup itself failed. Diagnostic output is now included in
that test's failures.

## Compatibility investigation history

### Approved compatibility investigation (2026-09-06)

The user approved a narrowly scoped worker-only proc compatibility change,
preserving explicit AppArmor protections. A diagnostic with the pinned CLI
proved that a tool could run `unshare -U /bin/true`; path-based proc denials alone
are therefore not sufficient to replace Docker's proc overmounts.

The candidate image now installs an immutable PATH-selected bubblewrap wrapper.
It passes an additional seccomp filter to distribution bubblewrap, applied after
sandbox setup and stacked with the CLI's filter. It denies namespace creation,
namespace entry, mount/remount APIs and root changes; ordinary clone/thread
creation remains allowed and clone3 returns ENOSYS for fallback. Unsupported
ABIs fail closed. The runner still uses `/usr/bin/bwrap` directly for its outer
setup. Official [CLI sandbox documentation](https://learn.chatgpt.com/docs/sandboxing)
guided verification of the PATH-selected bubblewrap boundary; the installed
binary, not documentation alone, was tested.

Both ABI policy unit tests and the local installed-CLI suite pass, including
workspace writes, tool user-namespace denial, network denial and detached-child
cleanup. This is not yet combined host acceptance with relaxed proc overmounts.

A separate `security/pagewright-worker-proc.apparmor` profile adds explicit
masked-proc read/write denials and read-only-proc write denials. It is staged at
`/tmp/pagewright-m26-acceptance.FFCg96BO/pagewright-worker-proc.apparmor` and passes
the host parser's no-load check. SHA-256:
`64f19c7e209d90c4f5a12bc05cb9e2f3bcd55ec1b7f2012d498f5d8e134f93d1`.
Administrator loading is pending. Existing profile and launch defaults have not
changed. No proc-unmasked test has been run. Next: load the separate profile,
implement the guarded opt-in launch configuration retaining non-proc masks,
then test the complete boundary and adversarial denial cases on the host.

At that stage, the decision was to keep M2.6 unchecked, image promotion and acceptance commits pending. Further work
requires reviewing a worker-only proc mount strategy with explicit equivalent
protections, or changing the process-isolation design; simply removing Docker's
protected-path policy is not an accepted fix. Any host-level change remains an
administrator action. Remote staging/logs and test image caches remain for
diagnosis; all test containers were launched with `--rm`. Existing services and
their data were not modified. The user-loaded AppArmor profile remains loaded.

## Final host acceptance

The administrator loaded the separate `pagewright-worker-proc` profile with
SHA-256 `64f19c7e209d90c4f5a12bc05cb9e2f3bcd55ec1b7f2012d498f5d8e134f93d1`.
Actual runtime identity is `pagewright-worker-proc (enforce)`.

The final acceptance launcher uses the same `workerHostConfig` and image-label
compatibility guard as production. It resolves the image to its immutable ID,
sets network `none`, and verifies the daemon's resulting limits/security/path
configuration. Only proc overmounts are removed; `/sys/firmware` and
`/sys/devices/virtual/powercap` masks remain, with read-only sysfs/root, UID 1000,
CapDrop ALL, no-new-privileges, custom seccomp, enforcing AppArmor and fixed limits.

| Final check | Result |
| --- | --- |
| Full production executor with fake Responses API | Pass |
| Private proc/PID namespace creation | Pass |
| Tool workspace writes; outside writes and network denied | Pass |
| Tool user-namespace creation denied | Pass |
| Sibling management file, socket, theme and provider-key isolation | Pass |
| Detached-child teardown on exit and cancellation (child-start proof required) | Pass |
| AppArmor identity and profile-specific denial probe | Pass |
| Protected proc read and sysctl write-open denials | Pass |
| CPU, memory, swap, PID, root-filesystem and daemon policy checks | Pass |
| Installed compiler and immutable trusted inputs | Pass |

Final logs in the staging directory: `proc-cli-final.log` (12.34s),
`proc-compiler-final.log` (1.72s), plus their build logs. Images:
`pagewright-cli-test:m26-proc-2` and `pagewright-compiler-test:m26-proc-2`.
Their respective immutable IDs are
`sha256:833020f752267a942d30ebba47757b61959af8320bdade1795df32c48cec4526` and
`sha256:acb1fdf591db211cfb97c1db551b5ab4f503de73371ec38c4fa4d5ad417a986d`.
Test containers are removed by exact returned IDs after inspection/log collection.
Logs, sources and image caches remain for diagnosis; unrelated services/data were
not changed. Both administrator-loaded profiles remain loaded. Persistent profile
installation and reboot handling were not performed. No paid provider, browser,
hosted CI or production deployment is claimed.
