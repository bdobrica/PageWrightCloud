//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// Evaluate the emitted classic BPF, including branch offsets, for both ABIs.
func evaluate(t *testing.T, code []byte, arch, nr, flags uint32) uint32 {
	t.Helper()
	var accumulator uint32
	for pc := 0; pc < len(code)/8; pc++ {
		var ins instruction
		if err := binary.Read(bytes.NewReader(code[pc*8:]), binary.LittleEndian, &ins); err != nil {
			t.Fatal(err)
		}
		switch ins.Code {
		case 0x20:
			switch ins.K {
			case 0:
				accumulator = nr
			case 4:
				accumulator = arch
			case 16:
				accumulator = flags
			default:
				t.Fatalf("unexpected offset %d", ins.K)
			}
		case 0x15, 0x45:
			match := accumulator == ins.K
			if ins.Code == 0x45 {
				match = accumulator&ins.K != 0
			}
			if match {
				pc += int(ins.Jt)
			} else {
				pc += int(ins.Jf)
			}
		case 0x06:
			return ins.K
		default:
			t.Fatalf("unexpected opcode %x", ins.Code)
		}
	}
	t.Fatal("filter fell through")
	return 0
}

func TestToolFilter(t *testing.T) {
	for _, tc := range []struct {
		arch         string
		audit, clone uint32
		denied       []uint32
	}{
		{"amd64", 0xc000003e, 56, []uint32{272, 308, 165, 166, 155, 161}},
		{"arm64", 0xc00000b7, 220, []uint32{97, 268, 40, 39, 41, 51}},
	} {
		t.Run(tc.arch, func(t *testing.T) {
			code, err := toolFilter(tc.arch)
			if err != nil {
				t.Fatal(err)
			}
			check := func(arch, nr, flags, want uint32) {
				t.Helper()
				if got := evaluate(t, code, arch, nr, flags); got != want {
					t.Errorf("arch=%x syscall=%d flags=%x: got %x want %x", arch, nr, flags, got, want)
				}
			}
			check(0, 0, 0, 0x80000000)
			check(tc.audit, 435, 0, 0x00050026)
			check(tc.audit, tc.clone, 0, 0x7fff0000)
			check(tc.audit, tc.clone, 0x00010f00, 0x7fff0000) // ordinary thread
			for _, flag := range []uint32{0x20000, 0x02000000, 0x04000000, 0x08000000, 0x10000000, 0x20000000, 0x40000000} {
				check(tc.audit, tc.clone, flag, 0x00050001)
			}
			for _, nr := range append(tc.denied, 428, 429, 430, 431, 432, 433, 442) {
				check(tc.audit, nr, 0, 0x00050001)
			}
			check(tc.audit, 1, 0, 0x7fff0000)
			if tc.arch == "amd64" {
				check(tc.audit, 0x40000001, 0, 0x80000000)
			}
		})
	}
	if _, err := toolFilter("unsupported"); err == nil {
		t.Fatal("unsupported architecture accepted")
	}
}
