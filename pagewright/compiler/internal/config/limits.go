package config

import (
	"fmt"
	"io"
	"os"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/util"
)

func readMetadata(path string) ([]byte, error) {
	if err := util.CheckPath(path, false); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("metadata must be a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if len(data) > 64<<10 {
		return nil, fmt.Errorf("compiler metadata too large")
	}
	return data, err
}
