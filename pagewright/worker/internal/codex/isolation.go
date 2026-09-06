package codex

import "os/exec"

// Outer namespace hides runner /proc, files and credentials from the CLI itself.
// Codex's inner workspace sandbox continues to deny tool network/outside writes.
// The PID namespace ensures even setsid descendants die when its init exits.
func wrapCLI(cmd *exec.Cmd, site, runtime string) {
	args := []string{"bwrap", "--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--die-with-parent", "--cap-drop", "ALL",
		"--ro-bind", "/usr", "/usr", "--ro-bind", "/bin", "/bin", "--ro-bind", "/lib", "/lib", "--ro-bind-try", "/lib64", "/lib64",
		"--ro-bind", "/etc", "/etc", "--ro-bind", "/opt/pagewright-cli", "/opt/pagewright-cli",
		"--proc", "/proc", "--dev", "/dev", "--size", "67108864", "--tmpfs", "/tmp",
		"--bind", site, site, "--bind", runtime, runtime, "--chdir", site, "--"}
	cmd.Args = append(args, cmd.Args...)
	cmd.Path = "/usr/bin/bwrap"
}
