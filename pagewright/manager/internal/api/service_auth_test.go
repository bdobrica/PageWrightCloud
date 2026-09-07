package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/serviceauth"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

func TestScopedCallbacksCannotMutateAnotherJobOrAttempt(t *testing.T) {
	key := strings.Repeat("k", 32)
	t.Setenv("PAGEWRIGHT_SERVICE_TOKEN", key)
	plain, q, _ := fixture()
	job := create(t, plain)
	job.Status = types.JobStatusRunning
	job.LockToken = "current-lock"
	job.FencingToken = 3
	q.jobs[job.JobID] = job
	signed := func(id, lock string) string {
		return serviceauth.Sign(key, serviceauth.Scope{Job: id, Site: job.SiteID, Source: job.SourceVersion, Target: job.TargetVersion, Lock: lock, Fence: 3, Expires: time.Now().Add(time.Minute).Unix()})
	}
	secured := serviceauth.Wrap("manager", plain)
	for _, tc := range []struct {
		token, method, path string
		status              int
	}{
		{"", "POST", "/jobs/" + job.JobID + "/result", 401},
		{signed("other-job", "current-lock"), "POST", "/jobs/" + job.JobID + "/result", 403},
		{signed(job.JobID, "old-lock"), "POST", "/jobs/" + job.JobID + "/result", 403},
		{signed(job.JobID, "old-lock"), "GET", "/jobs/" + job.JobID, 403},
		{signed(job.JobID, "current-lock"), "POST", "/jobs/" + job.JobID + "/write-commit", 403},
	} {
		body, _ := json.Marshal(callback(job))
		r := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("X-Pagewright-Verified-Lock", "current-lock") // Spoofing cannot repair an old token.
		before := q.writes
		w := httptest.NewRecorder()
		secured.ServeHTTP(w, r)
		if w.Code != tc.status || q.writes != before || q.jobs[job.JobID].Status != types.JobStatusRunning {
			t.Fatalf("status=%d expected=%d writes=%d/%d", w.Code, tc.status, q.writes, before)
		}
	}
	body, _ := json.Marshal(callback(job))
	r := httptest.NewRequest("POST", "/jobs/"+job.JobID+"/result", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+signed(job.JobID, "current-lock"))
	w := httptest.NewRecorder()
	secured.ServeHTTP(w, r)
	if w.Code != 200 || q.jobs[job.JobID].Status != types.JobStatusCompleted {
		t.Fatalf("own callback failed: %d", w.Code)
	}
}
