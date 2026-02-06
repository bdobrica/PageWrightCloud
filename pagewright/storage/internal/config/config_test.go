package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear environment variables
	os.Clearenv()

	cfg := LoadConfig()

	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "nfs", cfg.StorageBackend)
	assert.Equal(t, "/nfs", cfg.NFSBasePath)
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("PAGEWRIGHT_PORT", "9090")
	os.Setenv("PAGEWRIGHT_STORAGE_BACKEND", "s3")
	os.Setenv("PAGEWRIGHT_NFS_BASE_PATH", "/custom/path")
	defer os.Clearenv()

	cfg := LoadConfig()

	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "s3", cfg.StorageBackend)
	assert.Equal(t, "/custom/path", cfg.NFSBasePath)
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue int
		expected     int
	}{
		{"Valid integer", "1234", 5678, 1234},
		{"Invalid integer", "invalid", 5678, 5678},
		{"Empty string", "", 5678, 5678},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.envValue != "" {
				os.Setenv("TEST_VAR", tt.envValue)
			}

			result := getEnvInt("TEST_VAR", tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}
