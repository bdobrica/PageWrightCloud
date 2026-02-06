package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/redis/go-redis/v9"
)

const (
	queueKey     = "pagewright:queue"
	jobKeyPrefix = "pagewright:job:"
)

type RedisBackend struct {
	client *redis.Client
}

func NewRedisBackend(addr, password string, db int) (*RedisBackend, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisBackend{
		client: client,
	}, nil
}

func (r *RedisBackend) Push(ctx context.Context, job *types.Job) error {
	// Store job data
	jobKey := jobKeyPrefix + job.JobID
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	// Store job in hash and push to queue
	pipe := r.client.Pipeline()
	pipe.Set(ctx, jobKey, jobData, 24*time.Hour) // TTL of 24 hours
	pipe.RPush(ctx, queueKey, job.JobID)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to push job: %w", err)
	}

	return nil
}

func (r *RedisBackend) Pop(ctx context.Context) (*types.Job, error) {
	// Block for up to 5 seconds waiting for a job
	result, err := r.client.BLPop(ctx, 5*time.Second, queueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No job available
		}
		return nil, fmt.Errorf("failed to pop job: %w", err)
	}

	if len(result) < 2 {
		return nil, fmt.Errorf("invalid BLPOP result")
	}

	jobID := result[1]
	return r.GetJob(ctx, jobID)
}

func (r *RedisBackend) GetJob(ctx context.Context, jobID string) (*types.Job, error) {
	jobKey := jobKeyPrefix + jobID
	jobData, err := r.client.Get(ctx, jobKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("job not found: %s", jobID)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	var job types.Job
	if err := json.Unmarshal([]byte(jobData), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

func (r *RedisBackend) UpdateJob(ctx context.Context, job *types.Job) error {
	jobKey := jobKeyPrefix + job.JobID
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := r.client.Set(ctx, jobKey, jobData, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	return nil
}

func (r *RedisBackend) Close() error {
	return r.client.Close()
}
