package handlers

import "testing"

func TestOwnedPlatformNamespace(t *testing.T) {
	h := &SitesHandler{}
	if err := h.SetSiteDomain(" PAGEWRIGHT.IO "); err != nil {
		t.Fatal(err)
	}
	if !h.supportedSiteName("demo.pagewright.io") {
		t.Fatal("valid owned namespace rejected")
	}
	for _, label := range []string{"app", "api", "www", "preview", "admin", "auth", "assets", "cdn", "mail", "status", "support", "ns1", "ns2", "xn--example", "a.b", "-a", "a-"} {
		if h.supportedSiteName(label + ".pagewright.io") {
			t.Errorf("accepted %q", label)
		}
	}
	for _, host := range []string{"pagewright.io", "demo.pagewright.io.evil.test", "demo.other.test", "demo.pagewright.io.", "demo.pagewright.io;"} {
		if h.supportedSiteName(host) {
			t.Errorf("accepted %q", host)
		}
	}
}
