package handlers

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

func TestTLSReadinessFailsClosed(t *testing.T) {
	h := NewSitesHandler(nil, nil, nil, 25)
	h.SetHostingAddress("https", "443")
	path := filepath.Join(t.TempDir(), "ready.json")
	h.SetTLSStatePath(path)
	site := &types.Site{FQDN: "demo.pagewright.io"}
	checkPending := func() {
		t.Helper()
		got := h.publicSite(site)
		if got.LiveURL != "" || got.PreviewURL != "" || got.HostingStatus != "provisioning" {
			t.Fatalf("unsafe links: %+v", got)
		}
		if _, err := h.deploymentURL(site.FQDN, "preview"); !errors.Is(err, ErrTLSProvisioning) {
			t.Fatalf("wrong error: %v", err)
		}
	}
	checkPending()
	for _, raw := range []string{"{", `{"expires":9999999999,"hosts":{"demo.pagewright.io":true}}`, `{"expires":1,"hosts":{"demo.pagewright.io":true}}`} {
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		checkPending()
	}
	raw, _ := json.Marshal(map[string]any{"expires": time.Now().Unix() + 600, "hosts": map[string]bool{site.FQDN: true}})
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	got := h.publicSite(site)
	if got.PreviewURL != "https://demo.preview.pagewright.io/" || got.LiveURL != "https://demo.pagewright.io/" || got.HostingStatus != "ready" {
		t.Fatalf("not ready: %+v", got)
	}
	if _, err := h.deploymentURL("other.pagewright.io", "preview"); !errors.Is(err, ErrTLSProvisioning) {
		t.Fatal("foreign host accepted")
	}
	if err := os.WriteFile(path, append(raw, []byte(` {}`)...), 0644); err != nil {
		t.Fatal(err)
	}
	checkPending()
}
