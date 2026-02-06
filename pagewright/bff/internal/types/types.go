package types

import "time"

// Database Models

type User struct {
	ID            string    `db:"id" json:"id"`
	Email         string    `db:"email" json:"email"`
	PasswordHash  string    `db:"password_hash" json:"-"`
	OAuthProvider *string   `db:"oauth_provider" json:"oauth_provider,omitempty"`
	OAuthID       *string   `db:"oauth_id" json:"oauth_id,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

type Site struct {
	ID               string    `db:"id" json:"id"`
	FQDN             string    `db:"fqdn" json:"fqdn"`
	UserID           string    `db:"user_id" json:"user_id"`
	TemplateID       string    `db:"template_id" json:"template_id"`
	LiveVersionID    *string   `db:"live_version_id" json:"live_version_id,omitempty"`
	PreviewVersionID *string   `db:"preview_version_id" json:"preview_version_id,omitempty"`
	Enabled          bool      `db:"enabled" json:"enabled"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

type SiteAlias struct {
	ID        string    `db:"id" json:"id"`
	SiteID    string    `db:"site_id" json:"site_id"`
	Alias     string    `db:"alias" json:"alias"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Version struct {
	ID        string    `db:"id" json:"id"`
	SiteID    string    `db:"site_id" json:"site_id"`
	BuildID   string    `db:"build_id" json:"build_id"`
	Status    string    `db:"status" json:"status"` // pending, success, failed
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// API Request/Response Types

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"` // seconds
	User      User   `json:"user"`
}

type CreateSiteRequest struct {
	FQDN       string `json:"fqdn"`
	TemplateID string `json:"template_id"`
}

type UpdateSiteRequest struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type AddAliasRequest struct {
	Alias string `json:"alias"`
}

type DeployVersionRequest struct {
	Target string `json:"target"` // "live" or "preview"
}

type BuildRequest struct {
	Message        string  `json:"message"`
	ConversationID *string `json:"conversation_id,omitempty"` // For follow-up clarifications
}

type BuildResponse struct {
	JobID          *string `json:"job_id,omitempty"`          // Set when job is queued
	Question       *string `json:"question,omitempty"`        // Set when clarification needed
	ConversationID *string `json:"conversation_id,omitempty"` // For follow-up
}

type PaginatedRequest struct {
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"`
}

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalCount int         `json:"total_count"`
	TotalPages int         `json:"total_pages"`
}

// External Service Types (for communication with other microservices)

type ManagerJobRequest struct {
	SiteID          string            `json:"site_id"`
	BaseBuildID     string            `json:"base_build_id"`
	RequestedAction string            `json:"requested_action"`
	UserText        string            `json:"user_text"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type ManagerJobResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

type StorageVersionsResponse struct {
	Versions []StorageVersion `json:"versions"`
}

type StorageVersion struct {
	BuildID   string    `json:"build_id"`
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size"`
}

type ServingDeployRequest struct {
	SiteID    string `json:"site_id"`
	VersionID string `json:"version_id"`
}

type ServingActivateRequest struct {
	VersionID string `json:"version_id"`
}

type ServingAliasesRequest struct {
	Aliases []string `json:"aliases"`
}

// Error Response

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
