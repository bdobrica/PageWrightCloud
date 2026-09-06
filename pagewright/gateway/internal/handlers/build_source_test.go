package handlers

import (
	"errors"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

type sourceVersions struct {
	versions []clients.StorageVersion
	err      error
	site     string
}

func (s *sourceVersions) ListVersions(site string) ([]clients.StorageVersion, error) {
	s.site = site
	return s.versions, s.err
}

func TestBuildSourcePrecedence(t *testing.T) {
	now := time.Now().UTC()
	v := func(id string, age time.Duration) clients.StorageVersion {
		return clients.StorageVersion{BuildID: id, Status: "completed", Timestamp: now.Add(-age)}
	}
	live := "live"
	for _, tc := range []struct {
		name     string
		versions []clients.StorageVersion
		live     *string
		want     string
		err      error
		fail     bool
	}{
		{name: "bootstrap", want: "initial"},
		{name: "live fallback", live: &live, want: live},
		{name: "bootstrap newer than draft", versions: []clients.StorageVersion{v("initial", 0), v("draft", time.Hour)}, want: "draft"},
		{name: "latest draft before live", versions: []clients.StorageVersion{v("older", time.Hour), v("draft", 0), v("live", 2*time.Hour)}, live: &live, want: "draft"},
		{name: "stable timestamp tie", versions: []clients.StorageVersion{v("z", 0), v("a", 0)}, want: "a"},
		{name: "storage failure never falls back", live: &live, err: errors.New("offline"), fail: true},
		{name: "invalid timestamp", versions: []clients.StorageVersion{{BuildID: "draft", Status: "completed"}}, fail: true},
		{name: "incomplete response", versions: []clients.StorageVersion{{BuildID: "draft", Status: "running", Timestamp: now}}, fail: true},
		{name: "duplicates", versions: []clients.StorageVersion{v("draft", 0), v("draft", 0)}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &sourceVersions{versions: tc.versions, err: tc.err}
			h := &BuildHandler{storageClient: store}
			got, err := h.selectBuildSource(&types.Site{ID: "owned-site", LiveVersionID: tc.live})
			if tc.fail {
				if err == nil || got != "" {
					t.Fatalf("expected failure, got %q, %v", got, err)
				}
			} else {
				if err != nil || got != tc.want {
					t.Fatalf("want %q, got %q, %v", tc.want, got, err)
				}
			}
			if store.site != "owned-site" {
				t.Fatalf("wrong scope: %q", store.site)
			}
		})
	}
}
