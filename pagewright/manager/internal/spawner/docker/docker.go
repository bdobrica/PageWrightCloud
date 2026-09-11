package docker

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/serviceauth"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
)

// Engine API v1.45; Unix-only. No ambient Docker context, remote TCP,
// automatic pulls, shell execution or inherited environment.
type Config struct{ Image, Network, Socket, WorkDir, StorageURL, LLMURL, LLMKey, LLMModel, AppArmorProfile string }
type DockerSpawner struct {
	cfg    Config
	client *http.Client
}

// Pinned default-deny Moby profile plus the nested-user-namespace operations
// required by bubblewrap. Always paired with non-root UID and zero capabilities.
//
//go:embed seccomp-worker.json
var workerSeccomp string

func workerSecurityOptions() []string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(workerSeccomp)); err != nil {
		panic("invalid embedded worker seccomp profile")
	}
	return []string{"no-new-privileges:true", "seccomp=" + compact.String()}
}

func NewDockerSpawner(cfg Config) (*DockerSpawner, error) {
	if cfg.AppArmorProfile != "" && cfg.AppArmorProfile != "pagewright-worker" && cfg.AppArmorProfile != "pagewright-worker-proc" {
		return nil, fmt.Errorf("only reviewed PageWright worker AppArmor profiles are supported")
	}
	tagged := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]*:[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`)
	if !tagged.MatchString(cfg.Image) || strings.HasSuffix(cfg.Image, ":latest") {
		return nil, fmt.Errorf("worker image requires an explicit non-latest tag or digest")
	}
	if cfg.Network == "" || cfg.Network == "host" || cfg.Network == "none" || cfg.Network == "bridge" || cfg.Network == "default" || strings.HasPrefix(cfg.Network, "container:") {
		return nil, fmt.Errorf("worker requires a dedicated Docker network")
	}
	if !strings.HasPrefix(cfg.Socket, "/") || path.Clean(cfg.Socket) != cfg.Socket {
		return nil, fmt.Errorf("Docker socket must be an absolute Unix path")
	}
	if (cfg.WorkDir != "/work" && !strings.HasPrefix(cfg.WorkDir, "/work/")) || path.Clean(cfg.WorkDir) != cfg.WorkDir {
		return nil, fmt.Errorf("worker directory must be /work or a clean child path")
	}
	if !validURL(cfg.StorageURL) || !validURL(cfg.LLMURL) {
		return nil, fmt.Errorf("worker endpoints must be HTTP(S) URLs without credentials, query or fragment")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", cfg.Socket)
	}}
	return &DockerSpawner{cfg: cfg, client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// Read-only daemon probe; readiness never pulls an image or starts a worker.
func (d *DockerSpawner) Ready(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://docker/_ping", nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Docker unavailable")
	}
	return nil
}

func (d *DockerSpawner) Spawn(ctx context.Context, job *types.Job, managerURL string) (string, error) {
	if job == nil || !validURL(managerURL) {
		return "", fmt.Errorf("%w: invalid job or callback endpoint", spawner.ErrNotStarted)
	}
	id, err := uuid.Parse(job.JobID)
	if err != nil || id == uuid.Nil || id.String() != job.JobID || job.SiteID == "" || job.OwnerID == "" || job.Prompt == "" || job.SourceVersion == "" || job.TargetVersion == "" || job.Status != types.JobStatusRunning || job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() {
		return "", fmt.Errorf("%w: incomplete launch snapshot", spawner.ErrNotStarted)
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("%w: request canceled before create", spawner.ErrNotStarted)
	}
	name := fmt.Sprintf("pagewright-job-%x", sha256.Sum256([]byte(job.JobID)))
	payload, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("%w: launch snapshot is not valid JSON", spawner.ErrNotStarted)
	}
	image, err := d.checkedWorkerImage(ctx, d.cfg.Image)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"Image":      image,
		"User":       "1000:1000",
		"Env":        []string{"PAGEWRIGHT_JOB=" + string(payload), "PAGEWRIGHT_WORKER_ID=" + name, "PAGEWRIGHT_WORK_DIR=" + d.cfg.WorkDir, "PAGEWRIGHT_MANAGER_URL=" + managerURL, "PAGEWRIGHT_STORAGE_URL=" + d.cfg.StorageURL, "PAGEWRIGHT_LLM_KEY=" + d.cfg.LLMKey, "PAGEWRIGHT_LLM_URL=" + d.cfg.LLMURL},
		"Labels":     map[string]string{"io.pagewright.role": "worker", "io.pagewright.job_id": job.JobID, "io.pagewright.site_id": job.SiteID, "io.pagewright.network": d.cfg.Network},
		"HostConfig": workerHostConfig(d.cfg),
	}
	if d.cfg.LLMModel != "" {
		body["Env"] = append(body["Env"].([]string), "PAGEWRIGHT_LLM_MODEL="+d.cfg.LLMModel)
	}
	if key := serviceauth.Key(); key != "" {
		token := serviceauth.Sign(key, serviceauth.Scope{Job: job.JobID, Site: job.SiteID, Source: job.SourceVersion, Target: job.TargetVersion, Lock: job.LockToken, Fence: job.FencingToken, Expires: time.Now().Add(17 * time.Minute).Unix()})
		body["Env"] = append(body["Env"].([]string), "PAGEWRIGHT_WORKER_TOKEN="+token)
	}
	data, _ := json.Marshal(body)
	status, response, err := d.call(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), data)
	if err != nil {
		return name, fmt.Errorf("Docker create outcome unknown")
	}
	if status != http.StatusCreated {
		switch status {
		case 400, 401, 403, 404, 406, 422:
			return "", fmt.Errorf("%w: Docker create rejected (HTTP %d)", spawner.ErrNotStarted, status)
		}
		return name, fmt.Errorf("Docker create outcome unknown (HTTP %d)", status)
	}
	var created struct {
		ID string `json:"Id"`
	}
	if json.Unmarshal(response, &created) != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(created.ID) {
		return name, fmt.Errorf("Docker create acknowledgement invalid; outcome unknown")
	}
	// Never adopt/start a pre-existing name after conflict or retry. A lost start
	// response may mean the worker already ran (even if it has since exited).
	status, _, err = d.call(ctx, "POST", "/containers/"+created.ID+"/start", nil)
	if err != nil || status != http.StatusNoContent {
		return created.ID, fmt.Errorf("Docker start outcome unknown")
	}
	return created.ID, nil
}

func (d *DockerSpawner) call(ctx context.Context, method, endpoint string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://docker/v1.45"+endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		return response.StatusCode, nil, fmt.Errorf("invalid Docker acknowledgement")
	}
	return response.StatusCode, data, nil
}
func (d *DockerSpawner) Close() error { d.client.CloseIdleConnections(); return nil }
