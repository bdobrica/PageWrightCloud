package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/artifact"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/nginx"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/storage"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/types"
	"github.com/gorilla/mux"
)

type Handler struct {
	deploymentMu sync.Mutex
	artifactMgr  *artifact.Manager
	nginxMgr     *nginx.Manager
	storageCli   *storage.Client
}

func NewHandler(artifactMgr *artifact.Manager, nginxMgr *nginx.Manager, storageCli *storage.Client) *Handler {
	return &Handler{
		artifactMgr: artifactMgr,
		nginxMgr:    nginxMgr,
		storageCli:  storageCli,
	}
}

func (h *Handler) SetupRoutes() *mux.Router {
	r := mux.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if fqdn := mux.Vars(r)["fqdn"]; fqdn != "" {
				if !types.ValidHost(fqdn) {
					http.Error(w, "invalid site hostname", 400)
					return
				}
				if err := h.artifactMgr.CheckSitePath(fqdn); err != nil {
					http.Error(w, "invalid site path", 400)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	})

	r.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// Site management
	r.HandleFunc("/sites/{fqdn}/artifacts", h.DeployArtifact).Methods("POST")
	r.HandleFunc("/sites/{fqdn}/deployment", h.ApplyDeployment).Methods("POST")
	r.HandleFunc("/sites/{fqdn}/activate", h.ActivatePublic).Methods("POST")
	r.HandleFunc("/sites/{fqdn}/preview", h.ActivatePreview).Methods("POST")
	r.HandleFunc("/sites/{fqdn}/aliases", h.ManageAliases).Methods("POST")
	r.HandleFunc("/sites/{fqdn}/disable", h.DisableSite).Methods("POST")
	r.HandleFunc("/sites/{fqdn}/enable", h.EnableSite).Methods("POST")
	r.HandleFunc("/sites/{fqdn}", h.RemoveSite).Methods("DELETE")

	// Maintenance mode
	r.HandleFunc("/maintenance/enable", h.EnableMaintenance).Methods("POST")
	r.HandleFunc("/maintenance/disable", h.DisableMaintenance).Methods("POST")

	return r
}

func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "healthy",
		"maintenance": fmt.Sprintf("%v", h.nginxMgr.IsMaintenanceMode()),
	})
}

func (h *Handler) DeployArtifact(w http.ResponseWriter, r *http.Request) {
	h.deploymentMu.Lock()
	defer h.deploymentMu.Unlock()
	if !h.legacyDeploymentAllowed(w, r) {
		return
	}
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	var req types.DeployRequest
	if err := boundedJSON(r, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Download artifact from storage
	if !deploymentID.MatchString(req.SiteID) || !deploymentID.MatchString(req.Version) {
		http.Error(w, "invalid artifact identity", 400)
		return
	}
	tmp, err := os.CreateTemp("", "pagewright-artifact-*.tar.gz")
	if err != nil {
		http.Error(w, "cannot stage artifact", 503)
		return
	}
	tmpFile := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpFile)

	if err := h.storageCli.FetchArtifactContext(r.Context(), req.SiteID, req.Version, tmpFile); err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch artifact: %v", err), http.StatusInternalServerError)
		return
	}

	// Deploy artifact
	if err := h.artifactMgr.DeployArtifact(fqdn, req.Version, tmpFile); err != nil {
		http.Error(w, fmt.Sprintf("Failed to deploy artifact: %v", err), http.StatusInternalServerError)
		return
	}

	// Cleanup old versions
	if err := h.artifactMgr.CleanupOldVersions(fqdn, req.Version); err != nil {
		fmt.Printf("Warning: failed to cleanup old versions: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Artifact deployed successfully",
		"fqdn":    fqdn,
		"version": req.Version,
	})
}

