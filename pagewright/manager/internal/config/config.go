package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DispatchConcurrency                                                                      int
	DispatchClaimTTL                                                                         time.Duration
	Port                                                                                     int
	QueueBackend                                                                             string
	WorkerSpawner                                                                            string
	RedisAddr                                                                                string
	RedisPassword                                                                            string
	RedisDB                                                                                  int
	LockTTL                                                                                  time.Duration
	LockRenewInterval                                                                        time.Duration
	WorkerImage                                                                              string
	WorkerTimeout                                                                            time.Duration
	WorkerNetwork, DockerSocket, WorkerWorkDir, WorkerStorageURL, WorkerLLMURL, WorkerLLMKey string
	WorkerAppArmorProfile                                                                    string
}

func LoadConfig() *Config {
	return &Config{
		DispatchConcurrency:   getEnvInt("PAGEWRIGHT_DISPATCH_CONCURRENCY", 4),
		DispatchClaimTTL:      getEnvDuration("PAGEWRIGHT_DISPATCH_CLAIM_TTL", 30*time.Second),
		Port:                  getEnvInt("PAGEWRIGHT_PORT", 8081),
		QueueBackend:          getEnv("PAGEWRIGHT_QUEUE_BACKEND", "redis"),
		WorkerSpawner:         getEnv("PAGEWRIGHT_WORKER_SPAWNER", "docker"),
		RedisAddr:             getEnv("PAGEWRIGHT_REDIS_ADDR", "localhost:6379"),
		RedisPassword:         getEnv("PAGEWRIGHT_REDIS_PASSWORD", ""),
		RedisDB:               getEnvInt("PAGEWRIGHT_REDIS_DB", 0),
		LockTTL:               getEnvDuration("PAGEWRIGHT_LOCK_TTL", 5*time.Minute),
		LockRenewInterval:     getEnvDuration("PAGEWRIGHT_LOCK_RENEW_INTERVAL", 1*time.Minute),
		WorkerImage:           getEnv("PAGEWRIGHT_WORKER_IMAGE", "pagewright-worker:m2.10"),
		WorkerNetwork:         getEnv("PAGEWRIGHT_WORKER_NETWORK", ""),
		WorkerAppArmorProfile: getEnv("PAGEWRIGHT_WORKER_APPARMOR_PROFILE", ""),
		DockerSocket:          getEnv("PAGEWRIGHT_DOCKER_SOCKET", "/var/run/docker.sock"),
		WorkerWorkDir:         getEnv("PAGEWRIGHT_WORKER_WORK_DIR", "/work"),
		WorkerStorageURL:      getEnv("PAGEWRIGHT_WORKER_STORAGE_URL", "http://storage:8080"),
		WorkerLLMURL:          getEnv("PAGEWRIGHT_WORKER_LLM_URL", "https://api.openai.com/v1"),
		WorkerLLMKey:          getEnv("PAGEWRIGHT_WORKER_LLM_KEY", ""),
		WorkerTimeout:         getEnvDuration("PAGEWRIGHT_WORKER_TIMEOUT", 30*time.Minute),
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

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
