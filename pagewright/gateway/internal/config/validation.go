package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Root Compose supplies discrete fields from one password setting. URL encoding
// happens here, not via shell interpolation, so punctuation remains password data.
func databaseURL() string {
	if raw := os.Getenv("PAGEWRIGHT_DATABASE_URL"); raw != "" {
		return raw
	}
	if os.Getenv("PAGEWRIGHT_DATABASE_HOST") == "" {
		return ""
	}
	u := &url.URL{Scheme: "postgres", Host: os.Getenv("PAGEWRIGHT_DATABASE_HOST"), Path: "/" + os.Getenv("PAGEWRIGHT_DATABASE_NAME"), User: url.UserPassword(os.Getenv("PAGEWRIGHT_DATABASE_USER"), os.Getenv("PAGEWRIGHT_POSTGRES_PASSWORD"))}
	u.RawQuery = "sslmode=" + url.QueryEscape(getEnv("PAGEWRIGHT_DATABASE_SSLMODE", "disable"))
	return u.String()
}

// Errors identify only configuration keys; never include supplied values or
// parser/driver errors, which may contain credentials and connection strings.
func ValidateDatabase(raw string) error {
	invalid := fmt.Errorf("invalid or missing PostgreSQL configuration; require host, database, user, password (16+ bytes) and explicit sslmode")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || strings.Trim(u.Path, "/") == "" || u.Fragment != "" {
		return invalid
	}
	password, ok := u.User.Password()
	if !ok || len(password) < 16 || strings.TrimSpace(password) != password {
		return invalid
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return invalid
	}
	// Prevent query credentials/host from overriding the validated URL fields.
	for key, values := range q {
		if key != "sslmode" || len(values) != 1 {
			return invalid
		}
	}
	switch q.Get("sslmode") {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return invalid
	}
	return nil
}

func (c *Config) Validate() error {
	if len(c.JWTSecret) < 32 || strings.TrimSpace(c.JWTSecret) != c.JWTSecret || strings.Contains(strings.ToLower(c.JWTSecret), "change-me") || strings.Contains(strings.ToLower(c.JWTSecret), "change-in-production") {
		return fmt.Errorf("PAGEWRIGHT_JWT_SECRET must be an explicit non-placeholder secret of at least 32 bytes")
	}
	if err := ValidateDatabase(c.DatabaseURL); err != nil {
		return err
	}
	for key, raw := range map[string]string{"PAGEWRIGHT_STORAGE_URL": c.StorageURL, "PAGEWRIGHT_MANAGER_URL": c.ManagerURL, "PAGEWRIGHT_SERVING_URL": c.ServingURL} {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid or missing %s", key)
		}
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid PAGEWRIGHT_GATEWAY_PORT")
	}
	if raw := os.Getenv("PAGEWRIGHT_GATEWAY_PORT"); raw != "" {
		if _, err := strconv.Atoi(raw); err != nil {
			return fmt.Errorf("invalid PAGEWRIGHT_GATEWAY_PORT")
		}
	}
	if c.JWTExpiration <= 0 || c.JWTExpiration > 24*time.Hour {
		return fmt.Errorf("PAGEWRIGHT_JWT_EXPIRATION must be positive and at most 24h")
	}
	if raw := os.Getenv("PAGEWRIGHT_JWT_EXPIRATION"); raw != "" {
		if _, err := time.ParseDuration(raw); err != nil {
			return fmt.Errorf("invalid PAGEWRIGHT_JWT_EXPIRATION")
		}
	}
	return nil
}
