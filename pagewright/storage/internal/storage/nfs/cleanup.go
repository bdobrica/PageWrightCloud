package nfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Supported deployment is Linux local filesystem / Docker named volume, as for
// immutable publication. Do not unlink this stable lock file during operation.
func uploadLock(root string, exclusive bool) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(root, ".uploads.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX | syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), mode); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil // Close releases the process-independent lock.
}

// CleanupStaging never follows symlinks or removes directories/final objects.
// Exclusive upload locking proves no supported writer can still use these files.
func (n *NFSBackend) CleanupStaging(ctx context.Context, now time.Time) (int, error) {
	lock, err := uploadLock(n.basePath, true)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer lock.Close()
	removed, visited := 0, 0
	err = filepath.WalkDir(n.basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > 100000 {
			return fmt.Errorf("staging scan exceeds 100000 entries; offline maintenance required")
		}
		if removed >= 128 {
			return fs.SkipAll
		}
		if !d.Type().IsRegular() || !strings.HasPrefix(d.Name(), ".upload-") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(now.Add(-7 * 24 * time.Hour)) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		removed++
		return syncDirectory(filepath.Dir(path))
	})
	return removed, err
}

func (n *NFSBackend) MaintainStaging(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		operation, cancel := context.WithTimeout(ctx, 5*time.Second)
		removed, err := n.CleanupStaging(operation, time.Now())
		cancel()
		if removed > 0 {
			log.Printf("Removed %d abandoned staging files past seven-day retention", removed)
		}
		if err != nil && ctx.Err() == nil {
			log.Print("Staging cleanup deferred")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
