package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
	"github.com/gorilla/mux"
)

const metadataLimit = 4 << 20

// These endpoints belong to the internal storage API, never static hosting or
// unauthenticated gateway routes. Internal-service authentication remains M4.
func (h *Handler) VersionMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	vars := mux.Vars(r)
	site, version := vars["site_id"], vars["build_id"]
	if !artifactID.MatchString(site) || !artifactID.MatchString(version) {
		http.Error(w, "invalid identity", 400)
		return
	}
	backend, ok := h.backend.(storage.VersionMetadata)
	if !ok {
		http.Error(w, "metadata unavailable", 501)
		return
	}
	manifest := strings.HasSuffix(r.URL.Path, "/manifest")
	if r.Method == "GET" {
		var data []byte
		var err error
		if manifest {
			data, err = backend.FetchManifest(site, version)
		} else {
			data, err = backend.FetchPrivateLog(site, version)
		}
		if err != nil {
			status := 500
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, storage.ErrIncomplete) {
				status = 404
			}
			http.Error(w, "metadata unavailable", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	encoding := strings.TrimSpace(strings.Join(r.Header.Values("Content-Encoding"), ","))
	if err != nil || media != "application/json" || (encoding != "" && !strings.EqualFold(encoding, "identity")) {
		http.Error(w, "JSON identity representation required", 415)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, metadataLimit))
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			http.Error(w, "metadata too large", 413)
		} else {
			http.Error(w, "invalid body", 400)
		}
		return
	}
	if manifest {
		var identity storage.ManifestIdentity
		if json.Unmarshal(data, &identity) != nil || identity.SiteID != site || identity.BuildID != version || identity.CreatedAt.IsZero() {
			http.Error(w, "manifest requires matching site_id/build_id and created_at", 400)
			return
		}
		err = backend.CommitManifest(site, version, data)
	} else {
		var log struct {
			Content *string `json:"content"`
		}
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&log) != nil || log.Content == nil || decoder.Decode(new(interface{})) != io.EOF {
			http.Error(w, "log requires a content string only", 400)
			return
		}
		err = backend.StorePrivateLog(site, version, data)
	}
	if err != nil {
		status := 500
		if errors.Is(err, storage.ErrIncomplete) || errors.Is(err, storage.ErrConflict) {
			status = 409
		}
		http.Error(w, "metadata write failed", status)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
