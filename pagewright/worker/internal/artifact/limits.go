package artifact

import (
	"fmt"
	"io"
)

// Stop compression output before it fills the staging volume. Pack's temp file
// is removed on failure and the previous destination is never overwritten.
type archiveWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *archiveWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, fmt.Errorf("compressed archive too large")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}
