package util

import "testing"

func TestOutputBufferLimit(t *testing.T) {
	var b OutputBuffer
	chunk := make([]byte, 1<<20)
	for i := 0; i < 32; i++ {
		if _, err := b.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Write([]byte("x")); err == nil {
		t.Fatal("render buffer overflow accepted")
	}
	if len(b.Bytes()) != 32<<20 {
		t.Fatal("overflow modified output")
	}
}
