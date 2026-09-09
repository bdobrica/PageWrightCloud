// Offline, privileged operator import/export. Not exposed over HTTP or included
// in production images. Never changes existing artifacts, jobs or deployments.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage/nfs"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var id = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
var sha = regexp.MustCompile(`^[a-f0-9]{64}$`)

type receipt struct {
	Site         string    `json:"site_id"`
	Target       string    `json:"build_id"`
	Base         string    `json:"base_build_id"`
	Provenance   string    `json:"provenance"`
	Calls        *int      `json:"provider_calls"`
	ArtifactHash string    `json:"artifact_sha256"`
	BaseHash     string    `json:"base_archive_sha256"`
	Kind         string    `json:"kind"`
	Passed       bool      `json:"checks_passed"`
	Browser      bool      `json:"browser_checks_performed"`
	Checks       []string  `json:"validation_checks"`
	Created      time.Time `json:"created_at"`
}

func hash(r io.Reader) (string, error) {
	h := sha256.New()
	_, err := io.Copy(h, r)
	return hex.EncodeToString(h.Sum(nil)), err
}
func regular(path string, limit int64) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("invalid operator input file")
	}
	return os.Open(path)
}
func importArtifact(backend *nfs.NFSBackend, bundle, site, base, target string) error {
	if !id.MatchString(site) || !id.MatchString(base) || !id.MatchString(target) || !strings.HasPrefix(target, "operator-m49-") || base == target {
		return fmt.Errorf("invalid operator artifact identity")
	}
	f, err := regular(filepath.Join(bundle, "manifest.json"), 65536)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	f.Close()
	if err != nil || len(data) > 65536 {
		return fmt.Errorf("invalid manifest size")
	}
	var r receipt
	if json.Unmarshal(data, &r) != nil || r.Site != site || r.Base != base || r.Target != target || r.Provenance != "operator-acceptance-v1" || r.Calls == nil || *r.Calls != 0 || r.Kind != "compiled" || !r.Passed || r.Browser || r.Created.IsZero() || !sha.MatchString(r.ArtifactHash) || !sha.MatchString(r.BaseHash) || !reflect.DeepEqual(r.Checks, []string{"allowed_source_changes", "trusted_compiler", "html_and_local_references", "archive_layout"}) {
		return fmt.Errorf("invalid operator compilation receipt")
	}
	if _, err = backend.FetchManifest(site, base); err != nil {
		return err
	}
	source, err := backend.FetchArtifact(site, base)
	if err != nil {
		return err
	}
	baseHash, err := hash(source)
	source.Close()
	if err != nil || baseHash != r.BaseHash {
		return fmt.Errorf("base archive mismatch")
	}
	archive, err := regular(filepath.Join(bundle, "artifact.tar.gz"), 128<<20)
	if err != nil {
		return err
	}
	defer archive.Close()
	artifactHash, err := hash(archive)
	if err != nil || artifactHash != r.ArtifactHash {
		return fmt.Errorf("compiled archive mismatch")
	}
	if _, err = archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	// Explicit operator import, not an impersonated fenced worker. Same immutable
	// persistence and manifest-last visibility as the storage service.
	guarded := backend.WithWriteGuard(func(digest string, size int64) error {
		if digest != r.ArtifactHash || size > 128<<20 {
			return fmt.Errorf("compiled archive changed before commit")
		}
		return nil
	})
	if err = guarded.StoreArtifact(site, target, archive); err != nil {
		return err
	}
	log := []byte(`{"provenance":"operator-acceptance-v1","provider_calls":0,"job_created":false,"browser_checks_performed":false}`)
	if err = backend.StorePrivateLog(site, target, log); err != nil {
		return err
	}
	return backend.CommitManifest(site, target, data)
}
func main() {
	root := flag.String("root", "/nfs", "operator-mounted storage root")
	site := flag.String("site", "", "operator-verified site ID")
	base := flag.String("base", "", "committed base version")
	target := flag.String("target", "", "new operator-m49- version")
	bundle := flag.String("bundle", "", "trusted compiler output directory")
	export := flag.Bool("export", false, "export committed base archive to stdout; read only")
	flag.Parse()
	backend, err := nfs.NewNFSBackend(*root)
	if err == nil && *export {
		_, err = backend.FetchManifest(*site, *base)
		if err == nil {
			var source io.ReadCloser
			source, err = backend.FetchArtifact(*site, *base)
			if err == nil {
				_, err = io.Copy(os.Stdout, source)
				source.Close()
			}
		}
	} else if err == nil {
		err = importArtifact(backend, *bundle, *site, *base, *target)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Operator artifact operation failed:", err)
		os.Exit(1)
	}
	if !*export {
		fmt.Println("Operator artifact committed; no AI job or deployment created:", *target)
	}
}
