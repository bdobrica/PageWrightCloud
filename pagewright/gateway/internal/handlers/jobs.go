package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/gorilla/mux"
)

type jobReadStore interface {
	ReadBuilds(context.Context, string, string, string, int, int) ([]database.BuildSubmission, int, error)
}

// PublicBuild deliberately omits generated instructions, request identities,
// private logs/results and worker/manager credentials.
type PublicBuild struct {
	JobID         string    `json:"job_id"`
	SiteID        string    `json:"site_id"`
	SourceVersion string    `json:"source_version"`
	TargetVersion string    `json:"target_version"`
	Status        string    `json:"status"`
	DispatchState string    `json:"dispatch_state"`
	ErrorCode     string    `json:"error_code,omitempty"`
	RecoveryError string    `json:"recovery_error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Jobs serves both the history page and one job. Background recovery is the
// only reconciliation writer; reads work even when manager/storage are down.
func (h *BuildHandler) Jobs(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil {
		respondError(w, 401, "authentication required")
		return
	}
	site, err := h.db.GetSiteByFQDNContext(r.Context(), mux.Vars(r)["fqdn"])
	if err != nil || site == nil {
		respondError(w, 404, "site not found")
		return
	}
	if site.UserID != user.UserID {
		respondError(w, 403, "access denied")
		return
	}
	page, size := 1, 25
	for key, dest := range map[string]*int{"page": &page, "page_size": &size} {
		if raw := r.URL.Query().Get(key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || (key == "page_size" && n > 100) || n > 2147483647 {
				respondError(w, 400, "invalid pagination")
				return
			}
			*dest = n
		}
	}
	job := mux.Vars(r)["job_id"]
	if job != "" {
		page, size = 1, 1
	}
	store, ok := h.db.(jobReadStore)
	if !ok {
		respondError(w, 500, "job history unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, total, err := store.ReadBuilds(ctx, user.UserID, site.ID, job, size, (page-1)*size)
	if err != nil {
		respondError(w, 500, "job history unavailable")
		return
	}
	data := make([]PublicBuild, 0, len(rows))
	for _, s := range rows {
		data = append(data, PublicBuild{JobID: s.JobID, SiteID: s.SiteID, SourceVersion: s.SourceVersion, TargetVersion: s.TargetVersion, Status: s.Status, DispatchState: s.DispatchState, ErrorCode: publicFailureCode(s.ErrorCode), RecoveryError: publicRecoveryCode(s.RecoveryError), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt})
	}
	w.Header().Set("Cache-Control", "no-store")
	if job != "" {
		if len(data) == 0 {
			respondError(w, 404, "job not found")
			return
		}
		respondJSON(w, data[0])
		return
	}
	respondJSON(w, map[string]interface{}{"data": data, "page": page, "page_size": size, "total_count": total, "total_pages": (total + size - 1) / size})
}

func publicFailureCode(code string) string {
	switch code {
	case "", "job_busy", "spawn_failed", "job_conflict", "manager_unavailable", "worker_exit", "artifact_incomplete":
		return code
	default:
		return "build_failed"
	}
}

func publicRecoveryCode(code string) string {
	switch code {
	case "", "manager_evidence_unavailable", "manager_evidence_missing_operator_required", "manager_evidence_conflict_operator_required":
		return code
	default:
		return "recovery_attention_required"
	}
}
