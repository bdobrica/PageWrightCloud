package handlers

import (
	"encoding/json"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"testing"
	"time"
)

func TestVersionNormalization(t *testing.T) {
	stamp := time.Date(2026, 9, 6, 15, 0, 0, 0, time.FixedZone("local", 10800))
	rows := []clients.StorageVersion{{BuildID: "z", Timestamp: stamp, Status: "completed"}, {BuildID: "a", Timestamp: stamp, Status: "completed"}}
	got, err := normalizeVersions("site", rows)
	if err != nil || len(got) != 2 || got[0].BuildID != "a" || got[0].ID != "a" || got[0].SiteID != "site" || got[0].Status != "completed" {
		t.Fatalf("%v %v", got, err)
	}
	data, _ := json.Marshal(got[0])
	var wire map[string]interface{}
	_ = json.Unmarshal(data, &wire)
	if wire["created_at"] != "2026-09-06T12:00:00Z" || len(wire) != 5 {
		t.Fatalf("wrong wire: %s", data)
	}
	for _, bad := range []clients.StorageVersion{{BuildID: "bad/id", Timestamp: stamp, Status: "completed"}, {BuildID: "a", Status: "completed"}, {BuildID: "a", Timestamp: stamp, Status: "success"}} {
		if _, err := normalizeVersions("site", []clients.StorageVersion{bad}); err == nil {
			t.Fatal("invalid record accepted")
		}
	}
	if _, err := normalizeVersions("site", append(rows, rows[0])); err == nil {
		t.Fatal("duplicate accepted")
	}
	empty, err := normalizeVersions("site", nil)
	data, _ = json.Marshal(empty)
	if err != nil || string(data) != "[]" {
		t.Fatal("empty response must be array")
	}
	for _, size := range []int{0, -1, 101} {
		if NewVersionsHandler(nil, nil, nil, size).defaultPageSize != 25 {
			t.Fatal("unsafe page default")
		}
	}
}

func TestConfiguredDeploymentURL(t *testing.T) {
	h := NewVersionsHandler(nil, nil, nil, 25)
	for _, tc := range []struct{ scheme, port, host, target, want string }{
		{"http", "8084", "site.example.test", "preview", "http://preview.site.example.test:8084/"},
		{"https", "443", "site.example.test", "preview", "https://preview.site.example.test/"},
		{"https", "8443", "site.example.test", "live", "https://site.example.test:8443/"},
		{"http", "80", "preview.site.example.test", "live", ""},
		{"http", "80", "site..test", "live", ""},
		{"http", "80", "site.example.test", "other", ""},
		{"http", "80", "site.example.test", "live", "http://site.example.test/"},
		{"javascript", "80", "site.example.test", "preview", ""},
		{"https", "0", "site.example.test", "preview", ""},
		{"http", "80", "site.example.test/evil", "preview", ""},
	} {
		h.SetHostingAddress(tc.scheme, tc.port)
		got, err := h.deploymentURL(tc.host, tc.target)
		if tc.want == "" {
			if err == nil {
				t.Fatal("unsafe URL accepted")
			}
		} else if err != nil || got != tc.want {
			t.Fatalf("%q %v", got, err)
		}
	}
}
