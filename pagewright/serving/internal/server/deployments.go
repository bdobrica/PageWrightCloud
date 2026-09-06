package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gorilla/mux"
)

type deploymentReceipt struct {
	SiteID   string `json:"site_id"`
	Sequence int64  `json:"sequence"`
	FQDN     string `json:"fqdn"`
	Version  string `json:"version"`
	Target   string `json:"target"`
	Status   string `json:"status"`
}

var deploymentID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

func (h *Handler) receiptPath(fqdn string) string {
	return filepath.Join(h.artifactMgr.GetSitePath(fqdn), ".deployment.json")
}
func readReceipt(path string) (*deploymentReceipt, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 4097))
	decoder.DisallowUnknownFields()
	var d deploymentReceipt
	if err = decoder.Decode(&d); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF || !validDeployment(d) || (d.Status != "pending" && d.Status != "activating" && d.Status != "completed" && d.Status != "failed") {
		return nil, errors.New("invalid deployment receipt")
	}
	return &d, nil
}
func validDeployment(d deploymentReceipt) bool {
	return d.Sequence > 0 && len(d.FQDN) <= 245 && deploymentID.MatchString(d.SiteID) && deploymentID.MatchString(d.Version) && deploymentID.MatchString(d.FQDN) && (d.Target == "live" || d.Target == "preview")
}
func saveReceipt(path string, d *deploymentReceipt) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".receipt-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = json.NewEncoder(f).Encode(d); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// One serving writer owns the volume (M3.5 supervisor lock). Persist the sequence
// before side effects; retries are identical and older requests never reactivate.
func (h *Handler) ApplyDeployment(w http.ResponseWriter, r *http.Request) {
	h.deploymentMu.Lock()
	defer h.deploymentMu.Unlock()
	if r.Context().Err() != nil {
		return
	}
	var d deploymentReceipt
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil || decoder.Decode(new(any)) != io.EOF || !validDeployment(d) || d.Status != "pending" || d.FQDN != mux.Vars(r)["fqdn"] {
		http.Error(w, "invalid deployment", 400)
		return
	}
	path := h.receiptPath(d.FQDN)
	prior, err := readReceipt(path)
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, "deployment receipt requires repair", 503)
		return
	}
	if prior != nil {
		if prior.FQDN != d.FQDN || prior.SiteID != d.SiteID || prior.Sequence > d.Sequence || (prior.Sequence < d.Sequence && (prior.Status == "pending" || prior.Status == "activating")) {
			http.Error(w, "conflicting deployment", 409)
			return
		}
		if prior.Sequence == d.Sequence {
			if prior.Version != d.Version || prior.Target != d.Target || prior.FQDN != d.FQDN {
				http.Error(w, "deployment identity conflict", 409)
				return
			}
			d = *prior
		}
	}
	if err = saveReceipt(path, &d); err != nil {
		http.Error(w, "cannot persist deployment intent", 503)
		return
	}
	if d.Status == "pending" || d.Status == "activating" {
		// A pre-activation failure has a durable terminal receipt: no pointer changed.
		tmp, err := os.CreateTemp("", "pagewright-deployment-*.tar.gz")
		if err != nil {
			http.Error(w, "staging unavailable", 503)
			return
		}
		archive := tmp.Name()
		tmp.Close()
		defer os.Remove(archive)
		err = h.storageCli.FetchArtifact(d.SiteID, d.Version, archive)
		if err == nil {
			err = h.artifactMgr.DeployArtifact(d.FQDN, d.Version, archive)
		}
		if err == nil {
			err = h.nginxMgr.EnsureSiteConfig(d.FQDN, h.artifactMgr.GetSitePath(d.FQDN))
		}
		if err != nil {
			if d.Status == "activating" {
				http.Error(w, "activation recovery unavailable", 503)
				return
			}
			d.Status = "failed"
		} else {
			d.Status = "activating"
			if err = saveReceipt(path, &d); err != nil {
				http.Error(w, "cannot persist activation intent", 503)
				return
			}
			if err = h.artifactMgr.ActivateVersion(d.FQDN, d.Version, d.Target == "preview"); err != nil {
				http.Error(w, "activation uncertain; retry same deployment", 503)
				return
			}
			d.Status = "completed"
		}
	}
	if d.Status == "completed" {
		name := "public"
		if d.Target == "preview" {
			name = "preview"
		}
		link, err := os.Readlink(filepath.Join(h.artifactMgr.GetSitePath(d.FQDN), name))
		if err != nil || link != filepath.Join("artifacts", d.Version, "public") {
			http.Error(w, "deployment pointer differs; operator repair required", 503)
			return
		}
	}
	if err = saveReceipt(path, &d); err != nil {
		http.Error(w, "cannot confirm deployment receipt", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

// Unfenced legacy writes/deletion may not bypass an enrolled site's sequence.
func (h *Handler) legacyDeploymentAllowed(w http.ResponseWriter, r *http.Request) bool {
	fqdn := mux.Vars(r)["fqdn"]
	if !deploymentID.MatchString(fqdn) {
		http.Error(w, "invalid site", 400)
		return false
	}
	if _, err := os.Lstat(h.receiptPath(fqdn)); !os.IsNotExist(err) {
		http.Error(w, "site uses fenced deployments", 409)
		return false
	}
	return true
}
