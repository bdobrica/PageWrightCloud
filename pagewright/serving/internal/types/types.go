package types

import "time"

// Site represents a configured site
type Site struct {
	FQDN           string    `json:"fqdn"`
	Domain         string    `json:"domain"`
	Subdomain      string    `json:"subdomain"`
	PublicVersion  string    `json:"public_version"`
	PreviewVersion string    `json:"preview_version"`
	Aliases        []string  `json:"aliases"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// DeployRequest for deploying a new artifact version
type DeployRequest struct {
	SiteID  string `json:"site_id"`
	Version string `json:"version"`
}

// ActivateRequest for switching active version
type ActivateRequest struct {
	Version string `json:"version"`
}

// AliasRequest for adding/removing domain aliases
type AliasRequest struct {
	Action  string   `json:"action"` // "add" or "remove"
	Aliases []string `json:"aliases"`
}

// MaintenanceRequest for maintenance mode
type MaintenanceRequest struct {
	Enabled bool   `json:"enabled"`
	Message string `json:"message,omitempty"`
}
