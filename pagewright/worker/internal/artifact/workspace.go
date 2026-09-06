package artifact

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileState struct {
	Digest [32]byte
	Mode   os.FileMode
}
type Snapshot map[string]FileState

// SnapshotWorkspace rejects entries that Pack would otherwise silently exclude.
// The only runtime exception is the trusted instruction file patched by runner.
func SnapshotWorkspace(root string) (Snapshot, error) {
	result := Snapshot{}
	var total int64
	err := filepath.Walk(root, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "." && rel != ".codex" && rel != ".codex/instructions.md" && !allowedPath(rel) {
			return fmt.Errorf("disallowed workspace path %q", rel)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular workspace entry %q", rel)
		}
		if len(result) >= maxEntries || info.Size() > maxFileBytes {
			return fmt.Errorf("workspace exceeds limits")
		}
		state := FileState{Mode: info.Mode()}
		if info.Mode().IsRegular() {
			total += info.Size()
			if total > maxExpandedBytes {
				return fmt.Errorf("workspace exceeds limits")
			}
			if info.Mode().Perm()&0111 != 0 {
				return fmt.Errorf("executable source/output file %q", rel)
			}
			f, err := os.Open(name)
			if err != nil {
				return err
			}
			h := sha256.New()
			n, copyErr := io.Copy(h, io.LimitReader(f, maxFileBytes+1))
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if n != info.Size() {
				return fmt.Errorf("workspace changed while reading")
			}
			copy(state.Digest[:], h.Sum(nil))
		}
		result[rel] = state
		return nil
	})
	return result, err
}

// Only content/ changes are accepted. public/, manifest and instructions must
// remain byte/mode-identical, including additions/deletions and directories.
func ValidateChanges(before, after Snapshot) ([]string, error) {
	all := map[string]bool{}
	for name := range before {
		all[name] = true
	}
	for name := range after {
		all[name] = true
	}
	changed := []string{}
	for name := range all {
		old, oldOK := before[name]
		next, nextOK := after[name]
		if oldOK == nextOK && old == next {
			continue
		}
		if !strings.HasPrefix(name, "content/") {
			return nil, fmt.Errorf("edit outside allowed source: %s", name)
		}
		if (oldOK && !old.Mode.IsDir()) || (nextOK && !next.Mode.IsDir()) {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// FreezeContent copies only validated source into a sibling directory outside
// the agent's writable root and verifies copied bytes against the snapshot.
func FreezeContent(site, dest string, snapshot Snapshot) error {
	for name, state := range snapshot {
		if name != "content" && !strings.HasPrefix(name, "content/") {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if state.Mode.IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		f, err := os.Open(filepath.Join(site, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
		closeErr := f.Close()
		if closeErr != nil {
			return closeErr
		}
		if err != nil {
			return err
		}
		if sha256.Sum256(data) != state.Digest {
			return fmt.Errorf("source changed during freeze: %s", name)
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			return err
		}
	}
	return nil
}
