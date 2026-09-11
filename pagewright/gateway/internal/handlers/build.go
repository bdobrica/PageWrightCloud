package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type BuildHandler struct {
	db              buildStore
	llmClient       instructionProvider
	managerClient   *clients.ManagerClient
	storageClient   completedVersionStore
	pilot           pilotAdmission
	pilotLimits     database.PilotLimits
	pilotAIDisabled bool
}

type pilotAdmission interface {
	AdmitPilot(context.Context, string, string, string, string, database.PilotLimits) error
	ReleasePilot(context.Context, string, string, string) error
}

func (h *BuildHandler) SetPilotLimits(store pilotAdmission, limits database.PilotLimits) {
	h.pilot = store
	h.pilotLimits = limits
}

func (h *BuildHandler) SetPilotAIAllowance(cents int) { h.pilotAIDisabled = cents < 100 }

type completedVersionStore interface {
	ListVersionsContext(context.Context, string) ([]clients.StorageVersion, error)
}

type buildStore interface {
	GetSiteByFQDNContext(context.Context, string) (*types.Site, error)
	FindBuildSubmission(context.Context, string, string, string) (*database.BuildSubmission, error)
	ReserveBuildSubmission(context.Context, *database.BuildSubmission) (*database.BuildSubmission, bool, error)
	ClaimBuildDispatch(context.Context, string) (bool, error)
	RecordBuildOutcome(context.Context, string, string, string, string, string, int) error
}

type instructionProvider interface {
	EvaluateRequestContext(context.Context, string) (*clients.EvaluationResponse, error)
	GenerateJobInstructionsContext(context.Context, string, string) (string, error)
}

func NewBuildHandler(db buildStore, llmClient instructionProvider, managerClient *clients.ManagerClient, storageClient completedVersionStore) *BuildHandler {
	return &BuildHandler{
		db:            db,
		llmClient:     llmClient,
		managerClient: managerClient,
		storageClient: storageClient,
	}
}

// conversationStore is a simple in-memory store for conversation context
// In production, use Redis or similar
var conversationStore = make(map[string]conversationContext)
var conversationMu sync.Mutex

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

	// Older JSON clients omitted Content-Type. Explicit non-JSON uploads are never
	// interpreted as text requests, even if their bytes happen to be valid JSON.
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			respondError(w, http.StatusUnsupportedMediaType, "text-only JSON requests are supported; attachments are unavailable")
			return
		}
	}

	var req types.BuildRequest
	if err := boundedJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		respondError(w, http.StatusBadRequest, "message is required")
		return
	}

	site, err := h.db.GetSiteByFQDNContext(r.Context(), fqdn)
	if err != nil || site == nil {
		respondError(w, http.StatusNotFound, "site not found")
		return
	}

	if site.UserID != user.UserID {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}
	if site.InitializationStatus == "pending" {
		respondError(w, http.StatusConflict, "site initialization incomplete; retry site creation first")
		return
	}
	key, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		respondError(w, http.StatusBadRequest, "Idempotency-Key must be a nonzero UUID")
		return
	}
	requestKey := key.String()
	body, _ := json.Marshal(req)
	requestHash := fmt.Sprintf("%x", sha256.Sum256(body))
	// Look up before the provider or conversation cache, so retries survive a
	// gateway restart and do not regenerate a committed prompt or version ID.
	existing, err := h.db.FindBuildSubmission(r.Context(), user.UserID, site.ID, requestKey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to look up build submission")
		return
	}
	if existing != nil {
		if existing.RequestHash != requestHash {
			respondError(w, http.StatusConflict, "Idempotency-Key was already used with different input")
			return
		}
		h.dispatchBuild(w, r, existing)
		return
	}

	// Check if this is a clarification response
	if h.pilot != nil {
		if h.pilotAIDisabled {
			w.Header().Set("Retry-After", "60")
			respondError(w, http.StatusTooManyRequests, "Paid AI is disabled. Contact the operator to configure an allowance.")
			return
		}
		if len(req.Message) > 16000 {
			respondError(w, http.StatusBadRequest, "build request exceeds 16000 bytes")
			return
		}
		if err := h.pilot.AdmitPilot(r.Context(), user.UserID, site.ID, requestKey, requestHash, h.pilotLimits); err != nil {
			if err == database.ErrPilotLimit {
				w.Header().Set("Retry-After", "60")
				respondError(w, http.StatusTooManyRequests, "Pilot build quota or concurrency limit reached. Wait for active work or contact the operator.")
			} else if err == database.ErrSubmissionConflict {
				respondError(w, http.StatusConflict, "Idempotency-Key was already used with different input")
			} else {
				respondError(w, http.StatusServiceUnavailable, "Usage checks unavailable; no new build was started")
			}
			return
		}
		defer func() {
			ctx, cancel := database.PilotCleanupContext()
			defer cancel()
			_ = h.pilot.ReleasePilot(ctx, user.UserID, site.ID, requestKey)
		}()
	}
	if req.ConversationID != nil {
		h.handleClarification(w, r, site, req, requestKey, requestHash)
		return
	}

	// Initial request - evaluate if clear
	evaluation, err := h.llmClient.EvaluateRequestContext(r.Context(), req.Message)
	if err != nil {
		if errors.Is(err, clients.ErrAIAllowance) {
			w.Header().Set("Retry-After", "60")
			respondError(w, http.StatusTooManyRequests, "AI allowance is exhausted, disabled or busy. Contact the operator before retrying.")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to evaluate request")
		return
	}

	if !evaluation.IsClear {
		// Need clarification - generate conversation ID
		conversationID := uuid.New().String()
		conversationMu.Lock()
		conversationStore[conversationID] = conversationContext{
			UserID:          user.UserID,
			SiteID:          site.ID,
			OriginalMessage: req.Message,
		}
		conversationMu.Unlock()

		respondJSON(w, types.BuildResponse{
			Question:       &evaluation.Question,
			ConversationID: &conversationID,
		})
		return
	}

	// Request is clear - generate instructions and enqueue job
	h.enqueueJob(w, r, site, req.Message, "", requestKey, requestHash)
}

