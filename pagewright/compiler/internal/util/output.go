package util

import (
	"bytes"
	"fmt"
)

// OutputBuffer bounds template/Markdown expansion before allocation grows.
type OutputBuffer struct{ buffer bytes.Buffer }

func (b *OutputBuffer) Write(p []byte) (int, error) {
	if len(p) > (32<<20)-b.buffer.Len() {
		return 0, fmt.Errorf("rendered output too large")
	}
	return b.buffer.Write(p)
}
func (b *OutputBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *OutputBuffer) String() string { return b.buffer.String() }
