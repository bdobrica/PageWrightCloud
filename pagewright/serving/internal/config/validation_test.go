package config

import "testing"

func TestUnsafeConfiguredPaths(t *testing.T) {
	for _, path := range []string{"/", "relative", "/var/www/../tmp", "/var//www", "/var/www;", "/var/www\n", "/var/$host"} {
		for field := 0; field < 3; field++ {
			c := &Config{WWWRoot: "/var/www", NginxSitesEnabled: "/etc/nginx/sites-enabled", MaintenancePagePath: "/etc/pagewright/503.html"}
			paths := []*string{&c.WWWRoot, &c.NginxSitesEnabled, &c.MaintenancePagePath}
			*paths[field] = path
			if c.ValidatePaths() == nil {
				t.Errorf("accepted %q", path)
			}
		}
	}
}
