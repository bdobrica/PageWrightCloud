package nginx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSiteConfig(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		sitesEnabledDir:     tmpDir,
		reloadCommand:       "echo reload",
		maintenancePagePath: "/etc/pagewright/503.html",
	}

	// enabled = true means enabled site
	err := mgr.CreateSiteConfig("blog.example.com", "/var/www/example.com/blog.example.com", []string{"www.example.com"}, true)
	require.NoError(t, err)

	// Verify config file was created
	configPath := filepath.Join(tmpDir, "blog.example.com")
	assert.FileExists(t, configPath)

	// Read and verify content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "server_name blog.example.com www.example.com;")
	assert.Contains(t, contentStr, "root /var/www/example.com/blog.example.com/public;")
	assert.Contains(t, contentStr, "server_name blog.preview.example.com;")
	assert.NotContains(t, contentStr, "location /preview/")
	assert.Contains(t, contentStr, "# Security headers")
}

func TestEnsurePreviewRoutingPreservesExistingPolicy(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "true", "/tmp/503.html")
	require.NoError(t, mgr.EnsureSiteConfig("draft.example.test", "/var/www/site"))
	path := filepath.Join(dir, "draft.example.test")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "root /var/www/site/preview;")
	require.NoError(t, mgr.CreateSiteConfig("draft.example.test", "/var/www/site", []string{"alias.example.test"}, false))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, mgr.EnsureSiteConfig("draft.example.test", "/different/path"))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	mgr.reloadCommand = "false"
	require.Error(t, mgr.EnsureSiteConfig("draft.example.test", "/var/www/site"))
	require.Error(t, mgr.EnsureSiteConfig("new.example.test", "/var/www/new"))
}

func TestLegacyHostingMigrationAndReservedNamespace(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			dir := t.TempDir()
			mgr := NewManager(dir, "true", "/tmp/503.html")
			path := filepath.Join(dir, "site.example.test")
			legacy := mgr.generateLegacySiteConfig("site.example.test", "/var/www/site", []string{"alias.example.test"}, enabled)
			require.NoError(t, os.WriteFile(path, []byte(legacy), 0644))
			require.NoError(t, mgr.EnsureSiteConfig("site.example.test", "/var/www/site"))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, mgr.generateSiteConfig("site.example.test", "/var/www/site", []string{"alias.example.test"}, enabled), string(data))
			require.NotContains(t, string(data), "location /preview/")
			require.NoError(t, os.WriteFile(path, []byte(legacy+"# custom policy\n"), 0644))
			require.Error(t, mgr.EnsureSiteConfig("site.example.test", "/var/www/site"))
			data, err = os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, legacy+"# custom policy\n", string(data))
		})
	}
	dir := t.TempDir()
	mgr := NewManager(dir, "true", "/tmp/503.html")
	require.Error(t, mgr.CreateSiteConfig("Preview.site.example.test", "/var/www/site", nil, true))
	require.Error(t, mgr.CreateSiteConfig("site.example.test", "/var/www/site", []string{"preview.other.test"}, true))
	require.Error(t, mgr.CreateSiteConfig("site.example.test", "/var/www/site", []string{"site.preview.example.test"}, true))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "legacy.example.test"), []byte("server { server_name site.preview.example.test; }"), 0644))
	require.Error(t, mgr.EnsureSiteConfig("site.example.test", "/var/www/site"))
	require.Error(t, mgr.CreateSiteConfig("site.example.test", "/var/www/site", nil, true))
	require.NoFileExists(t, filepath.Join(dir, "site.example.test"))
}

