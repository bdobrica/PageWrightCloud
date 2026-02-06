package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

type Manager struct {
	sitesEnabledDir     string
	reloadCommand       string
	maintenancePagePath string
	maintenanceEnabled  bool
	mu                  sync.RWMutex
}

func NewManager(sitesEnabledDir, reloadCommand, maintenancePagePath string) *Manager {
	return &Manager{
		sitesEnabledDir:     sitesEnabledDir,
		reloadCommand:       reloadCommand,
		maintenancePagePath: maintenancePagePath,
		maintenanceEnabled:  false,
	}
}

// CreateSiteConfig generates and writes nginx config for a site
func (m *Manager) CreateSiteConfig(fqdn string, sitePath string, aliases []string, enabled bool) error {
	configPath := filepath.Join(m.sitesEnabledDir, fqdn)

	config := m.generateSiteConfig(fqdn, sitePath, aliases, enabled)

	if err := os.MkdirAll(m.sitesEnabledDir, 0755); err != nil {
		return fmt.Errorf("failed to create sites-enabled directory: %w", err)
	}

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return m.Reload()
}

// RemoveSiteConfig removes nginx config for a site
func (m *Manager) RemoveSiteConfig(fqdn string) error {
	configPath := filepath.Join(m.sitesEnabledDir, fqdn)

	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove config file: %w", err)
	}

	return m.Reload()
}

// UpdateAliases updates server_name aliases in config
func (m *Manager) UpdateAliases(fqdn string, sitePath string, aliases []string, enabled bool) error {
	return m.CreateSiteConfig(fqdn, sitePath, aliases, enabled)
}

// SetMaintenanceMode enables/disables global maintenance mode
func (m *Manager) SetMaintenanceMode(enabled bool) error {
	m.mu.Lock()
	m.maintenanceEnabled = enabled
	m.mu.Unlock()

	// Create or remove maintenance config
	maintenanceConfigPath := filepath.Join(m.sitesEnabledDir, "000-maintenance")

	if enabled {
		config := m.generateMaintenanceConfig()
		if err := os.WriteFile(maintenanceConfigPath, []byte(config), 0644); err != nil {
			return fmt.Errorf("failed to write maintenance config: %w", err)
		}
	} else {
		os.Remove(maintenanceConfigPath)
	}

	return m.Reload()
}

// IsMaintenanceMode returns current maintenance mode status
func (m *Manager) IsMaintenanceMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maintenanceEnabled
}

// Reload sends SIGHUP to nginx to reload configuration
func (m *Manager) Reload() error {
	parts := strings.Fields(m.reloadCommand)
	if len(parts) == 0 {
		return fmt.Errorf("invalid reload command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload nginx: %w, output: %s", err, string(output))
	}

	return nil
}

func (m *Manager) generateSiteConfig(fqdn string, sitePath string, aliases []string, enabled bool) string {
	tmpl := `server {
    listen 80;
    server_name {{.FQDN}}{{if .Aliases}} {{.Aliases}}{{end}};

    root {{.SitePath}}/public;
    index index.html;

    {{if not .Enabled}}
    # Site disabled - return 503
    location / {
        return 503;
    }

    error_page 503 @maintenance;
    location @maintenance {
        root {{.MaintenancePath}};
        try_files /503.html =503;
    }
    {{else}}
    # Public site
    location / {
        try_files $uri $uri/ =404;
    }

    # Preview site
    location /preview/ {
        alias {{.SitePath}}/preview/;
        try_files $uri $uri/ =404;
    }

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    {{end}}
}
`

	t := template.Must(template.New("site").Parse(tmpl))

	var aliasesStr string
	if len(aliases) > 0 {
		aliasesStr = strings.Join(aliases, " ")
	}

	var buf strings.Builder
	data := struct {
		FQDN            string
		SitePath        string
		Aliases         string
		Enabled         bool
		MaintenancePath string
	}{
		FQDN:            fqdn,
		SitePath:        sitePath,
		Aliases:         aliasesStr,
		Enabled:         enabled,
		MaintenancePath: filepath.Dir(m.maintenancePagePath),
	}

	t.Execute(&buf, data)
	return buf.String()
}

func (m *Manager) generateMaintenanceConfig() string {
	return fmt.Sprintf(`server {
    listen 80 default_server;
    server_name _;

    location / {
        return 503;
    }

    error_page 503 @maintenance;
    location @maintenance {
        root %s;
        try_files /503.html =503;
    }
}
`, filepath.Dir(m.maintenancePagePath))
}
