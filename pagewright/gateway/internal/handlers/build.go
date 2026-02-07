package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/PageWrightCloud/pagewright/gateway/internal/types"
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
	user, _ := middleware.GetUserFromContext(r)
	vars := mux.Vars(r)
	fqdn := vars["fqdn"]

	var req types.BuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
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
		SiteID:          site.ID,
		BaseBuildID:     baseBuildID,
		RequestedAction: "edit",
		UserText:        instructions,
		Metadata: map[string]string{
			"original_message": originalMessage,
			"fqdn":             site.FQDN,
		},
	}

	jobResp, err := h.managerClient.EnqueueJob(jobReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to enqueue job")
		return
	}

	// Create version record in database
	h.db.CreateVersion(site.ID, jobResp.JobID, "pending")

	respondJSON(w, types.BuildResponse{
		JobID: &jobResp.JobID,
	})
}