func TestV2PreviewNamespaceMigration(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			dir := t.TempDir()
			mgr := NewManager(dir, "true", "/tmp/503.html")
			path := filepath.Join(dir, "site.example.test")
			old := mgr.generateV2SiteConfig("site.example.test", "/var/www/site", []string{"alias.example.test"}, enabled)
			require.Contains(t, old, "server_name preview.site.example.test;")
			for _, custom := range []string{old + "# custom\n", strings.Replace(old, "root /var/www/site/preview;", "root /other/preview;", 1)} {
				require.NoError(t, os.WriteFile(path, []byte(custom), 0644))
				require.Error(t, mgr.EnsureSiteConfig("site.example.test", "/var/www/site"))
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, custom, string(data))
			}
			require.NoError(t, os.WriteFile(path, []byte(old), 0644))
			mgr.reloadCommand = "false"
			require.Error(t, mgr.EnsureSiteConfig("site.example.test", "/var/www/site"))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, old, string(data))
			mgr.reloadCommand = "true"
			// Both forward and rollback reload failed; emulate supervisor recovery
			// before accepting another configuration transaction.
			require.NoError(t, RecoverConfigs(dir))
			require.NoError(t, mgr.EnsureSiteConfig("site.example.test", "/var/www/site"))
			data, err = os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, mgr.generateSiteConfig("site.example.test", "/var/www/site", []string{"alias.example.test"}, enabled), string(data))
			require.NotContains(t, string(data), "server_name preview.site.example.test;")
			require.Contains(t, string(data), "server_name site.preview.example.test;")
			require.NoError(t, mgr.EnsureSiteConfig("site.example.test", "/var/www/site"))
		})
	}
}

func TestUpdateAliases(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		sitesEnabledDir:     tmpDir,
		reloadCommand:       "echo reload",
		maintenancePagePath: "/etc/pagewright/503.html",
	}

	// Create initial config
	err := mgr.CreateSiteConfig("blog.example.com", "/var/www/example.com/blog.example.com", []string{}, true)
	require.NoError(t, err)

	// Update aliases
	err = mgr.UpdateAliases("blog.example.com", "/var/www/example.com/blog.example.com", []string{"www.example.com", "example.com"}, true)
	require.NoError(t, err)

	// Verify config was updated
	configPath := filepath.Join(tmpDir, "blog.example.com")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "server_name blog.example.com www.example.com example.com;")
}

func TestSetMaintenanceMode(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		sitesEnabledDir:     tmpDir,
		reloadCommand:       "echo reload",
		maintenancePagePath: "/etc/pagewright/503.html",
	}

	// Enable maintenance mode
	err := mgr.SetMaintenanceMode(true)
	require.NoError(t, err)

	// Verify maintenance config was created
	configPath := filepath.Join(tmpDir, "000-maintenance")
	assert.FileExists(t, configPath)

	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "default_server")
	assert.Contains(t, contentStr, "return 503")

	// Disable maintenance mode
	err = mgr.SetMaintenanceMode(false)
	require.NoError(t, err)

	// Verify maintenance config was removed
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDisableSite(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		sitesEnabledDir:     tmpDir,
		reloadCommand:       "echo reload",
		maintenancePagePath: "/etc/pagewright/503.html",
	}

	// Create initial enabled config
	err := mgr.CreateSiteConfig("blog.example.com", "/var/www/example.com/blog.example.com", []string{}, true)
	require.NoError(t, err)

	// Disable site (enabled=false)
	err = mgr.CreateSiteConfig("blog.example.com", "/var/www/example.com/blog.example.com", []string{}, false)
	require.NoError(t, err)

	// Verify config contains return 503
	configPath := filepath.Join(tmpDir, "blog.example.com")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "return 503;")
	assert.Contains(t, contentStr, "# Site disabled")
}

func TestEnableSite(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		sitesEnabledDir:     tmpDir,
		reloadCommand:       "echo reload",
		maintenancePagePath: "/etc/pagewright/503.html",
	}

	// Create disabled config (enabled=false)
	err := mgr.CreateSiteConfig("blog.example.com", "/var/www/example.com/blog.example.com", []string{}, false)
	require.NoError(t, err)

	// Enable site (enabled=true)
	err = mgr.CreateSiteConfig("blog.example.com", "/var/www/example.com/blog.example.com", []string{}, true)
	require.NoError(t, err)

	// Verify config is normal (no return 503)
	configPath := filepath.Join(tmpDir, "blog.example.com")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.NotContains(t, contentStr, "return 503;")
	assert.Contains(t, contentStr, "root /var/www")
}

func TestRemoveSiteConfig(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		sitesEnabledDir:     tmpDir,
		reloadCommand:       "echo reload",
		maintenancePagePath: "/etc/pagewright/503.html",
	}

	// Create config
	err := mgr.CreateSiteConfig("blog.example.com", "/var/www/example.com/blog.example.com", []string{}, true)
	require.NoError(t, err)

	configPath := filepath.Join(tmpDir, "blog.example.com")
	assert.FileExists(t, configPath)

	// Remove config
	err = mgr.RemoveSiteConfig("blog.example.com")
	require.NoError(t, err)

	// Verify config is gone
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))
}
