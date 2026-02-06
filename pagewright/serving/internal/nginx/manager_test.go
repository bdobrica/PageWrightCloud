package nginx

import (
	"os"
	"path/filepath"
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
	assert.Contains(t, contentStr, "location /preview/")
	assert.Contains(t, contentStr, "# Security headers")
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
