package nfs

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
)

// Publish a fully written inode with a no-replace hard link. Unlike rename,
// this is atomic across independent processes without overwriting a winner.
// Supported deployment: Linux local filesystem / Docker named volume.
func immutableWrite(root, path string, reader io.Reader) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, hash), reader)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Link(f.Name(), path); err != nil {
		if !os.IsExist(err) {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() != size {
			return storage.ErrConflict
		}
		existing, err := os.Open(path)
		if err != nil {
			return err
		}
		priorHash := sha256.New()
		_, copyErr := io.Copy(priorHash, existing)
		closeErr := existing.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if string(priorHash.Sum(nil)) != string(hash.Sum(nil)) {
			return storage.ErrConflict
		}
	}
	if err := os.Remove(f.Name()); err != nil {
		return err
	}
	// Flush the published name and every newly-created ancestor. Also do this
	// on identical retries to finish a prior request whose sync/response failed.
	for current := dir; ; current = filepath.Dir(current) {
		if err := syncDirectory(current); err != nil {
			return err
		}
		if current == root {
			return syncDirectory(filepath.Dir(root))
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}
