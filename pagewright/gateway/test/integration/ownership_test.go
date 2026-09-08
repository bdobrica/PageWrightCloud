//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// Real PostgreSQL identities and JWT middleware; upstream traps prove rejection
// precedes storage/serving/manager access, not merely response redaction.
func TestCrossUserEndpointMatrix(t *testing.T) {
	ctx := context.Background()
	var sites []*types.Site
	var tokens []string
	for i := 0; i < 2; i++ {
		user, err := testDB.CreateUser(uuid.NewString()+"@ownership.test", "hash", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		site, err := testDB.CreateSite(user.ID, uuid.NewString()+".ownership.test", "starter")
		if err != nil {
			t.Fatal(err)
		}
		token, err := testJWTManager.GenerateToken(user.ID, user.Email)
		if err != nil {
			t.Fatal(err)
		}
		sites = append(sites, site)
		tokens = append(tokens, token)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "upstream must not receive denied requests", 500)
	}))
	defer upstream.Close()
	s := handlers.NewSitesHandler(testDB, clients.NewServingClient(upstream.URL), clients.NewStorageClient(upstream.URL), 25)
	v := handlers.NewVersionsHandler(testDB, clients.NewStorageClient(upstream.URL), clients.NewServingClient(upstream.URL), 25)
	b := handlers.NewBuildHandler(testDB, nil, clients.NewManagerClient(upstream.URL), nil)
	a := handlers.NewAliasesHandler(testDB, nil)
	type endpoint struct {
		method, suffix, body string
		handler              http.HandlerFunc
		disabled             bool
	}
	endpoints := []endpoint{
		{"GET", "", "", s.GetSite, false},
		{"POST", "/enable", "", s.EnableSite, false}, {"POST", "/disable", "", s.DisableSite, false},
		{"GET", "/jobs", "", b.Jobs, false}, {"GET", "/jobs/{job_id}", "", b.Jobs, false},
		{"POST", "/build", `{"message":"cross-user mutation must fail"}`, b.Build, false},
		{"GET", "/versions", "", v.ListVersions, false}, {"GET", "/deployment", "", v.DeploymentStatus, false},
		{"GET", "/versions/{version_id}/download", "", v.DownloadVersion, false},
		{"POST", "/versions/{version_id}/deploy", `{"target":"preview"}`, v.DeployVersion, false},
		{"POST", "/versions/{version_id}/deploy", `{"target":"live"}`, v.DeployVersion, false},
		{"DELETE", "", "", s.DeleteSite, true}, {"DELETE", "/versions/{version_id}", "", v.DeleteVersion, true},
		{"GET", "/aliases", "", a.ListAliases, true}, {"POST", "/aliases", `{"alias":"foreign.test"}`, a.AddAlias, true},
		{"DELETE", "/aliases/foreign.test", "", a.DeleteAlias, true},
	}
	for owner, site := range sites {
		submission, _, err := testDB.ReserveBuildSubmission(ctx, &database.BuildSubmission{JobID: uuid.NewString(), SiteID: site.ID, OwnerID: site.UserID, SourceVersion: "initial", TargetVersion: uuid.NewString(), Prompt: "PRIVATE ownership prompt", RequestKey: uuid.NewString(), RequestHash: strings.Repeat("a", 64)})
		if err != nil {
			t.Fatal(err)
		}
		live, preview := "private-live", "private-preview"
		if err := testDB.UpdateSiteVersions(site.FQDN, &live, &preview); err != nil {
			t.Fatal(err)
		}
		before, err := testDB.GetSiteByFQDN(site.FQDN)
		if err != nil {
			t.Fatal(err)
		}
		beforeJobs, _, err := testDB.ReadBuilds(ctx, site.UserID, site.ID, "", 25, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, ep := range endpoints {
			t.Run(site.FQDN+ep.method+ep.suffix+ep.body, func(t *testing.T) {
				router := mux.NewRouter()
				router.Handle("/sites/{fqdn}"+ep.suffix, middleware.AuthMiddleware(testJWTManager)(ep.handler)).Methods(ep.method)
				path := "/sites/" + site.FQDN + strings.ReplaceAll(strings.ReplaceAll(ep.suffix, "{job_id}", submission.JobID), "{version_id}", submission.TargetVersion)
				for _, token := range []string{"", tokens[1-owner], tokens[owner]} {
					if token == tokens[owner] && !ep.disabled {
						continue
					}
					req := httptest.NewRequest(ep.method, path, strings.NewReader(ep.body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Idempotency-Key", uuid.NewString())
					if token != "" {
						req.Header.Set("Authorization", "Bearer "+token)
					}
					res := httptest.NewRecorder()
					router.ServeHTTP(res, req)
					want := 403
					if ep.disabled {
						want = 501
					}
					if token == "" {
						want = 401
					}
					if res.Code != want {
						t.Fatalf("got %d, want %d: %s", res.Code, want, res.Body.String())
					}
					for _, secret := range []string{site.ID, submission.JobID, submission.TargetVersion, submission.Prompt, live, preview} {
						if strings.Contains(res.Body.String(), secret) {
							t.Fatal("denied response exposed private state")
						}
					}
				}
			})
		}
		after, err := testDB.GetSiteByFQDN(site.FQDN)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("denied requests changed site: %v", err)
		}
		d, err := testDB.GetDeployment(ctx, site.ID)
		if err != nil || d != nil {
			t.Fatalf("denied deployment persisted intent: %+v %v", d, err)
		}
		rows, total, err := testDB.ReadBuilds(ctx, site.UserID, site.ID, "", 25, 0)
		if err != nil || total != 1 || !reflect.DeepEqual(rows, beforeJobs) {
			t.Fatal("denied build changed history")
		}
		// A valid account cannot retrieve a foreign job by substituting its own site.
		router := mux.NewRouter()
		router.Handle("/sites/{fqdn}/jobs/{job_id}", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(b.Jobs)))
		req := httptest.NewRequest("GET", "/sites/"+sites[1-owner].FQDN+"/jobs/"+submission.JobID, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[1-owner])
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != 404 {
			t.Fatalf("cross-site job: %d", res.Code)
		}
		req = httptest.NewRequest("GET", "/sites/"+site.FQDN+"/jobs/"+submission.JobID, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[owner])
		res = httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != 200 || !strings.Contains(res.Body.String(), submission.JobID) || strings.Contains(res.Body.String(), submission.Prompt) {
			t.Fatal("owner polling contract failed")
		}
		// Both the submission and current site owner must match before delivery.
		rows, total, err = testDB.ReadBuilds(ctx, sites[1-owner].UserID, site.ID, submission.JobID, 25, 0)
		if err != nil || total != 0 || len(rows) != 0 {
			t.Fatal("database owner filtering failed")
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("denied requests made %d upstream calls", calls.Load())
	}
	// Site lists must filter rows AND totals, for both real accounts.
	for i, token := range tokens {
		req := httptest.NewRequest("GET", "/sites", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(s.ListSites)).ServeHTTP(res, req)
		var result struct {
			Data  []types.Site `json:"data"`
			Total int          `json:"total_count"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil || res.Code != 200 || result.Total != 1 || len(result.Data) != 1 || result.Data[0].ID != sites[i].ID {
			t.Fatalf("site list scope: %s", res.Body.String())
		}
	}
}
