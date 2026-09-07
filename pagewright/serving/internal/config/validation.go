package config

import (
	"fmt"
	"path/filepath"
	"regexp"
)

// Paths are also rendered into nginx directives. Reject rather than clean:
// normalization must not silently redirect an operator's persistent volumes.
func (c *Config) ValidatePaths() error {
	safe := regexp.MustCompile(`^/[A-Za-z0-9/._-]+$`)
	for _, path := range []string{c.WWWRoot, c.NginxSitesEnabled, c.MaintenancePagePath} {
		if !safe.MatchString(path) || filepath.Clean(path) != path || path == "/" {
			return fmt.Errorf("invalid serving filesystem configuration")
		}
	}
	return nil
}