func (h *Handler) ActivatePublic(w http.ResponseWriter, r *http.Request) {
	h.deploymentMu.Lock()
	defer h.deploymentMu.Unlock()
	if !h.legacyDeploymentAllowed(w, r) {
		return
	}
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	var req types.ActivateRequest
	if err := boundedJSON(r, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure nginx config exists
	if !deploymentID.MatchString(req.Version) {
		http.Error(w, "invalid version identity", 400)
		return
	}
	sitePath := h.artifactMgr.GetSitePath(fqdn)
	if err := h.nginxMgr.CreateSiteConfig(fqdn, sitePath, nil, true); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update nginx config: %v", err), http.StatusInternalServerError)
		return
	}
	if err := h.artifactMgr.ActivateVersion(fqdn, req.Version, false); err != nil {
		http.Error(w, "Failed to activate version", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Public version activated",
		"fqdn":    fqdn,
		"version": req.Version,
	})
}

func (h *Handler) ActivatePreview(w http.ResponseWriter, r *http.Request) {
	h.deploymentMu.Lock()
	defer h.deploymentMu.Unlock()
	if !h.legacyDeploymentAllowed(w, r) {
		return
	}
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	var req types.ActivateRequest
	if err := boundedJSON(r, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if !deploymentID.MatchString(req.Version) {
		http.Error(w, "invalid version identity", 400)
		return
	}
	if err := h.nginxMgr.EnsureSiteConfig(fqdn, h.artifactMgr.GetSitePath(fqdn)); err != nil {
		http.Error(w, "Failed to provision preview routing", http.StatusInternalServerError)
		return
	}
	if err := h.artifactMgr.ActivateVersion(fqdn, req.Version, true); err != nil {
		http.Error(w, "Failed to activate preview", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Preview version activated",
		"fqdn":    fqdn,
		"version": req.Version,
	})
}

func (h *Handler) ManageAliases(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	var req types.AliasRequest
	if err := boundedJSON(r, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	sitePath := h.artifactMgr.GetSitePath(fqdn)

	// TODO: Load existing aliases and merge/remove based on action
	// For now, just set the aliases
	if err := h.nginxMgr.UpdateAliases(fqdn, sitePath, req.Aliases, true); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update aliases: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Aliases updated",
		"fqdn":    fqdn,
	})
}

func (h *Handler) DisableSite(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	sitePath := h.artifactMgr.GetSitePath(fqdn)
	if err := h.nginxMgr.CreateSiteConfig(fqdn, sitePath, nil, false); err != nil {
		http.Error(w, fmt.Sprintf("Failed to disable site: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Site disabled",
		"fqdn":    fqdn,
	})
}

func (h *Handler) EnableSite(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	sitePath := h.artifactMgr.GetSitePath(fqdn)
	if err := h.nginxMgr.CreateSiteConfig(fqdn, sitePath, nil, true); err != nil {
		http.Error(w, fmt.Sprintf("Failed to enable site: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Site enabled",
		"fqdn":    fqdn,
	})
}

func (h *Handler) RemoveSite(w http.ResponseWriter, r *http.Request) {
	h.deploymentMu.Lock()
	defer h.deploymentMu.Unlock()
	if !h.legacyDeploymentAllowed(w, r) {
		return
	}
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	// Check active pointers/evidence before removing routing, under the artifact
	// writer lock so activation cannot race the deletion guard.
	if err := h.artifactMgr.RemoveSite(fqdn, func() error { return h.nginxMgr.RemoveSiteConfig(fqdn) }); err != nil {
		http.Error(w, fmt.Sprintf("Failed to remove site files: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Site removed",
		"fqdn":    fqdn,
	})
}

func (h *Handler) EnableMaintenance(w http.ResponseWriter, r *http.Request) {
	if err := h.nginxMgr.SetMaintenanceMode(true); err != nil {
		http.Error(w, fmt.Sprintf("Failed to enable maintenance mode: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Maintenance mode enabled",
	})
}

func (h *Handler) DisableMaintenance(w http.ResponseWriter, r *http.Request) {
	if err := h.nginxMgr.SetMaintenanceMode(false); err != nil {
		http.Error(w, fmt.Sprintf("Failed to disable maintenance mode: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Maintenance mode disabled",
	})
}
