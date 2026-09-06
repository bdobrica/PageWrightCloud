//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/google/uuid"
)

func deploymentNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func deploymentCheck(t *testing.T, ok bool) {
	t.Helper()
	if !ok {
		t.Fatal("deployment invariant failed")
	}
}

func TestDeploymentRecoveryAndConcurrentReservations(t *testing.T) {
	ctx := context.Background()
	user, err := testDB.CreateUser(uuid.NewString()+"@deployment.test", "hash", nil, nil)
	deploymentNoError(t, err)
	site, err := testDB.CreateSite(user.ID, uuid.NewString()+".example.test", "starter")
	deploymentNoError(t, err)
	live := "old-live"
	deploymentNoError(t, testDB.UpdateSiteVersions(site.FQDN, &live, nil))
	d, err := testDB.ReserveDeployment(ctx, site.ID, "draft", "preview")
	deploymentNoError(t, err)
	retry, err := testDB.ReserveDeployment(ctx, site.ID, "draft", "preview")
	deploymentNoError(t, err)
	deploymentCheck(t, d.Sequence == retry.Sequence)
	_, err = testDB.ReserveDeployment(ctx, site.ID, "other", "live")
	deploymentCheck(t, errors.Is(err, database.ErrDeploymentBusy))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := testDB.ReserveDeployment(ctx, site.ID, "other", "live")
			if err != database.ErrDeploymentBusy {
				t.Errorf("concurrent reservation: %v", err)
			}
		}()
	}
	wg.Wait()
	deleted := false
	deploymentCheck(t, errors.Is(testDB.DeleteUndeployedSite(ctx, site.ID, func() error { deleted = true; return nil }), database.ErrDeploymentBusy))
	deploymentCheck(t, !deleted)

	// Serving already switched, but gateway response/DB confirmation was lost.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request database.Deployment
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad", 400)
			return
		}
		if request.Sequence != d.Sequence || request.Version != d.Version || request.Target != d.Target {
			t.Error("recovery changed intent")
		}
		request.Status = "completed"
		json.NewEncoder(w).Encode(request)
	}))
	defer upstream.Close()
	// A real transactional DB failure must retain pending intent and old pointers.
	_, err = testDB.Exec(`CREATE FUNCTION test_deployment_pointer_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected DB failure'; END $$`)
	deploymentNoError(t, err)
	_, err = testDB.Exec(`CREATE TRIGGER test_deployment_pointer_failure BEFORE UPDATE OF preview_version_id ON sites FOR EACH ROW EXECUTE FUNCTION test_deployment_pointer_failure()`)
	deploymentNoError(t, err)
	t.Cleanup(func() {
		testDB.Exec(`DROP TRIGGER IF EXISTS test_deployment_pointer_failure ON sites`)
		testDB.Exec(`DROP FUNCTION IF EXISTS test_deployment_pointer_failure()`)
	})
	deploymentCheck(t, testDB.FinishDeployment(ctx, d, "completed") != nil)
	saved, err := testDB.GetDeployment(ctx, site.ID)
	deploymentNoError(t, err)
	deploymentCheck(t, saved.Status == "pending")
	s, err := testDB.GetSiteByFQDN(site.FQDN)
	deploymentNoError(t, err)
	deploymentCheck(t, s.PreviewVersionID == nil)
	deploymentCheck(t, live == *s.LiveVersionID)
	_, err = testDB.Exec(`DROP TRIGGER test_deployment_pointer_failure ON sites`)
	deploymentNoError(t, err)
	// A fresh handler models a gateway restart; reconciliation replays exact intent.
	h := handlers.NewVersionsHandler(testDB, nil, clients.NewServingClient(upstream.URL), 25)
	recovery, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); h.RunDeploymentRecovery(recovery) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s, err := testDB.GetSiteByFQDN(site.FQDN)
		if err == nil && s.PreviewVersionID != nil && *s.PreviewVersionID == "draft" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovery timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	s, err = testDB.GetSiteByFQDN(site.FQDN)
	deploymentNoError(t, err)
	deploymentCheck(t, live == *s.LiveVersionID)
	newer, err := testDB.ReserveDeployment(ctx, site.ID, "new-live", "live")
	deploymentNoError(t, err)
	deploymentCheck(t, newer.Sequence > d.Sequence)
	deploymentCheck(t, errors.Is(testDB.FinishDeployment(ctx, d, "completed"), database.ErrDeploymentBusy))
	deploymentNoError(t, testDB.FinishDeployment(ctx, newer, "completed"))
	s, err = testDB.GetSiteByFQDN(site.FQDN)
	deploymentNoError(t, err)
	deploymentCheck(t, "draft" == *s.PreviewVersionID)
	deploymentCheck(t, "new-live" == *s.LiveVersionID)
}
