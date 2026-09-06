package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
)

// Do not accidentally apply the proc policy to an older worker image. Labels
// are a compatibility contract, not authentication of untrusted images. The
// operator still controls image provenance. Resolve to ID to avoid tag races.
func (d *DockerSpawner) checkedWorkerImage(ctx context.Context, image string) (string, error) {
	if d.cfg.AppArmorProfile != "pagewright-worker-proc" {
		return image, nil
	}
	status, body, err := d.call(ctx, "GET", "/images/"+url.PathEscape(image)+"/json", nil)
	var inspected struct {
		ID     string `json:"Id"`
		Config struct{ Labels map[string]string }
	}
	if err != nil || status != 200 || json.Unmarshal(body, &inspected) != nil ||
		!regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(inspected.ID) || inspected.Config.Labels["io.pagewright.sandbox-policy"] != "proc-v1" {
		return "", fmt.Errorf("%w: proc-compatible worker image is unavailable or lacks proc-v1 sandbox policy", spawner.ErrNotStarted)
	}
	return inspected.ID, nil
}

// Shared by production launches and real-image acceptance. Only the separately
// installed proc profile opts into replacing Docker's proc overmounts with
// AppArmor denies. Non-proc masks remain; sysfs itself stays read-only.
func workerHostConfig(cfg Config) map[string]any {
	host := map[string]any{
		"NetworkMode": cfg.Network, "RestartPolicy": map[string]string{"Name": "no"},
		"AutoRemove": false, "Privileged": false, "PublishAllPorts": false,
		"CapDrop": []string{"ALL"}, "SecurityOpt": workerSecurityOptions(),
		"NanoCpus": int64(1000000000), "Memory": int64(1073741824), "MemorySwap": int64(1073741824),
		"PidsLimit": 128, "ReadonlyRootfs": true, "Init": true, "ShmSize": 16777216,
		"Ulimits":   []map[string]any{{"Name": "nofile", "Soft": 1024, "Hard": 1024}, {"Name": "core", "Soft": 0, "Hard": 0}},
		"LogConfig": map[string]any{"Type": "json-file", "Config": map[string]string{"max-size": "10m", "max-file": "2"}},
		"Tmpfs": map[string]string{
			cfg.WorkDir: "rw,nosuid,nodev,size=268435456,mode=0700,uid=1000,gid=1000",
			"/tmp":      "rw,nosuid,nodev,noexec,size=67108864,mode=1777",
		},
	}
	if cfg.AppArmorProfile != "" {
		host["SecurityOpt"] = append(workerSecurityOptions(), "apparmor="+cfg.AppArmorProfile)
	}
	if cfg.AppArmorProfile == "pagewright-worker-proc" {
		host["MaskedPaths"] = []string{"/sys/firmware", "/sys/devices/virtual/powercap"}
		host["ReadonlyPaths"] = []string{} // Explicit empty, not null (daemon defaults).
	}
	return host
}