func (h *BuildHandler) handleClarification(w http.ResponseWriter, r *http.Request, site *types.Site, req types.BuildRequest, requestKey, requestHash string) {
	// Get conversation context
	conversationMu.Lock()
	ctx, exists := conversationStore[*req.ConversationID]
	conversationMu.Unlock()
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

	// Generate instructions with clarification and enqueue job
	// Keep the context on failure. Durable submissions are checked first on retry;
	// lifecycle/expiry for this pre-submission cache remains later browser work.
	h.enqueueJob(w, r, site, ctx.OriginalMessage, req.Message, requestKey, requestHash)
}

func (h *BuildHandler) enqueueJob(w http.ResponseWriter, r *http.Request, site *types.Site, originalMessage, clarification, requestKey, requestHash string) {
	// Generate job instructions using LLM
	instructions, err := h.llmClient.GenerateJobInstructionsContext(r.Context(), originalMessage, clarification)
	if err != nil {
		if errors.Is(err, clients.ErrAIAllowance) {
			w.Header().Set("Retry-After", "60")
			respondError(w, http.StatusTooManyRequests, "AI allowance is exhausted, disabled or busy. Contact the operator before retrying.")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to generate instructions")
		return
	}

	// Storage's manifest-last list is authoritative for completed artifacts;
	// gateway version rows can still say pending after a worker completes.
	baseBuildID, err := h.selectBuildSource(r.Context(), site)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to determine latest completed build; no job was dispatched")
		return
	}

	// Persist independent execution/artifact IDs and the version row together,
	// before any request can reach the manager.
	submission, _, err := h.db.ReserveBuildSubmission(r.Context(), &database.BuildSubmission{
		JobID:         uuid.NewString(),
		TargetVersion: uuid.NewString(),
		SiteID:        site.ID,
		OwnerID:       site.UserID, // Derived from the authenticated, owner-checked site.
		SourceVersion: baseBuildID,
		Prompt:        instructions,
		RequestKey:    requestKey,
		RequestHash:   requestHash,
	})
	if err != nil {
		h.reservationError(w, err)
		return
	}
	h.dispatchBuild(w, r, submission)
}
