package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/PageWrightCloud/pagewright/bff/internal/clients"
	"github.com/PageWrightCloud/pagewright/bff/internal/database"
	"github.com/PageWrightCloud/pagewright/bff/internal/middleware"
	"github.com/PageWrightCloud/pagewright/bff/internal/types"
	"github.com/gorilla/mux"
)

type AliasesHandler struct {
	db            *database.DB
	servingClient *clients.ServingClient
}

func NewAliasesHandler(db *database.DB, servingClient *clients.ServingClient) *AliasesHandler {
	return &AliasesHandler{
		db:            db,
		servingClient: servingClient,
	}
}

// ListAliases lists all aliases for a site
func (h *AliasesHandler) ListAliases(w http.ResponseWriter, r *http.Request) {
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

	aliases, err := h.db.GetSiteAliases(site.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get aliases")
		return
	}

	respondJSON(w, aliases)
}

// AddAlias adds a new alias to a site
func (h *AliasesHandler) AddAlias(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	var req types.AddAliasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Alias == "" {
		respondError(w, http.StatusBadRequest, "alias is required")
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

	// Create alias in database
	alias, err := h.db.CreateAlias(site.ID, req.Alias)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create alias")
		return
	}

	// Update serving infrastructure
	aliases, _ := h.db.GetSiteAliases(site.ID)
	aliasNames := make([]string, len(aliases))
	for i, a := range aliases {
		aliasNames[i] = a.Alias
	}

	if err := h.servingClient.UpdateAliases(fqdn, aliasNames); err != nil {
		// Rollback database change
		h.db.DeleteAlias(site.ID, req.Alias)
		respondError(w, http.StatusInternalServerError, "failed to update serving aliases")
		return
	}

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, alias)
}

// DeleteAlias removes an alias from a site
func (h *AliasesHandler) DeleteAlias(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]
	alias := vars["alias"]

	site, err := h.db.GetSiteByFQDN(fqdn)
	if err != nil || site == nil {
		respondError(w, http.StatusNotFound, "site not found")
		return
	}

	if site.UserID != user.UserID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Delete from database
	if err := h.db.DeleteAlias(site.ID, alias); err != nil {
		respondError(w, http.StatusNotFound, "alias not found")
		return
	}

	// Update serving infrastructure
	aliases, _ := h.db.GetSiteAliases(site.ID)
	aliasNames := make([]string, len(aliases))
	for i, a := range aliases {
		aliasNames[i] = a.Alias
	}

	h.servingClient.UpdateAliases(fqdn, aliasNames)

	w.WriteHeader(http.StatusNoContent)
}
