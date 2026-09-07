package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/gorilla/mux"
)

type SitesHandler struct {
	db               *database.DB
	servingClient    *clients.ServingClient
	storageClient    *clients.StorageClient
	defaultPageSize  int
	siteDomain       string
	RegistrationOpen bool
	hostingAddress
}

func NewSitesHandler(db *database.DB, servingClient *clients.ServingClient, storageClient *clients.StorageClient, defaultPageSize int) *SitesHandler {
	return &SitesHandler{
		db:              db,
		servingClient:   servingClient,
		storageClient:   storageClient,
		defaultPageSize: defaultPageSize,
		siteDomain:      "pagewright.dev",
		hostingAddress:  hostingAddress{"http", "8084"},
	}
}

type hostedSite struct {
	*types.Site
	LiveURL    string `json:"live_url"`
	PreviewURL string `json:"preview_url"`
}

func (h *SitesHandler) publicSite(site *types.Site) hostedSite {
	// Invalid configuration/legacy domains expose no clickable destination.
	live, _ := h.deploymentURL(site.FQDN, "live")
	preview, _ := h.deploymentURL(site.FQDN, "preview")
	return hostedSite{site, live, preview}
}

// CreateSite creates a new site
func (h *SitesHandler) CreateSite(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)

	var req types.CreateSiteRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(new(interface{})) != io.EOF {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.FQDN = strings.ToLower(strings.TrimSpace(req.FQDN))
	if !h.supportedSiteName(req.FQDN) || (req.TemplateID != "starter" && req.TemplateID != "template-1") {
		respondError(w, http.StatusBadRequest, "a single platform subdomain and starter template are required")
		return
	}

	site, record, err := h.db.ReserveSiteBootstrap(r.Context(), user.UserID, req.FQDN)
	if err != nil {
		if errors.Is(err, database.ErrSiteConflict) {
			respondError(w, http.StatusConflict, "domain is already reserved")
			return
		}
		respondError(w, http.StatusServiceUnavailable, "failed to reserve site; retry the same domain")
		return
	}
	if site.InitializationStatus != "ready" {
		if err := h.storageClient.InitializeSource(site.ID, record.VersionID, record.Archive, record.ExecutionLog, record.Manifest); err != nil {
			if errors.Is(err, clients.ErrBootstrapConflict) {
				respondError(w, http.StatusConflict, "initial source conflicts with stored data; no files were replaced; operator repair is required")
				return
			}
			respondError(w, http.StatusServiceUnavailable, "site initialization incomplete; retry creation with the same domain and starter template")
			return
		}
		if err := h.db.CompleteSiteBootstrap(r.Context(), site.ID); err != nil {
			respondError(w, http.StatusServiceUnavailable, "site initialization confirmation failed; retry the same domain")
			return
		}
		site, err = h.db.GetSiteByFQDN(req.FQDN)
		if err != nil || site == nil || site.ID != record.SiteID || site.UserID != user.UserID {
			respondError(w, http.StatusServiceUnavailable, "site initialization confirmed but response failed; retry the same domain")
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, h.publicSite(site))
}

var domainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func validSiteFQDN(fqdn string) bool {
	if len(fqdn) > 245 || strings.HasPrefix(fqdn, "preview.") || !strings.Contains(fqdn, ".") {
		return false
	}
	for _, label := range strings.Split(fqdn, ".") {
		if !domainLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// ListSites lists all sites for the authenticated user
func (h *SitesHandler) ListSites(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = h.defaultPageSize
	}

	offset := (page - 1) * pageSize

	sites, totalCount, err := h.db.GetUserSites(user.UserID, pageSize, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get sites")
		return
	}

	totalPages := (totalCount + pageSize - 1) / pageSize
	publicSites := make([]hostedSite, 0, len(sites))
	for i := range sites {
		publicSites = append(publicSites, h.publicSite(&sites[i]))
	}

	respondJSON(w, types.PaginatedResponse{
		Data:       publicSites,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
	})
}

// GetSite retrieves a single site
func (h *SitesHandler) GetSite(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	site, err := h.db.GetSiteByFQDN(fqdn)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get site")
		return
	}

	if site == nil {
		respondError(w, http.StatusNotFound, "site not found")
		return
	}

	// Check ownership
	if site.UserID != user.UserID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	respondJSON(w, h.publicSite(site))
}

// DeleteSite is disabled until DB, storage and serving deletion is coordinated.
// The lower-level deployment-record and active-artifact guards remain intact.
func (h *SitesHandler) DeleteSite(w http.ResponseWriter, r *http.Request) {
	mvpUnavailable(w, "Site deletion")
}

// EnableSite enables a site
func (h *SitesHandler) EnableSite(w http.ResponseWriter, r *http.Request) {
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

	// Enable in serving infrastructure
	if site.InitializationStatus == "pending" {
		respondError(w, http.StatusConflict, "site initialization incomplete; retry site creation first")
		return
	}
	if err := h.servingClient.EnableSite(fqdn); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to enable site")
		return
	}

	// Update database
	if err := h.db.UpdateSiteEnabled(fqdn, true); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update site")
		return
	}

	respondJSON(w, map[string]string{"status": "enabled"})
}

// DisableSite disables a site
func (h *SitesHandler) DisableSite(w http.ResponseWriter, r *http.Request) {
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

	// Disable in serving infrastructure
	if err := h.servingClient.DisableSite(fqdn); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to disable site")
		return
	}

	// Update database
	if err := h.db.UpdateSiteEnabled(fqdn, false); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update site")
		return
	}

	respondJSON(w, map[string]string{"status": "disabled"})
}
