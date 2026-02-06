package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                int
	WWWRoot             string
	NginxSitesEnabled   string
	NginxReloadCommand  string
	StorageURL          string
	MaxVersionsPerSite  int
	MaintenancePagePath string
	MaintenanceEnabled  bool
}

func LoadConfig() *Config {
	port, _ := strconv.Atoi(getEnv("PAGEWRIGHT_SERVING_PORT", "8083"))
	maxVersions, _ := strconv.Atoi(getEnv("PAGEWRIGHT_MAX_VERSIONS_PER_SITE", "10"))

	return &Config{
		Port:                port,
		WWWRoot:             getEnv("PAGEWRIGHT_WWW_ROOT", "/var/www"),
		NginxSitesEnabled:   getEnv("PAGEWRIGHT_NGINX_SITES_ENABLED", "/etc/nginx/sites-enabled"),
		NginxReloadCommand:  getEnv("PAGEWRIGHT_NGINX_RELOAD_COMMAND", "nginx -s reload"),
		StorageURL:          getEnv("PAGEWRIGHT_STORAGE_URL", "http://localhost:8080"),
		MaxVersionsPerSite:  maxVersions,
		MaintenancePagePath: getEnv("PAGEWRIGHT_MAINTENANCE_PAGE_PATH", "/etc/pagewright/503.html"),
		MaintenanceEnabled:  false,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
