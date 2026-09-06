package docker

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Keep the upstream allowlist unchanged. Audit every additional rule here.
func TestWorkerSeccompDelta(t *testing.T) {
	upstream, err := os.ReadFile("seccomp-upstream.json")
	if err != nil {
		t.Fatal(err)
	}
	var base, worker map[string]any
	if err := json.Unmarshal(upstream, &base); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(workerSeccomp), &worker); err != nil {
		t.Fatal(err)
	}
	var additions []any
	if err := json.Unmarshal([]byte(`[
 {"names":["clone"],"action":"SCMP_ACT_ALLOW","args":[{"index":0,"value":268435456,"valueTwo":268435456,"op":"SCMP_CMP_MASKED_EQ"}],"excludes":{"arches":["s390","s390x"]}},
 {"names":["unshare"],"action":"SCMP_ACT_ALLOW","args":[{"index":0,"value":2214461439,"valueTwo":0,"op":"SCMP_CMP_MASKED_EQ"}]},
 {"names":["mount","umount2","pivot_root","chroot"],"action":"SCMP_ACT_ALLOW"}
 ]`), &additions); err != nil {
		t.Fatal(err)
	}
	base["syscalls"] = append(base["syscalls"].([]any), additions...)
	if !reflect.DeepEqual(base, worker) {
		t.Fatal("worker seccomp differs from audited upstream plus namespace/mount delta")
	}
	if worker["defaultAction"] != "SCMP_ACT_ERRNO" {
		t.Fatal("must retain default-deny profile")
	}
}

func TestRejectUnreviewedAppArmorProfile(t *testing.T) {
	for _, name := range []string{"unconfined", "docker-default", "arbitrary-profile"} {
		cfg := configFixture()
		cfg.AppArmorProfile = name
		if _, err := NewDockerSpawner(cfg); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
}
