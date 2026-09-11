package docker

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func configFixture() Config {
	return Config{Image: "pagewright-worker:m2.1", Network: "test-network", Socket: "/var/run/docker.sock", WorkDir: "/work", StorageURL: "http://storage:8080", LLMURL: "https://provider.test/v1", LLMKey: "worker-only-secret"}
}

func TestReadyOnlyPingsDaemon(t *testing.T) {
	var failed atomic.Bool
	d := fakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/_ping" {
			t.Error("readiness attempted non-ping operation")
		}
		if failed.Load() {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte("OK"))
	})
	if err := d.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed.Store(true)
	if err := d.Ready(context.Background()); err == nil {
		t.Fatal("failed daemon ready")
	}
	failed.Store(false)
	if err := d.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Ready(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func jobFixture() *types.Job {
	return &types.Job{JobID: uuid.NewString(), SiteID: "site", OwnerID: "owner", Prompt: "quoted \" request\n$DO_NOT_EXECUTE", SourceVersion: "initial", TargetVersion: uuid.NewString(), Status: types.JobStatusRunning, LockToken: "lease", FencingToken: 7, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
}

func TestExplicitWorkerModel(t *testing.T) {
	d := fakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/create") {
			var body struct{ Env []string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(body.Env, "\n"), "PAGEWRIGHT_LLM_MODEL=gpt-5.1-codex-mini") {
				t.Error("model not forwarded")
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"Id": strings.Repeat("a", 64)})
		} else {
			w.WriteHeader(204)
		}
	})
	d.cfg.LLMModel = "gpt-5.1-codex-mini"
	if _, err := d.Spawn(context.Background(), jobFixture(), "http://manager:8081"); err != nil {
		t.Fatal(err)
	}
}

func fakeEngine(t *testing.T, h http.HandlerFunc) *DockerSpawner {
	t.Helper()
	dir, err := os.MkdirTemp("", "pw-docker-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cfg := configFixture()
	cfg.Socket = filepath.Join(dir, "engine.sock")
	listener, err := net.Listen("unix", cfg.Socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: h}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	d, err := NewDockerSpawner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSpawnWireContract(t *testing.T) {
	t.Setenv("PAGEWRIGHT_JWT_SECRET", "never-forward-this")
	job := jobFixture()
	var calls atomic.Int32
	id := strings.Repeat("a", 64)
	d := fakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if r.Method != "POST" {
			t.Error("wrong method")
		}
		if call == 1 {
			if r.URL.Path != "/v1.45/containers/create" || !strings.HasPrefix(r.URL.Query().Get("name"), "pagewright-job-") {
				t.Errorf("create URL: %s", r.URL)
			}
			var body struct {
				Image      string
				User       string
				Env        []string
				Labels     map[string]string
				HostConfig struct {
					NanoCpus, Memory, MemorySwap, PidsLimit int64
					ReadonlyRootfs, Init                    bool
					NetworkMode                             string
					Privileged, PublishAllPorts, AutoRemove bool
					Binds                                   []string
					Tmpfs                                   map[string]string
					RestartPolicy                           struct{ Name string }
					SecurityOpt, CapDrop                    []string
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			env := map[string]string{}
			for _, value := range body.Env {
				parts := strings.SplitN(value, "=", 2)
				env[parts[0]] = parts[1]
			}
			var got types.Job
			if err := json.Unmarshal([]byte(env["PAGEWRIGHT_JOB"]), &got); err != nil {
				t.Error(err)
			}
			if got.JobID != job.JobID || got.Prompt != job.Prompt || got.OwnerID != job.OwnerID || got.FencingToken != 7 || got.LockToken != "lease" {
				t.Errorf("lost job fields: %+v", got)
			}
			wantEnv := 7
			if os.Getenv("PAGEWRIGHT_SERVICE_TOKEN") != "" {
				wantEnv++
			}
			if env["PAGEWRIGHT_SERVICE_TOKEN"] != "" {
				t.Error("master credential leaked to worker")
			}
			if len(env) != wantEnv || env["PAGEWRIGHT_LLM_KEY"] != "worker-only-secret" || env["PAGEWRIGHT_STORAGE_URL"] != "http://storage:8080" || env["PAGEWRIGHT_MANAGER_URL"] != "http://manager:8081" || env["PAGEWRIGHT_WORK_DIR"] != "/work" {
				t.Error("wrong environment allowlist")
			}
			hc := body.HostConfig
			if hc.NanoCpus != 1000000000 || hc.Memory != 1073741824 || hc.MemorySwap != hc.Memory || hc.PidsLimit != 128 || !hc.ReadonlyRootfs || !hc.Init {
				t.Error("missing resource ceilings")
			}
			if body.Image != "pagewright-worker:m2.1" || body.User != "1000:1000" || hc.NetworkMode != "test-network" || hc.Privileged || hc.PublishAllPorts || hc.AutoRemove || len(hc.Binds) != 0 || hc.RestartPolicy.Name != "no" || !strings.Contains(hc.Tmpfs["/work"], "uid=1000,gid=1000") || body.Labels["io.pagewright.job_id"] != job.JobID || len(hc.SecurityOpt) != 2 || len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
				t.Errorf("unsafe container configuration: %+v", body)
			}
			if strings.Join(hc.SecurityOpt, "\n") != strings.Join(workerSecurityOptions(), "\n") {
				t.Error("missing embedded sandbox profile")
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"Id": id})
		} else {
			if r.URL.Path != "/v1.45/containers/"+id+"/start" {
				t.Error(r.URL)
			}
			w.WriteHeader(204)
		}
	})
	got, err := d.Spawn(context.Background(), job, "http://manager:8081")
	if err != nil || got != id || calls.Load() != 2 {
		t.Fatalf("spawn: %s %v calls=%d", got, err, calls.Load())
	}
}

func TestSpawnErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name          string
		create, start int
		badJSON, drop bool
		definite      bool
	}{
		{name: "missing image", create: 404, definite: true}, {name: "bad request", create: 400, definite: true}, {name: "denied", create: 403, definite: true},
		{name: "name conflict", create: 409}, {name: "daemon failure", create: 500}, {name: "redirect", create: 302}, {name: "invalid ack", create: 201, badJSON: true},
		{name: "lost create ack", drop: true}, {name: "lost start ack", create: 201, drop: true}, {name: "start rejected", create: 201, start: 500}, {name: "already started", create: 201, start: 304},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			d := fakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				if tc.drop && (tc.create == 0 || call == 2) {
					connection, _, _ := w.(http.Hijacker).Hijack()
					connection.Close()
					return
				}
				if call == 1 {
					w.WriteHeader(tc.create)
					if tc.create == 201 {
						if tc.badJSON {
							w.Write([]byte(`{"Id":"../unsafe"}`))
						} else {
							json.NewEncoder(w).Encode(map[string]string{"Id": strings.Repeat("a", 64)})
						}
					}
				} else {
					w.WriteHeader(tc.start)
				}
			})
			id, err := d.Spawn(context.Background(), jobFixture(), "http://manager:8081")
			if err == nil || errors.Is(err, spawner.ErrNotStarted) != tc.definite {
				t.Fatalf("wrong classification: %v", err)
			}
			if !tc.definite && id == "" {
				t.Fatal("uncertain launch lost correlation identity")
			}
			if (tc.create != 201 || tc.badJSON) && calls.Load() != 1 {
				t.Fatal("started without valid create acknowledgement")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("credential leak")
			}
		})
	}
}

func TestInvalidConfigurationAndLaunch(t *testing.T) {
	for _, change := range []func(*Config){func(c *Config) { c.Image = "worker" }, func(c *Config) { c.Image = "worker:latest" }, func(c *Config) { c.Network = "host" }, func(c *Config) { c.Network = "" }, func(c *Config) { c.Socket = "tcp://docker:2375" }, func(c *Config) { c.WorkDir = "/" }, func(c *Config) { c.WorkDir = "/work/../etc" }, func(c *Config) { c.StorageURL = "http://user:secret@storage" }} {
		c := configFixture()
		change(&c)
		if _, err := NewDockerSpawner(c); err == nil {
			t.Errorf("accepted invalid configuration")
		}
	}
	d := fakeEngine(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid launch contacted daemon") })
	if _, err := d.Spawn(context.Background(), nil, "http://manager"); !errors.Is(err, spawner.ErrNotStarted) {
		t.Fatal(err)
	}
	job := jobFixture()
	job.JobID = "../bad"
	if _, err := d.Spawn(context.Background(), job, "http://manager"); !errors.Is(err, spawner.ErrNotStarted) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	invalidTime := jobFixture()
	invalidTime.CreatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := d.Spawn(ctx, invalidTime, "http://manager"); !errors.Is(err, spawner.ErrNotStarted) {
		t.Fatal(err)
	}
	cancel()
	if _, err := d.Spawn(ctx, jobFixture(), "http://manager"); !errors.Is(err, spawner.ErrNotStarted) {
		t.Fatal(err)
	}
}
