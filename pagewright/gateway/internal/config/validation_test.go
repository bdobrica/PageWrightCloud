package config

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func validConfig() *Config {
	return &Config{Port: 8085, DatabaseURL: "postgres://pilot:database-password-for-test@localhost/pilot?sslmode=disable", JWTSecret: strings.Repeat("s", 32), JWTExpiration: 15 * time.Minute, StorageURL: "http://storage:8080", ManagerURL: "http://manager:8081", ServingURL: "http://serving:8083"}
}
func TestCriticalConfiguration(t *testing.T) {
	t.Setenv("PAGEWRIGHT_GATEWAY_PORT", "")
	t.Setenv("PAGEWRIGHT_JWT_EXPIRATION", "")
	if err := validConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Config){
		"missing JWT":      func(c *Config) { c.JWTSecret = "" },
		"old JWT":          func(c *Config) { c.JWTSecret = "dev-secret-change-in-production" },
		"missing database": func(c *Config) { c.DatabaseURL = "" },
		"old database password": func(c *Config) {
			c.DatabaseURL = "postgres://pagewright:pagewright@postgres/pagewright?sslmode=disable"
		},
		"missing service": func(c *Config) { c.StorageURL = "" },
		"credential URL":  func(c *Config) { c.ManagerURL = "http://user:sentinel-private@manager" },
		"bad port":        func(c *Config) { c.Port = 0 },
		"bad duration":    func(c *Config) { c.JWTExpiration = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			mutate(c)
			err := c.Validate()
			if err == nil {
				t.Fatal("accepted invalid config")
			}
			if strings.Contains(err.Error(), "sentinel-private") {
				t.Fatal("secret leaked")
			}
		})
	}
	for key, value := range map[string]string{"PAGEWRIGHT_GATEWAY_PORT": "invalid", "PAGEWRIGHT_JWT_EXPIRATION": "invalid"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if validConfig().Validate() == nil {
				t.Fatal("silent fallback")
			}
		})
	}
}
func TestDatabaseEncodingAndOverrideRejection(t *testing.T) {
	t.Setenv("PAGEWRIGHT_DATABASE_URL", "")
	t.Setenv("PAGEWRIGHT_DATABASE_HOST", "postgres:5432")
	t.Setenv("PAGEWRIGHT_DATABASE_USER", "pagewright")
	t.Setenv("PAGEWRIGHT_DATABASE_NAME", "pagewright")
	t.Setenv("PAGEWRIGHT_DATABASE_SSLMODE", "disable")
	password := "punctuation:@/?#%&=plus+password"
	t.Setenv("PAGEWRIGHT_POSTGRES_PASSWORD", password)
	raw := databaseURL()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := u.User.Password()
	if got != password || ValidateDatabase(raw) != nil {
		t.Fatal("password did not round-trip")
	}
	for _, bad := range []string{raw + "&password=override", raw + "&host=other", raw + "&sslmode=disable", "postgres://sentinel-private%invalid@host/db"} {
		err := ValidateDatabase(bad)
		if err == nil || strings.Contains(err.Error(), "sentinel-private") {
			t.Fatalf("unsafe validation: %v", err)
		}
	}
	t.Setenv("PAGEWRIGHT_JWT_SECRET", "")
	if LoadConfig().JWTSecret != "" {
		t.Fatal("default signing secret returned")
	}
}
