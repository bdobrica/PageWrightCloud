package api

import (
	"encoding/json"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"strings"
	"testing"
)

func TestUncertainSpawnRetainsReservationAndLock(t *testing.T) {
	for _, fastCallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "running", true: "fast callback"}[fastCallback], func(t *testing.T) {
			h, q, l, s := fixture()
			s.uncertain = true
			req := explicitRequest()
			if fastCallback {
				s.onSpawn = func(j *types.Job) {
					if r := call(t, h, "POST", "/jobs/"+j.JobID+"/result", callback(*j)); r.Code != 200 {
						t.Fatal(r.Body)
					}
				}
			}
			response := call(t, h, "POST", "/jobs", req)
			assertError(t, response, 503, "spawn_uncertain")
			if strings.Contains(response.Body.String(), "private") {
				t.Fatal("leaked Docker error")
			}
			want := types.JobStatusRunning
			released := 0
			if fastCallback {
				want = types.JobStatusCompleted
				released = 1
			}
			saved := q.jobs[req.JobID]
			if saved.Status != want || saved.WorkerID != "worker" || l.released != released {
				t.Fatalf("unsafe uncertain outcome: %+v releases=%d", saved, l.released)
			}
			replay := call(t, h, "POST", "/jobs", req)
			var got types.Job
			if err := json.Unmarshal(replay.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if replay.Code != 200 || got.Status != want || len(s.jobs) != 1 || l.acquired != 1 {
				t.Fatalf("retry respawned: %d %+v", replay.Code, got)
			}
		})
	}
}
