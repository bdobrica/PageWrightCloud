package nginx

import (
	"fmt"
	"os"
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
	validationCommand   string
	probeURL            string
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validateSite(fqdn, sitePath, aliases); err != nil {
		return err
	}
	return m.change(fqdn, []byte(m.generateSiteConfig(fqdn, sitePath, aliases, enabled)), false)
}

// EnsureSiteConfig provisions first-preview routing without replacing an existing
// site's aliases or enabled/disabled policy. Retry reloads even an existing file.
func (m *Manager) EnsureSiteConfig(fqdn, sitePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validateSite(fqdn, sitePath, nil); err != nil {
		return err
	}
	path := filepath.Join(m.sitesEnabledDir, fqdn)
	before, exists, err := readConfig(path)
	if err != nil {
		return err
	}
	if exists {
		return m.change(fqdn, before, false)
	}
	return m.change(fqdn, []byte(m.generateSiteConfig(fqdn, sitePath, nil, true)), false)
}

// RemoveSiteConfig removes nginx config for a site.
func (m *Manager) RemoveSiteConfig(fqdn string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !configName.MatchString(fqdn) {
		return fmt.Errorf("invalid config name")
	}
	return m.change(fqdn, nil, true)
}

// UpdateAliases updates server_name aliases in config
func (m *Manager) UpdateAliases(fqdn string, sitePath string, aliases []string, enabled bool) error {
	return m.CreateSiteConfig(fqdn, sitePath, aliases, enabled)
}

// SetMaintenanceMode enables/disables global maintenance mode
func (m *Manager) SetMaintenanceMode(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.change("000-maintenance", []byte(m.generateMaintenanceConfig()), !enabled); err != nil {
		return err
	}
	m.maintenanceEnabled = enabled
	return nil
}

// IsMaintenanceMode returns current maintenance mode status
func (m *Manager) IsMaintenanceMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, err := os.Stat(filepath.Join(m.sitesEnabledDir, "000-maintenance"))
	return err == nil
}

// Reload sends SIGHUP to nginx to reload configuration
func (m *Manager) Reload() error {
	if m.validationCommand != "" {
		if err := runCommand(m.validationCommand); err != nil {
			return err
		}
	}
	return runCommand(m.reloadCommand)
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
