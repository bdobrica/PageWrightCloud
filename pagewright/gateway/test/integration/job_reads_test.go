//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func TestOwnerJobHistoryRefreshAndConservativeVersions(t *testing.T) {
	ctx := context.Background()
	user, err := testDB.CreateUser(uuid.NewString()+"@history.test", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	site, err := testDB.CreateSite(user.ID, uuid.NewString()+".history.test", "starter")
	if err != nil {
		t.Fatal(err)
	}
	other, err := testDB.CreateSite(user.ID, uuid.NewString()+".history.test", "starter")
	if err != nil {
		t.Fatal(err)
	}
	token, err := testJWTManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := testJWTManager.GenerateToken(uuid.NewString(), "foreign@history.test")
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := testDB.ReserveBuildSubmission(ctx, &database.BuildSubmission{JobID: uuid.NewString(), SiteID: site.ID, OwnerID: user.ID, SourceVersion: "initial", TargetVersion: uuid.NewString(), Prompt: "PRIVATE GENERATED PROMPT", RequestKey: uuid.NewString(), RequestHash: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"versions": []clients.StorageVersion{{BuildID: s.TargetVersion, Status: "completed", Timestamp: time.Now().UTC()}}})
	}))
	defer storage.Close()
	request := func(path, auth string, status int) []byte {
		t.Helper()
		// A new handler on every read models loss of all gateway/browser memory;
		// nil upstreams also establish that job reads do not depend on manager/storage.
		h := handlers.NewBuildHandler(testDB, nil, nil, nil)
		v := handlers.NewVersionsHandler(testDB, clients.NewStorageClient(storage.URL), nil, 25)
		router := mux.NewRouter()
		api := router.PathPrefix("/sites/{fqdn}").Subrouter()
		api.Use(middleware.AuthMiddleware(testJWTManager))
		api.HandleFunc("/jobs", h.Jobs)
		api.HandleFunc("/jobs/{job_id}", h.Jobs)
		api.HandleFunc("/versions", v.ListVersions)
		r := httptest.NewRequest("GET", path, nil)
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	base := "/sites/" + site.FQDN
	request(base+"/jobs", "", 401)
	request(base+"/jobs", foreign, 403)
	request(base+"/jobs/"+s.JobID, foreign, 403)
	request("/sites/"+other.FQDN+"/jobs/"+s.JobID, token, 404)
	request(base+"/jobs/not-a-uuid", token, 404)
	for _, query := range []string{"?page=0", "?page_size=101", "?page=999999999999999999999"} {
		request(base+"/jobs"+query, token, 400)
	}
	for _, status := range []types.JobStatus{types.JobStatusPending, types.JobStatusRunning, types.JobStatusFailed} {
		if status != types.JobStatusPending {
			if status == types.JobStatusRunning {
				if _, err := testDB.ClaimBuildDispatch(ctx, s.JobID); err != nil {
					t.Fatal(err)
				}
			}
			job := &types.Job{JobAccepted: types.JobAccepted{JobID: s.JobID, SiteID: s.SiteID, OwnerID: s.OwnerID, SourceVersion: s.SourceVersion, TargetVersion: s.TargetVersion, Status: status}, Prompt: s.Prompt, CreatedAt: time.Now(), UpdatedAt: time.Now()}
			if status == types.JobStatusFailed {
				job.ErrorCode = "artifact_incomplete"
				job.ErrorMessage = "PRIVATE raw worker failure"
			}
			if err := testDB.ObserveBuildJob(ctx, job); err != nil {
				t.Fatal(err)
			}
		}
		data := request(base+"/jobs/"+s.JobID, token, 200)
		var got handlers.PublicBuild
		if err := json.Unmarshal(data, &got); err != nil || got.Status != string(status) {
			t.Fatalf("snapshot %s %v", data, err)
		}
		if strings.Contains(string(data), "PRIVATE") || strings.Contains(string(data), s.RequestKey) || strings.Contains(string(data), "owner_id") {
			t.Fatalf("private fields exposed: %s", data)
		}
		var page struct {
			Data  []handlers.PublicBuild `json:"data"`
			Total int                    `json:"total_count"`
		}
		if err := json.Unmarshal(request(base+"/jobs?page_size=1", token, 200), &page); err != nil || page.Total != 1 || len(page.Data) != 1 || page.Data[0].Status != string(status) {
			t.Fatalf("history %+v %v", page, err)
		}
		// A committed artifact alone cannot claim an unconfirmed/failed build succeeded.
		if strings.Contains(string(request(base+"/versions", token, 200)), s.TargetVersion) {
			t.Fatal("storage revived unconfirmed build")
		}
	}
	rows, total, err := testDB.ReadBuilds(ctx, uuid.NewString(), site.ID, "", 25, 0)
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("DB owner scope: %v %d %v", rows, total, err)
	}
	request(base+"/jobs?page=2147483647", token, 200)
	late := &types.Job{JobAccepted: types.JobAccepted{JobID: s.JobID, SiteID: s.SiteID, OwnerID: s.OwnerID, SourceVersion: s.SourceVersion, TargetVersion: s.TargetVersion, Status: types.JobStatusCompleted}, Prompt: s.Prompt, CreatedAt: time.Now(), UpdatedAt: time.Now(), ManifestPath: "/sites/" + s.SiteID + "/artifacts/" + s.TargetVersion + "/manifest"}
	if err := testDB.ObserveBuildJob(ctx, late); err == nil {
		t.Fatal("late completion revived failed history")
	}
	if strings.Contains(string(request(base+"/versions", token, 200)), s.TargetVersion) {
		t.Fatal("late completion revived version")
	}
	proposal := *s
	proposal.JobID, proposal.TargetVersion, proposal.RequestKey = uuid.NewString(), uuid.NewString(), uuid.NewString()
	second, _, err := testDB.ReserveBuildSubmission(ctx, &proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ClaimBuildDispatch(ctx, second.JobID); err != nil {
		t.Fatal(err)
	}
	late.JobID, late.TargetVersion = second.JobID, second.TargetVersion
	late.ManifestPath = "/sites/" + second.SiteID + "/artifacts/" + second.TargetVersion + "/manifest"
	if err := testDB.ObserveBuildJob(ctx, late); err != nil {
		t.Fatal(err)
	}
	var completed handlers.PublicBuild
	if err := json.Unmarshal(request(base+"/jobs/"+second.JobID, token, 200), &completed); err != nil || completed.Status != "completed" {
		t.Fatalf("completed refresh %+v %v", completed, err)
	}
	for i, id := range []string{second.JobID, s.JobID} {
		var page struct {
			Data  []handlers.PublicBuild `json:"data"`
			Total int                    `json:"total_count"`
		}
		if err := json.Unmarshal(request(base+"/jobs?page_size=1&page="+strconv.Itoa(i+1), token, 200), &page); err != nil || page.Total != 2 || len(page.Data) != 1 || page.Data[0].JobID != id {
			t.Fatalf("pagination %+v %v", page, err)
		}
	}
}
