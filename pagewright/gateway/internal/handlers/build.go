package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type BuildHandler struct {
	db            *database.DB
	llmClient     *clients.LLMClient
	managerClient *clients.ManagerClient
}

func NewBuildHandler(db *database.DB, llmClient *clients.LLMClient, managerClient *clients.ManagerClient) *BuildHandler {
	return &BuildHandler{
		db:            db,
		llmClient:     llmClient,
		managerClient: managerClient,
	}
}

// conversationStore is a simple in-memory store for conversation context
// In production, use Redis or similar
var conversationStore = make(map[string]conversationContext)

type conversationContext struct {
	UserID          string
	SiteID          string
	OriginalMessage string
}

// Build handles build requests with OpenAI clarification loop
func (h *BuildHandler) Build(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	var req types.BuildRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		respondError(w, http.StatusBadRequest, "expected one JSON request")
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		respondError(w, http.StatusBadRequest, "message is required")
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

	// Check if this is a clarification response
	if req.ConversationID != nil {
		h.handleClarification(w, r, site, req)
		return
	}

	// Initial request - evaluate if clear
	evaluation, err := h.llmClient.EvaluateRequest(req.Message)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to evaluate request")
		return
	}

	if !evaluation.IsClear {
		// Need clarification - generate conversation ID
		conversationID := uuid.New().String()
		conversationStore[conversationID] = conversationContext{
			UserID:          user.UserID,
			SiteID:          site.ID,
			OriginalMessage: req.Message,
		}

		respondJSON(w, types.BuildResponse{
			Question:       &evaluation.Question,
			ConversationID: &conversationID,
		})
		return
	}

	// Request is clear - generate instructions and enqueue job
	h.enqueueJob(w, site, req.Message, "")
}

func (h *BuildHandler) handleClarification(w http.ResponseWriter, r *http.Request, site *types.Site, req types.BuildRequest) {
	// Get conversation context
	ctx, exists := conversationStore[*req.ConversationID]
	if !exists {
		respondError(w, http.StatusBadRequest, "invalid conversation_id")
		return
	}

	// Verify ownership
	user, _ := middleware.GetUserFromContext(r)
	if ctx.UserID != user.UserID || ctx.SiteID != site.ID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Clean up conversation
	delete(conversationStore, *req.ConversationID)

	// Generate instructions with clarification and enqueue job
	h.enqueueJob(w, site, ctx.OriginalMessage, req.Message)
}

func (h *BuildHandler) enqueueJob(w http.ResponseWriter, site *types.Site, originalMessage, clarification string) {
	// Generate job instructions using LLM
	instructions, err := h.llmClient.GenerateJobInstructions(originalMessage, clarification)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate instructions")
		return
	}

	// Get current live version as base
	baseBuildID := "initial"
	if site.LiveVersionID != nil {
		baseBuildID = *site.LiveVersionID
	}

	// Enqueue job in manager
	jobReq := clients.ManagerJobRequest{
		SiteID:        site.ID,
		OwnerID:       site.UserID, // Derived from the authenticated, owner-checked site.
		SourceVersion: baseBuildID,
		Prompt:        instructions,
	}

	jobResp, err := h.managerClient.EnqueueJob(jobReq)
	if err != nil {
		var managerErr *clients.ManagerError
		if errors.As(err, &managerErr) && managerErr.StatusCode == http.StatusConflict {
			respondError(w, http.StatusConflict, "site already has an active job")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to enqueue job")
		return
	}

	// Legacy persistence is replaced by durable pre-dispatch mapping in M1.2.
	// Do not treat this row's build_id as the manager's canonical target_version.
	h.db.CreateVersion(site.ID, jobResp.JobID, "pending")

	respondJSON(w, types.BuildResponse{
		JobAccepted: &jobResp.JobAccepted,
	})
}
