package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear environment
	os.Clearenv()

	cfg := LoadConfig()

	assert.Equal(t, 8082, cfg.Port)
	assert.Equal(t, "/work", cfg.WorkDir)
	assert.Equal(t, "", cfg.LLMKey)
	assert.Equal(t, "https://api.openai.com/v1", cfg.LLMBaseURL)
	assert.Equal(t, "http://localhost:8081", cfg.ManagerURL)
	assert.Equal(t, "http://localhost:8080", cfg.StorageURL)
	assert.Equal(t, "/usr/local/bin/codex", cfg.CodexBinary)
	assert.Equal(t, "/.codex/instructions.md", cfg.InstructionsPath)
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Clearenv()
	os.Setenv("PAGEWRIGHT_WORKER_PORT", "9000")
	os.Setenv("PAGEWRIGHT_WORK_DIR", "/custom/work")
	os.Setenv("PAGEWRIGHT_LLM_KEY", "test-key-123")
	os.Setenv("PAGEWRIGHT_LLM_URL", "https://custom.api.com")
	os.Setenv("PAGEWRIGHT_MANAGER_URL", "http://manager:8081")
	os.Setenv("PAGEWRIGHT_STORAGE_URL", "http://storage:8080")
	os.Setenv("PAGEWRIGHT_CODEX_BINARY", "/custom/codex")
	os.Setenv("PAGEWRIGHT_INSTRUCTIONS_PATH", "/custom/instructions.md")

	cfg := LoadConfig()

	assert.Equal(t, 9000, cfg.Port)
	assert.Equal(t, "/custom/work", cfg.WorkDir)
	assert.Equal(t, "test-key-123", cfg.LLMKey)
	assert.Equal(t, "https://custom.api.com", cfg.LLMBaseURL)
	assert.Equal(t, "http://manager:8081", cfg.ManagerURL)
	assert.Equal(t, "http://storage:8080", cfg.StorageURL)
	assert.Equal(t, "/custom/codex", cfg.CodexBinary)
	assert.Equal(t, "/custom/instructions.md", cfg.InstructionsPath)

	os.Clearenv()
}
