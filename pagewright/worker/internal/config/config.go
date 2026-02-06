package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port             int
	WorkDir          string
	LLMKey           string
	LLMBaseURL       string
	ManagerURL       string
	StorageURL       string
	JobJSON          string
	CodexBinary      string
	InstructionsPath string
}

func LoadConfig() *Config {
	port, _ := strconv.Atoi(getEnv("PAGEWRIGHT_WORKER_PORT", "8082"))

	return &Config{
		Port:             port,
		WorkDir:          getEnv("PAGEWRIGHT_WORK_DIR", "/work"),
		LLMKey:           getEnv("PAGEWRIGHT_LLM_KEY", ""),
		LLMBaseURL:       getEnv("PAGEWRIGHT_LLM_URL", "https://api.openai.com/v1"),
		ManagerURL:       getEnv("PAGEWRIGHT_MANAGER_URL", "http://localhost:8081"),
		StorageURL:       getEnv("PAGEWRIGHT_STORAGE_URL", "http://localhost:8080"),
		JobJSON:          getEnv("PAGEWRIGHT_JOB", ""),
		CodexBinary:      getEnv("PAGEWRIGHT_CODEX_BINARY", "/usr/local/bin/codex"),
		InstructionsPath: getEnv("PAGEWRIGHT_INSTRUCTIONS_PATH", "/.codex/instructions.md"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
