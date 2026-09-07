package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Server
	Port int

	// Database
	DatabaseURL string

	// External Services
	StorageURL    string
	ManagerURL    string
	ServingURL    string
	HostingScheme string
	HostingPort   string
	SiteDomain    string

	// JWT
	JWTSecret     string
	JWTExpiration time.Duration

	// Google OAuth
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	// OpenAI/LLM
	LLMKey string
	LLMURL string

	// Pagination
	DefaultPageSize int
}

func LoadConfig() *Config {
	return &Config{
		Port:               getEnvInt("PAGEWRIGHT_GATEWAY_PORT", 8085),
		DatabaseURL:        databaseURL(),
		StorageURL:         getEnv("PAGEWRIGHT_STORAGE_URL", ""),
		ManagerURL:         getEnv("PAGEWRIGHT_MANAGER_URL", ""),
		ServingURL:         getEnv("PAGEWRIGHT_SERVING_URL", ""),
		HostingScheme:      getEnv("PAGEWRIGHT_HOSTING_SCHEME", "http"),
		HostingPort:        getEnv("PAGEWRIGHT_HOSTING_PORT", "8084"),
		SiteDomain:         getEnv("PAGEWRIGHT_SITE_DOMAIN", "pagewright.dev"),
		JWTSecret:          getEnv("PAGEWRIGHT_JWT_SECRET", ""),
		JWTExpiration:      getEnvDuration("PAGEWRIGHT_JWT_EXPIRATION", 15*time.Minute),
		GoogleClientID:     getEnv("PAGEWRIGHT_GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("PAGEWRIGHT_GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("PAGEWRIGHT_GOOGLE_REDIRECT_URL", "http://localhost:8085/auth/google/callback"),
		LLMKey:             getEnv("PAGEWRIGHT_LLM_KEY", ""),
		LLMURL:             getEnv("PAGEWRIGHT_LLM_URL", "https://api.openai.com/v1"),
		DefaultPageSize:    getEnvInt("PAGEWRIGHT_DEFAULT_PAGE_SIZE", 25),
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
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
