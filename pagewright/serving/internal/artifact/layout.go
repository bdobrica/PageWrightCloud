package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// Layout policy is identical in worker and serving; integration checks drift.
// These conservative MVP limits also bound validation/decompression work.
const maxArchiveBytes int64 = 64 << 20
const maxExpandedBytes int64 = 256 << 20
const maxFileBytes int64 = 32 << 20
const maxEntries = 10000

type LayoutManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	ThemeID       string `json:"theme_id"`
	FileCount     int    `json:"-"`
	TotalSize     int64  `json:"-"`
}

func allowedPath(name string) bool {
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	if name == "" || len(name) > 1024 || path.Clean(name) != name || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return false
	}
	if name == "manifest.json" {
		return true
	}
	parts := strings.Split(name, "/")
	if parts[0] != "content" && parts[0] != "public" {
		return false
	}
	for _, part := range parts {
		lower := strings.ToLower(part)
		if strings.HasPrefix(lower, "credentials.") || strings.HasPrefix(lower, "secrets.") || strings.HasPrefix(lower, "instructions.") || strings.HasPrefix(lower, "prompt.") || lower == "id_rsa" || lower == "id_ed25519" || strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx") {
			return false
		}
		if strings.HasPrefix(part, ".") || lower == "instructions.md" || lower == "manifest.json" || lower == "execution.json" || lower == "logs" || lower == "secrets" || lower == "credentials" || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".log") {
			return false
		}
	}
	if parts[0] == "public" {
		if len(parts) > 1 && (strings.EqualFold(parts[1], "content") || strings.EqualFold(parts[1], "source") || strings.EqualFold(parts[1], "src")) {
			return false
		}
		lower := strings.ToLower(path.Base(name))
		if lower == "site.json" || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".mdx") || strings.HasSuffix(lower, ".map") {
			return false
		}
	}
	return true
}

// readArchive validates every entry and trailer. It writes only inside a fresh,
// caller-owned stage; publicOnly discards source rather than putting it on www.
func readArchive(archive, stage string, publicOnly bool) (LayoutManifest, error) {
	var manifest LayoutManifest
	f, err := os.Open(archive)
	if err != nil {
		return manifest, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return manifest, err
	}
	if info.Size() > maxArchiveBytes {
		return manifest, fmt.Errorf("compressed archive too large")
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		return manifest, err
	}
	defer gz.Close()
	// Include headers, padding and gzip trailer drain in the expansion limit.
	limited := &io.LimitedReader{R: gz, N: maxExpandedBytes + 1}
	tr := tar.NewReader(limited)
	seen := map[string]byte{}
	parents := map[string]bool{}
	var config []byte
	hasPublic, hasHome, hasIndex, hasManifest := false, false, false, false
	entries := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, err
		}
		entries++
		if entries > maxEntries {
			return manifest, fmt.Errorf("too many archive entries")
		}
		name := h.Name
		if h.Typeflag == tar.TypeDir {
			name = strings.TrimSuffix(name, "/")
		}
		if !allowedPath(name) {
			return manifest, fmt.Errorf("disallowed archive path %q", h.Name)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			return manifest, fmt.Errorf("unsupported archive entry %q", name)
		}
		if (name == "manifest.json" && h.Typeflag != tar.TypeReg) || ((name == "content" || name == "public") && h.Typeflag != tar.TypeDir) || len(h.PAXRecords) > 0 || len(h.Xattrs) > 0 {
			return manifest, fmt.Errorf("invalid archive entry %q", name)
		}
		if _, ok := seen[name]; ok {
			return manifest, fmt.Errorf("duplicate archive path %q", name)
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if kind, ok := seen[parent]; ok && kind != tar.TypeDir {
				return manifest, fmt.Errorf("file/directory conflict")
			}
			parents[parent] = true
		}
		if h.Typeflag == tar.TypeReg {
			if parents[name] {
				return manifest, fmt.Errorf("file/directory conflict")
			}
		}
		seen[name] = h.Typeflag
		if h.Size < 0 || h.Size > maxFileBytes {
			return manifest, fmt.Errorf("archive entry too large")
		}
		if h.Typeflag == tar.TypeDir && h.Size != 0 {
			return manifest, fmt.Errorf("directory has data")
		}
		if name == "public" || strings.HasPrefix(name, "public/") {
			hasPublic = true
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		manifest.FileCount++
		manifest.TotalSize += h.Size
		if name == "manifest.json" || name == "content/site.json" {
			if h.Size > 64<<10 {
				return manifest, fmt.Errorf("manifest/config too large")
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				return manifest, err
			}
			if name == "manifest.json" {
				decoder := json.NewDecoder(bytes.NewReader(data))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&manifest) != nil || decoder.Decode(new(interface{})) != io.EOF {
					return manifest, fmt.Errorf("invalid layout manifest")
				}
				hasManifest = true
			} else {
				config = data
			}
			if !publicOnly {
				if err := writeStageFile(stage, name, bytes.NewReader(data)); err != nil {
					return manifest, err
				}
			}
			continue
		}
		if name == "content/home/index.md" || name == "content/home/index.mdx" {
			hasHome = h.Size > 0
		}
		if name == "public/index.html" {
			hasIndex = h.Size > 0
		}
		if !publicOnly || strings.HasPrefix(name, "public/") {
			if err := writeStageFile(stage, name, tr); err != nil {
				return manifest, err
			}
		} else {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return manifest, err
			}
		}
	}
	// tar EOF is not gzip EOF. Reject hidden trailing payload, allowing only tar
	// zero padding, and consume the checksum within the decompression budget.
	buffer := make([]byte, 32768)
	for {
		n, err := limited.Read(buffer)
		for _, b := range buffer[:n] {
			if b != 0 {
				return manifest, fmt.Errorf("data after tar end")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, err
		}
	}
	if limited.N <= 0 {
		return manifest, fmt.Errorf("expanded archive too large")
	}
	var site struct {
		Name string `json:"site_name"`
	}
	if json.Unmarshal(config, &site) != nil || strings.TrimSpace(site.Name) == "" || !hasHome {
		return manifest, fmt.Errorf("valid site.json and home source required")
	}
	// M1.6 persisted source-only bootstraps predate in-archive manifests.
	if !hasManifest && !hasPublic && !publicOnly {
		manifest.SchemaVersion = 1
		manifest.Kind = "source"
		manifest.ThemeID = "starter"
	}
	if manifest.SchemaVersion != 1 || manifest.ThemeID != "starter" || (manifest.Kind != "source" && manifest.Kind != "compiled") {
		return manifest, fmt.Errorf("unsupported layout manifest")
	}
	if hasPublic != (manifest.Kind == "compiled") || (hasPublic && !hasIndex) {
		return manifest, fmt.Errorf("public layout does not match manifest")
	}
	if publicOnly && (!hasManifest || manifest.Kind != "compiled") {
		return manifest, fmt.Errorf("source-only archive cannot be deployed")
	}
	return manifest, nil
}

func writeStageFile(stage, name string, reader io.Reader) error {
	if stage == "" {
		_, err := io.Copy(io.Discard, reader)
		return err
	}
	target := filepath.Join(stage, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, reader)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
