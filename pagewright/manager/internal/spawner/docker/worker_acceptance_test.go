//go:build dockerintegration

package docker

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"testing"
	"time"
)

// Run only explicitly, using a locally built acceptance image. No pulls, binds,
// published ports or network. Cleanup targets only the returned container ID.
func TestRealWorkerAcceptance(t *testing.T) {
	image := os.Getenv("TEST_WORKER_ACCEPTANCE_IMAGE")
	if image == "" {
		t.Skip("run test-worker-cli/compiler for real-image acceptance")
	}
	if !regexp.MustCompile(`^pagewright-(cli|compiler)-test:[a-zA-Z0-9_.-]+$`).MatchString(image) {
		t.Fatal("dedicated acceptance image required")
	}
	cfg := configFixture()
	cfg.AppArmorProfile = os.Getenv("PAGEWRIGHT_WORKER_APPARMOR_PROFILE")
	d, err := NewDockerSpawner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	image, err = d.checkedWorkerImage(context.Background(), image)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Network = "none" // Test only; production requires a dedicated network.
	host := workerHostConfig(cfg)
	body, err := json.Marshal(map[string]any{
		"Image": image, "User": "1000:1000", "HostConfig": host,
		"Env":    []string{"TMPDIR=/work", "PAGEWRIGHT_EXPECT_APPARMOR=" + cfg.AppArmorProfile},
		"Labels": map[string]string{"io.pagewright.role": "acceptance"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	status, response, err := d.call(ctx, "POST", "/containers/create", body)
	if err != nil || status != 201 {
		t.Fatalf("create: %d %v %s", status, err, response)
	}
	var created struct {
		ID string `json:"Id"`
	}
	if json.Unmarshal(response, &created) != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(created.ID) {
		t.Fatal("invalid create ID")
	}
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		status, _, err := d.call(cleanupCtx, "DELETE", "/containers/"+created.ID+"?force=true", nil)
		if err != nil || status != 204 {
			t.Errorf("cleanup %s: %d %v", created.ID, status, err)
		}
	})
	status, response, err = d.call(ctx, "POST", "/containers/"+created.ID+"/start", nil)
	if err != nil || status != 204 {
		t.Fatalf("start: %d %v %s", status, err, response)
	}
	for {
		status, response, err = d.call(ctx, "GET", "/containers/"+created.ID+"/json", nil)
		if err != nil || status != 200 {
			t.Fatalf("inspect: %d %v", status, err)
		}
		var inspected struct {
			State struct {
				Status   string
				ExitCode int
			}
			HostConfig map[string]json.RawMessage
		}
		if err := json.Unmarshal(response, &inspected); err != nil {
			t.Fatal(err)
		}
		if inspected.State.Status == "exited" {
			for _, key := range []string{"MaskedPaths", "ReadonlyPaths", "SecurityOpt", "CapDrop", "ReadonlyRootfs", "Privileged", "Memory", "MemorySwap", "NanoCpus", "PidsLimit"} {
				if want, exists := host[key]; exists {
					var actual any
					if err := json.Unmarshal(inspected.HostConfig[key], &actual); err != nil {
						t.Fatal(err)
					}
					wantJSON, _ := json.Marshal(want)
					actualJSON, _ := json.Marshal(actual)
					if string(wantJSON) != string(actualJSON) {
						t.Errorf("daemon %s: %s want %s", key, actualJSON, wantJSON)
					}
				}
			}
			status, logs, err := d.call(ctx, "GET", "/containers/"+created.ID+"/logs?stdout=true&stderr=true", nil)
			if err != nil || status != 200 {
				t.Fatalf("logs: %d %v", status, err)
			}
			t.Logf("container output: %s", logs)
			if inspected.State.ExitCode != 0 {
				t.Fatalf("acceptance exit %d", inspected.State.ExitCode)
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
