package docker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestInspectOwnershipAndKill(t *testing.T) {
	for _, scenario := range []string{"exited", "created", "running", "missing", "wrong_role", "wrong_network", "wrong_attempt", "wrong_name", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			job := jobFixture()
			launch := *job
			if scenario == "wrong_attempt" {
				launch.FencingToken++
			}
			data, _ := json.Marshal(launch)
			name := fmt.Sprintf("pagewright-job-%x", sha256.Sum256([]byte(job.JobID)))
			id := strings.Repeat("a", 64)
			kills := 0
			d := fakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					if r.URL.Path != "/v1.45/containers/"+id+"/kill" || r.URL.Query().Get("signal") != "SIGKILL" {
						t.Error("wrong kill target")
					}
					kills++
					w.WriteHeader(204)
					return
				}
				if r.URL.Path != "/v1.45/containers/"+name+"/json" {
					t.Error("wrong inspect target")
				}
				if scenario == "missing" {
					w.WriteHeader(404)
					return
				}
				if scenario == "invalid" {
					w.Write([]byte("{"))
					return
				}
				labels := map[string]string{"io.pagewright.role": "worker", "io.pagewright.job_id": job.JobID, "io.pagewright.site_id": job.SiteID, "io.pagewright.network": "test-network"}
				if scenario == "wrong_role" {
					labels["io.pagewright.role"] = "other"
				}
				network := "test-network"
				if scenario == "wrong_network" {
					network = "other"
				}
				recordName := "/" + name
				if scenario == "wrong_name" {
					recordName = "/other"
				}
				state := "exited"
				if scenario == "running" {
					state = "running"
				}
				if scenario == "created" {
					state = "created"
				}
				json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": recordName, "Config": map[string]any{"Labels": labels, "Env": []string{"PAGEWRIGHT_JOB=" + string(data)}}, "HostConfig": map[string]any{"NetworkMode": network}, "State": map[string]any{"Status": state, "Running": state == "running", "ExitCode": 137, "OOMKilled": true}})
			})
			state, err := d.Inspect(context.Background(), job)
			valid := scenario == "exited" || scenario == "created" || scenario == "running" || scenario == "missing"
			if (err == nil) != valid {
				t.Fatalf("inspection %+v %v", state, err)
			}
			if scenario == "exited" && (!state.Exited || state.ExitCode != 137 || !state.OOMKilled) {
				t.Fatal("lost exit evidence")
			}
			if scenario == "created" && (!state.Created || state.Running || state.Exited) {
				t.Fatal("created worker classified incorrectly")
			}
			if scenario == "running" {
				if err := d.Kill(context.Background(), state.ID); err != nil {
					t.Fatal(err)
				}
			}
			if (kills == 1) != (scenario == "running") {
				t.Fatal("unsafe mutation")
			}
			if err := d.Kill(context.Background(), "../other"); err == nil {
				t.Fatal("unsafe ID accepted")
			}
		})
	}
}

func TestRemovalNeverForcesOrDeletesVolumes(t *testing.T) {
	for _, code := range []int{204, 404, 409, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			id := strings.Repeat("a", 64)
			d := fakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "DELETE" || r.URL.Path != "/v1.45/containers/"+id || r.URL.RawQuery != "" {
					t.Errorf("unsafe delete request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(code)
			})
			err := d.Remove(context.Background(), id)
			if (err == nil) != (code == 204 || code == 404) {
				t.Fatalf("remove outcome: %v", err)
			}
			if err := d.Remove(context.Background(), "../unrelated"); err == nil {
				t.Fatal("unsafe ID accepted")
			}
		})
	}
}
