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
