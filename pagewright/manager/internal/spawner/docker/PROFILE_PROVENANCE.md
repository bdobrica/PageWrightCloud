# Worker syscall profile

`seccomp-upstream.json` is the unmodified Moby default from
[`moby/profiles` commit 61eaf32614c7c71b60bd8927d3e6a4ffc8ff1f31](https://github.com/moby/profiles/blob/61eaf32614c7c71b60bd8927d3e6a4ffc8ff1f31/seccomp/default.json).
Copyright The Moby Authors; Apache-2.0, see `SECCOMP-LICENSE`. The same upstream
revision's AppArmor template is the basis for `security/pagewright-worker.apparmor`.

`seccomp-worker.json` preserves the entire upstream default-deny profile and
appends exactly three rules, asserted by `TestWorkerSeccompDelta`:

- `clone` with `CLONE_NEWUSER` set (normal Linux argument ordering; s390 excluded).
- `unshare` with only NEWUSER, NEWNS, NEWNET, NEWPID, NEWIPC and NEWUTS flag bits.
- `mount`, `umount2`, `pivot_root` and `chroot`, needed to construct bubblewrap's
  isolated filesystem. The kernel still requires capabilities in the relevant
  namespace. The outer UID 1000 process has no capabilities and cannot mount.

`clone3` retains ENOSYS fallback; `setns`, BPF, keyring and other capability-gated
operations retain upstream restrictions. This exposes extra user-namespace kernel
surface and must stay coupled to UID 1000, CapDrop ALL and no-new-privileges.
It is not a general-purpose profile or proof against kernel vulnerabilities.
Only manager-created workers receive the embedded profile; no daemon defaults,
manager profile, host sysctl or host-wide LSM policies are modified.

When updating, review upstream changes and the delta, regenerate the worker JSON,
and run both daemon and installed-CLI negative acceptance tests. Never substitute
`seccomp=unconfined`. See [Docker's seccomp model](https://docs.docker.com/engine/security/seccomp/)
and [worker operations](../../../../../docs/WORKER_CLI.md).

M2.6 retains this container filter unchanged. The worker image additionally
installs `cmd/sandbox-bwrap`, a PATH wrapper that stacks a tool-only classic BPF
filter after distribution bubblewrap finishes setup. That filter denies further
namespace creation/entry, mount APIs and root changes, preserves ordinary clone
threads and returns ENOSYS for clone3 fallback. Its amd64/arm64 policy and branch
offsets have unit tests; installed acceptance verifies actual tool denial.

The separate `security/pagewright-worker-proc.apparmor` derives from the same
AppArmor template and adds explicit Docker-protected proc path denies. Only that
fixed profile opts into replacing Docker's proc overmounts; non-proc masks remain.
The shared launch policy and image compatibility guard are in `policy.go`.
See [the boundary and kernel review](../../../../../docs/WORKER_ISOLATION.md)
and [host evidence](../../../../../docs/M2_6_HOST_ACCEPTANCE.md).
