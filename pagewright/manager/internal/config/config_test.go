package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfigDefaults(t *testing.T) {
	os.Clearenv()

	cfg := LoadConfig()

	assert.Equal(t, 8081, cfg.Port)
	assert.Equal(t, "redis", cfg.QueueBackend)
	assert.Equal(t, "docker", cfg.WorkerSpawner)
	assert.Equal(t, "localhost:6379", cfg.RedisAddr)
	assert.Equal(t, "", cfg.RedisPassword)
	assert.Equal(t, 0, cfg.RedisDB)
	assert.Equal(t, 5*time.Minute, cfg.LockTTL)
	assert.Equal(t, "pagewright-worker:latest", cfg.WorkerImage)
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("PAGEWRIGHT_PORT", "9091")
	os.Setenv("PAGEWRIGHT_QUEUE_BACKEND", "nats")
	os.Setenv("PAGEWRIGHT_WORKER_SPAWNER", "kubernetes")
	os.Setenv("PAGEWRIGHT_REDIS_ADDR", "redis.example.com:6380")
	os.Setenv("PAGEWRIGHT_REDIS_PASSWORD", "secret")
	os.Setenv("PAGEWRIGHT_REDIS_DB", "1")
	os.Setenv("PAGEWRIGHT_LOCK_TTL", "10m")
	os.Setenv("PAGEWRIGHT_WORKER_IMAGE", "custom-worker:v1")
	defer os.Clearenv()

	cfg := LoadConfig()

	assert.Equal(t, 9091, cfg.Port)
	assert.Equal(t, "nats", cfg.QueueBackend)
	assert.Equal(t, "kubernetes", cfg.WorkerSpawner)
	assert.Equal(t, "redis.example.com:6380", cfg.RedisAddr)
	assert.Equal(t, "secret", cfg.RedisPassword)
	assert.Equal(t, 1, cfg.RedisDB)
	assert.Equal(t, 10*time.Minute, cfg.LockTTL)
	assert.Equal(t, "custom-worker:v1", cfg.WorkerImage)
}
