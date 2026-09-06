package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
)

func TestProcCompatibilityRequiresSeparateProfile(t *testing.T) {
	for _, profile := range []string{"", "pagewright-worker", "pagewright-worker-proc"} {
		cfg := configFixture()
		cfg.AppArmorProfile = profile
		d, err := NewDockerSpawner(cfg)
		if err != nil {
			t.Fatal(err)
		}
		d.Close()
		host := workerHostConfig(cfg)
		_, masked := host["MaskedPaths"]
		_, readonly := host["ReadonlyPaths"]
		if masked != (profile == "pagewright-worker-proc") || readonly != masked {
			t.Fatalf("incorrect proc opt-in: %s", profile)
		}
		if masked {
			got, _ := json.Marshal(host["MaskedPaths"])
			if string(got) != `["/sys/firmware","/sys/devices/virtual/powercap"]` {
				t.Fatalf("lost non-proc masks: %s", got)
			}
			got, _ = json.Marshal(host["ReadonlyPaths"])
			if string(got) != "[]" {
				t.Fatal("must be explicit empty, not null")
			}
		}
	}
}

func TestProcImageCompatibilityGate(t *testing.T) {
	for _, label := range []string{"", "old-policy", "proc-v1"} {
		t.Run(label, func(t *testing.T) {
			id := "sha256:" + strings.Repeat("b", 64)
			d := fakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/json") {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				json.NewEncoder(w).Encode(map[string]any{"Id": id, "Config": map[string]any{"Labels": map[string]string{"io.pagewright.sandbox-policy": label}}})
			})
			d.cfg.AppArmorProfile = "pagewright-worker-proc"
			got, err := d.checkedWorkerImage(context.Background(), d.cfg.Image)
			if label == "proc-v1" {
				if err != nil || got != id {
					t.Fatalf("image was not pinned: %s %v", got, err)
				}
			} else if !errors.Is(err, spawner.ErrNotStarted) || got != "" {
				t.Fatalf("incompatible image accepted: %s %v", got, err)
			}
		})
	}
}
