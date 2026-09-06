package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
	"github.com/gorilla/mux"
)

type writeCommit struct {
	JobID         string `json:"job_id"`
	SiteID        string `json:"site_id"`
	OwnerID       string `json:"owner_id"`
	SourceVersion string `json:"source_version"`
	TargetVersion string `json:"target_version"`
	LockToken     string `json:"lock_token"`
	FencingToken  int64  `json:"fencing_token"`
	Part          string `json:"part"`
	SHA256        string `json:"sha256"`
	Size          int64  `json:"size"`
}

// Production must explicitly construct this handler. Plain NewHandler is the
// isolated transport/backend fixture used by unit tests, not a runtime switch.
func NewFencedHandler(backend storage.Backend, managerURL string) (*Handler, error) {
	u, err := url.Parse(managerURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid commit authority URL")
	}
	if _, ok := backend.(storage.FencedBackend); !ok {
		return nil, fmt.Errorf("backend lacks fenced publication")
	}
	return &Handler{backend: backend, commitURL: strings.TrimRight(managerURL, "/")}, nil
}

func (h *Handler) fencedWrite(part string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		if h.commitURL == "" || r.Method == "GET" || vars["build_id"] == "initial" {
			next(w, r)
			return
		}
		var commit writeCommit
		header := r.Header.Get("X-Pagewright-Attempt")
		if len(header) > 4096 || json.Unmarshal([]byte(header), &commit) != nil || commit.JobID == "" || commit.LockToken == "" || commit.FencingToken <= 0 || commit.SiteID != vars["site_id"] || commit.TargetVersion != vars["build_id"] {
			http.Error(w, "matching worker attempt required", 409)
			return
		}
		commit.Part = part
		copy := *h
		copy.attempt = &commit
		copy.backend = h.backend.(storage.FencedBackend).WithWriteGuard(func(digest string, size int64) error {
			commit.SHA256, commit.Size = digest, size
			data, err := json.Marshal(commit)
			if err != nil {
				return err
			}
			req, err := http.NewRequestWithContext(r.Context(), "POST", h.commitURL+"/jobs/"+url.PathEscape(commit.JobID)+"/write-commit", bytes.NewReader(data))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			response, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("commit authority unavailable")
			}
			defer response.Body.Close()
			if response.StatusCode == 409 {
				return storage.ErrFenced
			}
			if response.StatusCode != 204 {
				return fmt.Errorf("commit authority unavailable")
			}
			return nil
		})
		if part == "artifact" {
			r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
			copy.StoreArtifact(w, r)
		} else {
			copy.VersionMetadata(w, r)
		}
	}
}
