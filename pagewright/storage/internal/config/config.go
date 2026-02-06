package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port           int
	StorageBackend string
	NFSBasePath    string
}

func LoadConfig() *Config {
	return &Config{
		Port:           getEnvInt("PAGEWRIGHT_PORT", 8080),
		StorageBackend: getEnv("PAGEWRIGHT_STORAGE_BACKEND", "nfs"),
		NFSBasePath:    getEnv("PAGEWRIGHT_NFS_BASE_PATH", "/nfs"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
