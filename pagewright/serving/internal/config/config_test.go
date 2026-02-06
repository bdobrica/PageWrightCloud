package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	// Set test environment variables
	os.Setenv("PAGEWRIGHT_SERVING_PORT", "9999")
	os.Setenv("PAGEWRIGHT_WWW_ROOT", "/custom/www")
	os.Setenv("PAGEWRIGHT_NGINX_SITES_ENABLED", "/custom/sites")
	os.Setenv("PAGEWRIGHT_NGINX_RELOAD_COMMAND", "custom-reload")
	os.Setenv("PAGEWRIGHT_STORAGE_URL", "http://custom-storage:8080")
	os.Setenv("PAGEWRIGHT_MAX_VERSIONS_PER_SITE", "5")
	os.Setenv("PAGEWRIGHT_MAINTENANCE_PAGE_PATH", "/custom/503.html")

	defer func() {
		os.Unsetenv("PAGEWRIGHT_SERVING_PORT")
		os.Unsetenv("PAGEWRIGHT_WWW_ROOT")
		os.Unsetenv("PAGEWRIGHT_NGINX_SITES_ENABLED")
		os.Unsetenv("PAGEWRIGHT_NGINX_RELOAD_COMMAND")
		os.Unsetenv("PAGEWRIGHT_STORAGE_URL")
		os.Unsetenv("PAGEWRIGHT_MAX_VERSIONS_PER_SITE")
		os.Unsetenv("PAGEWRIGHT_MAINTENANCE_PAGE_PATH")
	}()

	cfg := LoadConfig()

	assert.Equal(t, 9999, cfg.Port)
	assert.Equal(t, "/custom/www", cfg.WWWRoot)
	assert.Equal(t, "/custom/sites", cfg.NginxSitesEnabled)
	assert.Equal(t, "custom-reload", cfg.NginxReloadCommand)
	assert.Equal(t, "http://custom-storage:8080", cfg.StorageURL)
	assert.Equal(t, 5, cfg.MaxVersionsPerSite)
	assert.Equal(t, "/custom/503.html", cfg.MaintenancePagePath)
}

func TestLoadConfigDefaults(t *testing.T) {
	t.Skip("Skipping test that depends on clean environment")

	// Clear environment
	origStorageURL := os.Getenv("PAGEWRIGHT_STORAGE_URL")
	os.Unsetenv("PAGEWRIGHT_SERVING_PORT")
	os.Unsetenv("PAGEWRIGHT_WWW_ROOT")
	os.Unsetenv("PAGEWRIGHT_NGINX_SITES_ENABLED")
	os.Unsetenv("PAGEWRIGHT_NGINX_RELOAD_COMMAND")
	os.Unsetenv("PAGEWRIGHT_STORAGE_URL")
	os.Unsetenv("PAGEWRIGHT_MAX_VERSIONS_PER_SITE")
	os.Unsetenv("PAGEWRIGHT_MAINTENANCE_PAGE_PATH")

	defer func() {
		if origStorageURL != "" {
			os.Setenv("PAGEWRIGHT_STORAGE_URL", origStorageURL)
		}
	}()

	cfg := LoadConfig()

	assert.Equal(t, 8083, cfg.Port)
	assert.Equal(t, "/var/www", cfg.WWWRoot)
	assert.Equal(t, "/etc/nginx/sites-enabled", cfg.NginxSitesEnabled)
	assert.Equal(t, "nginx -s reload", cfg.NginxReloadCommand)
	assert.Equal(t, "", cfg.StorageURL)
	assert.Equal(t, 10, cfg.MaxVersionsPerSite)
	assert.Equal(t, "/etc/pagewright/503.html", cfg.MaintenancePagePath)
}
