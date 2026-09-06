package handlers

import (
	"net/http"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
)

// Retain endpoint compatibility without exposing unsupported namespace mutations.
type AliasesHandler struct{}

func NewAliasesHandler(_ *database.DB, _ *clients.ServingClient) *AliasesHandler {
	return &AliasesHandler{}
}

func (h *AliasesHandler) ListAliases(w http.ResponseWriter, r *http.Request) {
	mvpUnavailable(w, "Aliases")
}
func (h *AliasesHandler) AddAlias(w http.ResponseWriter, r *http.Request) {
	mvpUnavailable(w, "Aliases")
}
func (h *AliasesHandler) DeleteAlias(w http.ResponseWriter, r *http.Request) {
	mvpUnavailable(w, "Aliases")
}
