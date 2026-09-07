package nginx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnsafeConfigInputsHaveNoEffects(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "false", "/etc/pagewright/503.html")
	for _, host := range []string{"a..test", "a.test;", "a.test\nroot /tmp;", "000-maintenance", "../outside", "A.test"} {
		require.Error(t, m.CreateSiteConfig(host, "/var/www/site", nil, true))
		require.Error(t, m.RemoveSiteConfig(host))
	}
	for _, path := range []string{"/var/www/../outside", "/var/www//site", "/var/www/site;", "/var/www/site\nroot /tmp;", "/"} {
		require.Error(t, m.CreateSiteConfig("site.example.test", path, nil, true))
	}
	require.Error(t, m.CreateSiteConfig("site.example.test", "/var/www/site", []string{"x.test;"}, true))
	m.maintenancePagePath = "/tmp/inject;root/503.html"
	require.Error(t, m.SetMaintenanceMode(true))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.NoFileExists(t, filepath.Join(dir, "000-maintenance"))
}
