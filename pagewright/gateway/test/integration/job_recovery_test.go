//go:build integration

package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
)

// Scope shared service fixtures to this test's job. Real concurrent batch claims
// and cooldowns are covered by the isolated-schema database tests.
type recoveryScope struct {
	*database.DB
	id string
}

func (s recoveryScope) RecoveryBatch(ctx context.Context) ([]database.BuildSubmission, error) {
	var rows []database.BuildSubmission
	err := s.SelectContext(ctx, &rows, `SELECT * FROM build_submissions WHERE job_id=$1 AND (dispatch_state IN ('ready','dispatching') OR (dispatch_state='accepted' AND status IN ('pending','running')))`, s.id)
	return rows, err
}

func TestGatewayRecoveryAcrossHandlerRestart(t *testing.T) {
	for _, beforeClaim := range []bool{true, false} {
		t.Run(map[bool]string{true: "ready", false: "claimed_before_send"}[beforeClaim], func(t *testing.T) {
			site, _ := submissionSite(t)
			ctx := context.Background()
			key := uuid.NewString()
			proposal := &database.BuildSubmission{JobID: uuid.NewString(), SiteID: site.ID, OwnerID: site.UserID, SourceVersion: "initial", TargetVersion: uuid.NewString(), Prompt: "Change the title", RequestKey: key, RequestHash: strings.Repeat("a", 64)}
			s, _, err := testDB.ReserveBuildSubmission(ctx, proposal)
			if err != nil {
				t.Fatal(err)
			}
			if !beforeClaim {
				if _, err := testDB.ClaimBuildDispatch(ctx, s.JobID); err != nil {
					t.Fatal(err)
				}
			}
			manager := &submissionManager{t: t, key: key}
			server := httptest.NewServer(manager)
			defer server.Close()
			store := recoveryScope{testDB, s.JobID}
			recover := func() error {
				return handlers.NewBuildHandler(store, &submissionProvider{}, clients.NewManagerClient(server.URL), emptyCompletedVersions{}).RecoverJobs(ctx)
			}
			err = recover()
			if beforeClaim {
				if err != nil || manager.posts.Load() != 1 {
					t.Fatalf("ready recovery %v posts=%d", err, manager.posts.Load())
				}
			} else {
				if err == nil || manager.posts.Load() != 0 {
					t.Fatal("missing evidence was redispatched")
				}
				row, _ := testDB.FindBuildSubmission(ctx, s.OwnerID, s.SiteID, key)
				if row.RecoveryError != "manager_evidence_missing_operator_required" || row.DispatchState != "dispatching" {
					t.Fatalf("missing evidence not retained: %+v", row)
				}
			}
			manager.mu.Lock()
			manager.job = &types.Job{JobAccepted: types.JobAccepted{JobID: s.JobID, SiteID: s.SiteID, OwnerID: s.OwnerID, SourceVersion: s.SourceVersion, TargetVersion: s.TargetVersion, Status: types.JobStatusCompleted}, Prompt: s.Prompt, ManifestPath: "/sites/" + s.SiteID + "/artifacts/" + s.TargetVersion + "/manifest", Result: "recovered", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			manager.mu.Unlock()
			if err := recover(); err != nil {
				t.Fatal(err)
			}
			got, err := testDB.FindBuildSubmission(ctx, s.OwnerID, s.SiteID, key)
			if err != nil || got.Status != "completed" || got.DispatchState != "accepted" || got.RecoveryError != "" {
				t.Fatalf("completion not recovered %+v %v", got, err)
			}
			if err := recover(); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := testDB.Get(&count, `SELECT count(*) FROM job_history WHERE job_id=$1 AND status='completed'`, s.JobID); err != nil || count != 1 {
				t.Fatal("terminal history duplicated")
			}
			want := int32(0)
			if beforeClaim {
				want = 1
			}
			if manager.posts.Load() != want {
				t.Fatal("restart replayed execution")
			}
		})
	}
}
