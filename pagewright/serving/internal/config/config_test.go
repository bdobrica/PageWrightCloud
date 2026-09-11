package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	for key, value := range map[string]string{
		"PAGEWRIGHT_SERVING_PORT": "9999", "PAGEWRIGHT_WWW_ROOT": "/custom/www",
		"PAGEWRIGHT_NGINX_SITES_ENABLED": "/custom/sites", "PAGEWRIGHT_NGINX_RELOAD_COMMAND": "custom-reload",
		"PAGEWRIGHT_STORAGE_URL": "http://custom-storage:8080", "PAGEWRIGHT_MAX_VERSIONS_PER_SITE": "5",
		"PAGEWRIGHT_MAINTENANCE_PAGE_PATH": "/custom/503.html",
	} {
		t.Setenv(key, value)
	}
	require.Equal(t, &Config{Port: 9999, WWWRoot: "/custom/www", NginxSitesEnabled: "/custom/sites",
		NginxReloadCommand: "custom-reload", StorageURL: "http://custom-storage:8080", MaxVersionsPerSite: 5,
		MaintenancePagePath: "/custom/503.html"}, LoadConfig())
}

func TestLoadConfigDefaults(t *testing.T) {
	// Empty and unset values share getEnv's default contract. Setenv restores
	// every caller value (including absence); these tests must not run parallel.
	for _, key := range []string{"PAGEWRIGHT_SERVING_PORT", "PAGEWRIGHT_WWW_ROOT",
		"PAGEWRIGHT_NGINX_SITES_ENABLED", "PAGEWRIGHT_NGINX_RELOAD_COMMAND", "PAGEWRIGHT_STORAGE_URL",
		"PAGEWRIGHT_MAX_VERSIONS_PER_SITE", "PAGEWRIGHT_MAINTENANCE_PAGE_PATH"} {
		t.Setenv(key, "")
	}
	require.Equal(t, &Config{Port: 8083, WWWRoot: "/var/www", NginxSitesEnabled: "/etc/nginx/sites-enabled",
		NginxReloadCommand: "nginx -s reload", StorageURL: "http://localhost:8080", MaxVersionsPerSite: 10,
		MaintenancePagePath: "/etc/pagewright/503.html"}, LoadConfig())
}
