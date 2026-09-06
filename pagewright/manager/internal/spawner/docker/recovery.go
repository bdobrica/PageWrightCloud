package docker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

// Inspect uses the deterministic name even when create/start acknowledgement was
// lost. A matching name alone is not authority to inspect/kill an unrelated job.
func (d *DockerSpawner) Inspect(ctx context.Context, job *types.Job) (spawner.WorkerState, error) {
	var out spawner.WorkerState
	name := fmt.Sprintf("pagewright-job-%x", sha256.Sum256([]byte(job.JobID)))
	code, data, err := d.call(ctx, "GET", "/containers/"+name+"/json", nil)
	if err != nil {
		return out, err
	}
	if code == http.StatusNotFound {
		return out, nil
	}
	if code != 200 {
		return out, fmt.Errorf("worker inspection unavailable (%d)", code)
	}
	var record struct {
		ID     string `json:"Id"`
		Name   string
		Config struct {
			Labels map[string]string
			Env    []string
		}
		HostConfig struct{ NetworkMode string }
		State      struct {
			Status    string
			Running   bool
			ExitCode  int
			OOMKilled bool
		}
	}
	if json.Unmarshal(data, &record) != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(record.ID) || record.Name != "/"+name {
		return out, fmt.Errorf("invalid worker inspection")
	}
	labels := record.Config.Labels
	if labels["io.pagewright.role"] != "worker" || labels["io.pagewright.job_id"] != job.JobID || labels["io.pagewright.site_id"] != job.SiteID || labels["io.pagewright.network"] != d.cfg.Network || record.HostConfig.NetworkMode != d.cfg.Network {
		return out, fmt.Errorf("worker ownership mismatch")
	}
	var launch types.Job
	count := 0
	for _, env := range record.Config.Env {
		if strings.HasPrefix(env, "PAGEWRIGHT_JOB=") {
			count++
			if json.Unmarshal([]byte(strings.TrimPrefix(env, "PAGEWRIGHT_JOB=")), &launch) != nil {
				return out, fmt.Errorf("invalid launch identity")
			}
		}
	}
	if count != 1 || launch.JobID != job.JobID || launch.SiteID != job.SiteID || launch.OwnerID != job.OwnerID || launch.SourceVersion != job.SourceVersion || launch.TargetVersion != job.TargetVersion || launch.LockToken != job.LockToken || launch.FencingToken != job.FencingToken {
		return out, fmt.Errorf("worker attempt mismatch")
	}
	out = spawner.WorkerState{ID: record.ID, Exists: true, Running: record.State.Running, Exited: !record.State.Running && (record.State.Status == "exited" || record.State.Status == "dead"), ExitCode: record.State.ExitCode, OOMKilled: record.State.OOMKilled}
	out.Created = !record.State.Running && record.State.Status == "created"
	return out, nil
}

// Non-force removal by verified immutable ID. Docker rejects a concurrent start;
// never remove volumes or turn a 409 into a force-delete.
func (d *DockerSpawner) Remove(ctx context.Context, id string) error {
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) {
		return fmt.Errorf("invalid worker ID")
	}
	code, _, err := d.call(ctx, "DELETE", "/containers/"+id, nil)
	if err != nil {
		return err
	}
	if code != 204 && code != 404 {
		return fmt.Errorf("worker removal unconfirmed (%d)", code)
	}
	return nil
}

// Only immutable IDs obtained from a verified inspection can be passed here.
func (d *DockerSpawner) Kill(ctx context.Context, id string) error {
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) {
		return fmt.Errorf("invalid worker ID")
	}
	code, _, err := d.call(ctx, "POST", "/containers/"+id+"/kill?signal=SIGKILL", nil)
	if err != nil {
		return err
	}
	if code != 204 && code != 404 && code != 409 {
		return fmt.Errorf("worker kill unavailable (%d)", code)
	}
	return nil
}
