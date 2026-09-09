package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/gorilla/mux"
)

type VersionsHandler struct {
	db              *database.DB
	storageClient   *clients.StorageClient
	servingClient   *clients.ServingClient
	defaultPageSize int
	hostingAddress
}

func NewVersionsHandler(db *database.DB, storageClient *clients.StorageClient, servingClient *clients.ServingClient, defaultPageSize int) *VersionsHandler {
	if defaultPageSize < 1 || defaultPageSize > 100 {
		defaultPageSize = 25
	}
	return &VersionsHandler{
		db:              db,
		storageClient:   storageClient,
		servingClient:   servingClient,
		defaultPageSize: defaultPageSize,
		hostingAddress:  hostingAddress{scheme: "http", port: "8084"},
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
	stored, err := h.storageClient.ListVersions(site.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}
	versions, err := normalizeVersions(site.ID, stored)
	if err != nil {
		respondError(w, http.StatusBadGateway, "invalid storage version response")
		return
	}

	// Apply pagination
	unconfirmed, err := h.db.UnconfirmedBuilds(r.Context(), site.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to reconcile versions")
		return
	}
	confirmed := versions[:0]
	for _, version := range versions {
		if !unconfirmed[version.BuildID] {
			confirmed = append(confirmed, version)
		}
	}
	versions = confirmed

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = h.defaultPageSize
	}

	// Compare before multiplying so arbitrarily large page values cannot overflow.
	start := len(versions)
	if page-1 <= len(versions)/pageSize {
		start = (page - 1) * pageSize
	}
	end := start + min(pageSize, len(versions)-start)

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

// VersionSummary is an artifact identity, not a database version-row identity.
type VersionSummary struct {
	ID        string    `json:"id"`
	SiteID    string    `json:"site_id"`
	BuildID   string    `json:"build_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

var versionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)

func normalizeVersions(siteID string, stored []clients.StorageVersion) ([]VersionSummary, error) {
	result := make([]VersionSummary, 0, len(stored))
	seen := map[string]bool{}
	for _, v := range stored {
		if !versionIDPattern.MatchString(v.BuildID) || v.Timestamp.IsZero() || v.Status != "completed" || seen[v.BuildID] {
			return nil, fmt.Errorf("invalid committed version")
		}
		seen[v.BuildID] = true
		result = append(result, VersionSummary{v.BuildID, siteID, v.BuildID, "completed", v.Timestamp.UTC()})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].BuildID < result[j].BuildID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

// DeployVersion deploys a version to live or preview
func (h *VersionsHandler) DeployVersion(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]
	versionID := vars["version_id"]
	if !versionIDPattern.MatchString(versionID) {
		respondError(w, 400, "invalid version identifier")
		return
	}

	var req types.DeployVersionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF {
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
	publicURL, err := h.deploymentURL(fqdn, req.Target)
	if errors.Is(err, ErrTLSProvisioning) {
		w.Header().Set("Retry-After", "60")
		respondError(w, http.StatusServiceUnavailable, ErrTLSProvisioning.Error())
		return
	}
	if err != nil {
		respondError(w, 500, "invalid public hosting configuration")
		return
	}
	d, err := h.db.ReserveDeployment(r.Context(), site.ID, versionID, req.Target)
	if errors.Is(err, database.ErrDeploymentBusy) {
		respondError(w, 409, "another deployment is being reconciled; retry its selected version first")
		return
	}
	if err != nil {
		respondError(w, 503, "cannot persist deployment intent")
		return
	}
	if err = h.reconcileDeployment(r.Context(), d); err != nil {
		respondError(w, 500, "deployment not confirmed; recovery retains the selected target; retry the same version")
		return
	}

	respondJSON(w, map[string]string{
		"status":     "deployed",
		"target":     req.Target,
		"version_id": versionID,
		"url":        publicURL,
	})
}

// DeleteVersion is deliberately disabled until coordinated active-version
// protection exists. Do not call storage or delete database rows.
func (h *VersionsHandler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusNotImplemented, "version deletion is disabled in this MVP")
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
	reader, err := h.storageClient.OpenArtifact(site.ID, versionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch artifact")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename="+versionID+".tar.gz")
	if _, err := io.Copy(w, reader); err != nil {
		// Do not terminate an incomplete chunked response as if it were complete.
		panic(http.ErrAbortHandler)
	}
}
