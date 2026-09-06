package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

func TestSiteHostingMatchesDeployment(t *testing.T) {
	sites := NewSitesHandler(nil, nil, nil, 25)
	versions := NewVersionsHandler(nil, nil, nil, 25)
	for _, settings := range [][2]string{{"http", "8084"}, {"https", "443"}, {"https", "8443"}} {
		sites.SetHostingAddress(settings[0], settings[1])
		versions.SetHostingAddress(settings[0], settings[1])
		site := &types.Site{ID: "id", FQDN: "site.example.test"}
		data, _ := json.Marshal(sites.publicSite(site))
		var wire map[string]any
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{"live", "preview"} {
			want, err := versions.deploymentURL(site.FQDN, target)
			if err != nil || wire[target+"_url"] != want {
				t.Fatalf("site/deploy disagreement: %s %v", data, err)
			}
		}
	}
	sites.SetHostingAddress("invalid", "80")
	if got := sites.publicSite(&types.Site{FQDN: "site.example.test"}); got.LiveURL != "" || got.PreviewURL != "" {
		t.Fatal("unsafe config advertised")
	}
	for _, host := range []string{"preview.site.test", "site..test", strings.Repeat("a", 64) + ".test", strings.Repeat("a.", 123) + "test"} {
		if validSiteFQDN(host) {
			t.Fatalf("invalid/reserved hostname allowed: %q", host)
		}
	}
}
