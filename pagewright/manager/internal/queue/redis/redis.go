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
local siteType = redis.call('TYPE', KEYS[3]).ok
if siteType ~= 'none' and siteType ~= 'hash' then return redis.error_reply('sites must be a hash') end
local job = cjson.decode(ARGV[1])
if redis.call('HGET', KEYS[3], job.site_id) then
 job.status = 'failed'
 job.error_code = 'job_busy'
 job.error_message = 'Site already has an accepted job'
 local data = cjson.encode(job)
 redis.call('SET', KEYS[1], data)
 return {data, 1}
end
redis.call('SET', KEYS[1], ARGV[1])
redis.call('HSET', KEYS[3], job.site_id, ARGV[2])
redis.call('RPUSH', KEYS[2], ARGV[2])
return {ARGV[1], 1}
`)

func (r *RedisBackend) CreateJob(ctx context.Context, job *types.Job) (*types.Job, bool, error) {
	data, err := json.Marshal(job)
	if err != nil {
		return nil, false, fmt.Errorf("marshal job: %w", err)
	}
	result, err := createJob.Run(ctx, r.client, []string{r.jobKeyPrefix + job.JobID, r.queueKey, r.queueKey + ":sites"}, string(data), job.JobID).Slice()
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

	if err := updateOutcome.Run(ctx, r.client, append(r.dispatchKeys(), jobKey), string(jobData)).Err(); err != nil {
		if err == redis.Nil {
			return queue.ErrJobNotFound
		}
		return fmt.Errorf("failed to update job: %w", err)
	}

	return nil
}

// Merge callback outcome fields atomically with dispatch metadata and slot release.
// Terminal jobs cannot be reopened by a late running callback.
var updateOutcome = redis.NewScript(dispatchPrelude + `
local raw = redis.call('GET', KEYS[8])
if not raw then return false end
local job = cjson.decode(raw)
local update = cjson.decode(ARGV[1])
if job.status == 'completed' or job.status == 'failed' then
 if update.status ~= job.status then return redis.error_reply('terminal job cannot change status') end
 return 1
end
for _,key in ipairs({'status','result','error_message','error_code','manifest_path','updated_at'}) do job[key] = update[key] end
local data = cjson.encode(job)
redis.call('SET', KEYS[8], data, 'XX', 'KEEPTTL')
if job.status == 'completed' or job.status == 'failed' then
 redis.call('SREM', KEYS[2], job.job_id)
 redis.call('ZREM', KEYS[3], job.job_id)
 redis.call('HSET', KEYS[5], job.job_id, 'terminal')
 if redis.call('HGET', KEYS[6], job.site_id) == job.job_id then redis.call('HDEL', KEYS[6], job.site_id) end
end
return 1
`)

func (r *RedisBackend) Close() error {
	return r.client.Close()
}
