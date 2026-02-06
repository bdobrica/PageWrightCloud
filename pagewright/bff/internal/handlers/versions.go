package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/PageWrightCloud/pagewright/bff/internal/clients"
	"github.com/PageWrightCloud/pagewright/bff/internal/database"
	"github.com/PageWrightCloud/pagewright/bff/internal/middleware"
	"github.com/PageWrightCloud/pagewright/bff/internal/types"
	"github.com/gorilla/mux"
)

type VersionsHandler struct {
	db              *database.DB
	storageClient   *clients.StorageClient
	servingClient   *clients.ServingClient
	defaultPageSize int
}

func NewVersionsHandler(db *database.DB, storageClient *clients.StorageClient, servingClient *clients.ServingClient, defaultPageSize int) *VersionsHandler {
	return &VersionsHandler{
		db:              db,
		storageClient:   storageClient,
		servingClient:   servingClient,
		defaultPageSize: defaultPageSize,
	}
}

// ListVersions lists all versions for a site (from storage service)
func (h *VersionsHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	site, err := h.db.GetSiteByFQDN(fqdn)
	if err != nil || site == nil {
		respondError(w, http.StatusNotFound, "site not found")
		return
	}

	if site.UserID != user.UserID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Fetch versions from storage service
	versions, err := h.storageClient.ListVersions(site.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}

	// Apply pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = h.defaultPageSize
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(versions) {
		start = len(versions)
	}
	if end > len(versions) {
		end = len(versions)
	}

	paginatedVersions := versions[start:end]
	totalPages := (len(versions) + pageSize - 1) / pageSize

	respondJSON(w, types.PaginatedResponse{
		Data:       paginatedVersions,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: len(versions),
		TotalPages: totalPages,
	})
}

// DeployVersion deploys a version to live or preview
func (h *VersionsHandler) DeployVersion(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]
	versionID := vars["version_id"]

	var req types.DeployVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Target != "live" && req.Target != "preview" {
		respondError(w, http.StatusBadRequest, "target must be 'live' or 'preview'")
		return
	}

	site, err := h.db.GetSiteByFQDN(fqdn)
	if err != nil || site == nil {
		respondError(w, http.StatusNotFound, "site not found")
		return
	}

	if site.UserID != user.UserID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Deploy artifact to serving infrastructure
	if err := h.servingClient.DeployArtifact(fqdn, site.ID, versionID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to deploy artifact")
		return
	}

	// Activate the version
	if req.Target == "live" {
		if err := h.servingClient.ActivateVersion(fqdn, versionID); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to activate version")
			return
		}
		// Update database
		h.db.UpdateSiteVersions(fqdn, &versionID, nil)
	} else {
		if err := h.servingClient.ActivatePreview(fqdn, versionID); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to activate preview")
			return
		}
		// Update database
		h.db.UpdateSiteVersions(fqdn, nil, &versionID)
	}

	respondJSON(w, map[string]string{
		"status":     "deployed",
		"target":     req.Target,
		"version_id": versionID,
	})
}

// DeleteVersion deletes a version
func (h *VersionsHandler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]
	versionID := vars["version_id"]

	site, err := h.db.GetSiteByFQDN(fqdn)
	if err != nil || site == nil {
		respondError(w, http.StatusNotFound, "site not found")
		return
	}

	if site.UserID != user.UserID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Check if version is currently live or preview
	if (site.LiveVersionID != nil && *site.LiveVersionID == versionID) ||
		(site.PreviewVersionID != nil && *site.PreviewVersionID == versionID) {
		respondError(w, http.StatusBadRequest, "cannot delete currently deployed version")
		return
	}

	// Delete from storage
	if err := h.storageClient.DeleteVersion(site.ID, versionID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete version")
		return
	}

	// Delete from database
	h.db.DeleteVersion(site.ID, versionID)

	w.WriteHeader(http.StatusNoContent)
}

// DownloadVersion downloads a version artifact
func (h *VersionsHandler) DownloadVersion(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]
	versionID := vars["version_id"]

	site, err := h.db.GetSiteByFQDN(fqdn)
	if err != nil || site == nil {
		respondError(w, http.StatusNotFound, "site not found")
		return
	}

	if site.UserID != user.UserID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Fetch artifact from storage
	data, err := h.storageClient.FetchArtifact(site.ID, versionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch artifact")
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename="+versionID+".tar.gz")
	w.Write(data)
}
