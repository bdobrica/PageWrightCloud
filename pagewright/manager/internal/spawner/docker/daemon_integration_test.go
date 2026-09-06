//go:build dockerintegration

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRealDockerCreateStart(t *testing.T) {
	t.Setenv("PAGEWRIGHT_JWT_SECRET", "never-forward")
	t.Setenv("PAGEWRIGHT_REDIS_PASSWORD", "never-forward")
	cfg := configFixture()
	cfg.Network = os.Getenv("TEST_DOCKER_NETWORK")
	cfg.Image = os.Getenv("TEST_DOCKER_IMAGE")
	if !strings.HasPrefix(cfg.Network, "pagewright-spawner-test-") || !strings.HasPrefix(cfg.Image, "pagewright-spawner-test-") {
		t.Fatal("run make test-docker-spawner; dedicated test network/image required")
	}
	d, err := NewDockerSpawner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	job := jobFixture()
	id, err := d.Spawn(ctx, job, "http://manager:8081")
	if id != "" {
		t.Cleanup(func() {
			status, _, err := d.call(context.Background(), "DELETE", "/containers/"+id+"?force=true", nil)
			if err != nil || (status != 204 && status != 404) {
				t.Errorf("test container cleanup failed: %d %v", status, err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		State struct {
			Status   string
			ExitCode int
		}
		HostConfig struct {
			NanoCpus, Memory, MemorySwap, PidsLimit int64
			ReadonlyRootfs, Init                    bool
			NetworkMode                             string
			Binds                                   []string
			Privileged                              bool
			SecurityOpt, CapDrop, CapAdd            []string
		}
		Config struct {
			Image string
			User  string
			Env   []string
		}
	}
	for {
		status, body, err := d.call(ctx, "GET", "/containers/"+id+"/json", nil)
		if err != nil || status != 200 {
			t.Fatalf("inspect %d %v", status, err)
		}
		if err := json.Unmarshal(body, &state); err != nil {
			t.Fatal(err)
		}
		if state.State.Status == "exited" {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if state.State.ExitCode != 0 {
		_, body, _ := d.call(ctx, "GET", "/containers/"+id+"/logs?stdout=true&stderr=true", nil)
		t.Fatalf("fixture failed: %d %q", state.State.ExitCode, body)
	}
	observed, err := d.Inspect(ctx, job)
	if err != nil || !observed.Exists || !observed.Exited || observed.ID != id || observed.ExitCode != 0 {
		t.Fatalf("recovery inspection: %+v %v", observed, err)
	}
	if err := d.Kill(ctx, observed.ID); err != nil {
		t.Fatalf("already-exited kill reconciliation: %v", err)
	}
	if state.HostConfig.NetworkMode != cfg.Network || len(state.HostConfig.Binds) != 0 || state.Config.Image != cfg.Image {
		t.Fatal("incorrect Docker configuration")
	}
	hc := state.HostConfig
	if hc.NanoCpus != 1000000000 || hc.Memory != 1073741824 || hc.MemorySwap != hc.Memory || hc.PidsLimit != 128 || !hc.ReadonlyRootfs || !hc.Init {
		t.Fatal("daemon did not apply resource ceilings")
	}
	if state.Config.User != "1000:1000" || state.HostConfig.Privileged || len(state.HostConfig.CapAdd) != 0 || len(state.HostConfig.CapDrop) != 1 || len(state.HostConfig.SecurityOpt) != 2 {
		t.Fatal("worker confinement missing from daemon inspect")
	}
	// Never restart an exited container (or adopt a running one) after conflict.
	if _, err := d.Spawn(ctx, job, "http://manager:8081"); err == nil || errors.Is(err, spawner.ErrNotStarted) {
		t.Fatalf("conflicting name was not uncertain: %v", err)
	}
	missing := cfg
	missing.Image = "pagewright-spawner-test-missing:" + job.JobID
	absent, err := NewDockerSpawner(missing)
	if err != nil {
		t.Fatal(err)
	}
	defer absent.Close()
	if _, err := absent.Spawn(ctx, jobFixture(), "http://manager:8081"); !errors.Is(err, spawner.ErrNotStarted) {
		t.Fatalf("missing image not definitive: %v", err)
	}
}
