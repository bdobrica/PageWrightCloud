//go:build linux

// sandbox-bwrap is the bwrap selected by the trusted CLI's PATH. The runner's
// outer namespace invokes /usr/bin/bwrap directly. This adds a tool-only filter
// AFTER bwrap setup, without dropping the CLI's own inherited seccomp filters.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"syscall"
)

type instruction struct {
	Code   uint16
	Jt, Jf uint8
	K      uint32
}

func toolFilter(arch string) ([]byte, error) {
	var audit, clone uint32
	var denied []uint32
	switch arch {
	case "amd64":
		audit, clone = 0xc000003e, 56
		denied = []uint32{272, 308, 165, 166, 155, 161}
	case "arm64":
		audit, clone = 0xc00000b7, 220
		denied = []uint32{97, 268, 40, 39, 41, 51}
	default:
		return nil, fmt.Errorf("unsupported sandbox architecture")
	}
	// unshare, setns, mount, umount2, pivot_root, chroot; then new mount API.
	denied = append(denied, 428, 429, 430, 431, 432, 433, 442)
	const allow = 0x7fff0000
	const deniedErrno = 0x00050001 // EPERM
	code := []instruction{
		{Code: 0x20, K: 4}, // seccomp_data.arch
		{Code: 0x15, Jt: 1, K: audit},
		{Code: 0x06, K: 0x80000000}, // Kill alternate ABI rather than allow bypass.
		{Code: 0x20, K: 0},          // seccomp_data.nr
	}
	if arch == "amd64" {
		code = append(code, instruction{Code: 0x45, Jf: 1, K: 0x40000000}, instruction{Code: 0x06, K: 0x80000000})
	}
	// clone3 returns ENOSYS so runtimes can fall back to inspected clone flags.
	code = append(code, instruction{Code: 0x15, Jf: 1, K: 435}, instruction{Code: 0x06, K: 0x00050026})
	for _, nr := range denied {
		code = append(code, instruction{Code: 0x15, Jf: 1, K: nr}, instruction{Code: 0x06, K: deniedErrno})
	}
	code = append(code,
		instruction{Code: 0x15, Jf: 3, K: clone},
		instruction{Code: 0x20, K: 16},                // low word of args[0], supported little-endian ABIs
		instruction{Code: 0x45, Jf: 1, K: 0x7e020000}, // all clone namespace flags
		instruction{Code: 0x06, K: deniedErrno},
		instruction{Code: 0x06, K: allow},
	)
	var result bytes.Buffer
	if err := binary.Write(&result, binary.LittleEndian, code); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func run() error {
	data, err := toolFilter(runtime.GOARCH)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("/tmp", "pagewright-tool-filter-")
	if err != nil {
		return err
	}
	defer file.Close()
	if err := os.Remove(file.Name()); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	fd := file.Fd()
	// Preserve caller descriptors (including the CLI's seccomp/args FDs) and
	// allocate a new one rather than overwriting a guessed descriptor number.
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_SETFD, 0); errno != 0 {
		return errno
	}
	args := append([]string{"bwrap", "--add-seccomp-fd", strconv.Itoa(int(fd))}, os.Args[1:]...)
	return syscall.Exec("/usr/bin/bwrap", args, os.Environ())
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tool sandbox setup failed:", err)
		os.Exit(1)
	}
}
