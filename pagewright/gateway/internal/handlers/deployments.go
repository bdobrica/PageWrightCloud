package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/gorilla/mux"
)

func (h *VersionsHandler) DeploymentStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, _ := middleware.GetUserFromContext(r)
	site, err := h.db.GetSiteByFQDN(mux.Vars(r)["fqdn"])
	if err != nil || site == nil {
		respondError(w, 404, "site not found")
		return
	}
	if site.UserID != user.UserID {
		respondError(w, 403, "access denied")
		return
	}
	d, err := h.db.GetDeployment(r.Context(), site.ID)
	if err != nil {
		respondError(w, 503, "deployment state unavailable")
		return
	}
	respondJSON(w, d)
}

func (h *VersionsHandler) reconcileDeployment(ctx context.Context, d *database.Deployment) error {
	h.db.TouchDeployment(ctx, d)
	status, err := h.servingClient.ApplyDeployment(ctx, d)
	if err != nil {
		return err
	}
	if err = h.db.FinishDeployment(ctx, d, status); err != nil {
		return err
	}
	if status == "failed" {
		return errors.New("deployment failed before activation; retry is safe")
	}
	return nil
}

// Replay only the persisted identity. Serving fences and deduplicates requests,
// including requests delayed beyond gateway timeouts/restarts.
func (h *VersionsHandler) RunDeploymentRecovery(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		ds, err := h.db.PendingDeployments(ctx)
		if err == nil {
			for i := range ds {
				if ctx.Err() != nil {
					return
				}
				attempt, cancel := context.WithTimeout(ctx, 30*time.Second)
				_ = h.reconcileDeployment(attempt, &ds[i])
				cancel()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
