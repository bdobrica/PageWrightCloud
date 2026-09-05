package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/redis/go-redis/v9"
)

const (
	queueKey     = "pagewright:queue"
	jobKeyPrefix = "pagewright:job:"
)

type RedisBackend struct {
	client       *redis.Client
	queueKey     string
	jobKeyPrefix string
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
		client:       client,
		queueKey:     queueKey,
		jobKeyPrefix: jobKeyPrefix,
	}, nil
}

// Lua runs reservation and queue append atomically. Check the queue type before
// writing because Redis scripts do not roll back preceding commands on error.
var createJob = redis.NewScript(`
local existing = redis.call('GET', KEYS[1])
if existing then return {existing, 0} end
local queueType = redis.call('TYPE', KEYS[2]).ok
if queueType ~= 'none' and queueType ~= 'list' then
 return redis.error_reply('queue must be a list')
end
redis.call('SET', KEYS[1], ARGV[1])
redis.call('RPUSH', KEYS[2], ARGV[2])
return {ARGV[1], 1}
`)

func (r *RedisBackend) CreateJob(ctx context.Context, job *types.Job) (*types.Job, bool, error) {
	data, err := json.Marshal(job)
	if err != nil {
		return nil, false, fmt.Errorf("marshal job: %w", err)
	}
	result, err := createJob.Run(ctx, r.client, []string{r.jobKeyPrefix + job.JobID, r.queueKey}, string(data), job.JobID).Slice()
	if err != nil {
		return nil, false, fmt.Errorf("reserve job: %w", err)
	}
	var stored types.Job
	if err := json.Unmarshal([]byte(result[0].(string)), &stored); err != nil {
		return nil, false, fmt.Errorf("decode reserved job: %w", err)
	}
	return &stored, result[1].(int64) == 1, nil
}

// Merge only worker_id so a callback finishing during Spawn cannot be reverted.
var setWorkerID = redis.NewScript(`
local existing = redis.call('GET', KEYS[1])
if not existing then return false end
local job = cjson.decode(existing)
job.worker_id = ARGV[1]
local data = cjson.encode(job)
redis.call('SET', KEYS[1], data, 'XX', 'KEEPTTL')
return data
`)

func (r *RedisBackend) SetWorkerID(ctx context.Context, jobID, workerID string) (*types.Job, error) {
	data, err := setWorkerID.Run(ctx, r.client, []string{r.jobKeyPrefix + jobID}, workerID).Text()
	if err == redis.Nil {
		return nil, queue.ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("set worker ID: %w", err)
	}
	var job types.Job
	if err := json.Unmarshal([]byte(data), &job); err != nil {
		return nil, fmt.Errorf("decode job: %w", err)
	}
	return &job, nil
}

func (r *RedisBackend) Pop(ctx context.Context) (*types.Job, error) {
	// Block for up to 5 seconds waiting for a job
	result, err := r.client.BLPop(ctx, 5*time.Second, r.queueKey).Result()
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
	jobKey := r.jobKeyPrefix + jobID
	jobData, err := r.client.Get(ctx, jobKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("%w: %s", queue.ErrJobNotFound, jobID)
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
	jobKey := r.jobKeyPrefix + job.JobID
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := r.client.SetArgs(ctx, jobKey, jobData, redis.SetArgs{Mode: "XX", KeepTTL: true}).Err(); err != nil {
		if err == redis.Nil {
			return queue.ErrJobNotFound
		}
		return fmt.Errorf("failed to update job: %w", err)
	}

	return nil
}

func (r *RedisBackend) Close() error {
	return r.client.Close()
}
