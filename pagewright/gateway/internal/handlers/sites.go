package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/gorilla/mux"
)

type SitesHandler struct {
	db              *database.DB
	servingClient   *clients.ServingClient
	defaultPageSize int
}

func NewSitesHandler(db *database.DB, servingClient *clients.ServingClient, defaultPageSize int) *SitesHandler {
	return &SitesHandler{
		db:              db,
		servingClient:   servingClient,
		defaultPageSize: defaultPageSize,
	}
}

// CreateSite creates a new site
func (h *SitesHandler) CreateSite(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)

	var req types.CreateSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.FQDN == "" || req.TemplateID == "" {
		respondError(w, http.StatusBadRequest, "fqdn and template_id are required")
		return
	}

	// Create site in database
	site, err := h.db.CreateSite(user.UserID, req.FQDN, req.TemplateID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create site")
		return
	}

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, site)
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

	respondJSON(w, types.PaginatedResponse{
		Data:       sites,
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

	respondJSON(w, site)
}

// DeleteSite deletes a site
func (h *SitesHandler) DeleteSite(w http.ResponseWriter, r *http.Request) {
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

	if site.UserID != user.UserID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Delete from serving infrastructure
	if err := h.servingClient.DeleteSite(fqdn); err != nil {
		// Log error but continue with database deletion
		// In production, consider using a background job for cleanup
	}

	// Delete from database
	if err := h.db.DeleteSite(fqdn); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete site")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
