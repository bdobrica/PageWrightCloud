package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// All metadata uses the queue namespace, including isolated integration queues.
// Claims use Redis time, not clocks on manager replicas. Only pre-intent claims
// expire; launch intent is never automatically retried after a crash.
const dispatchPrelude = `
local expected = {'list','set','zset','hash','hash','hash','string'}
for i=1,7 do
 local t = redis.call('TYPE', KEYS[i]).ok
 if t ~= 'none' and t ~= expected[i] then return redis.error_reply('invalid dispatch key type') end
end
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
`

func (r *RedisBackend) dispatchKeys() []string {
	q := r.queueKey
	return []string{q, q + ":active", q + ":claims", q + ":tokens", q + ":states", q + ":sites", q + ":limit"}
}

// Import the previous queue before accepting HTTP requests. Old running entries
// consume slots but are never relaunched; terminal history remains in job keys.
var initializeDispatch = redis.NewScript(dispatchPrelude + `
local format = redis.call('GET', KEYS[8])
local limit = redis.call('GET', KEYS[7])
if limit and limit ~= ARGV[1] and redis.call('SCARD', KEYS[2])>0 then return redis.error_reply('drain active jobs before changing dispatch limit') end
if format then
 if format ~= '2' then return redis.error_reply('unknown queue format') end
 redis.call('SET', KEYS[7], ARGV[1])
 return 1
end
if redis.call('LLEN', KEYS[1])>10000 then return redis.error_reply('legacy queue exceeds migration bound') end
local rows = {}
local seen = {}
local sites = {}
for _,id in ipairs(redis.call('LRANGE', KEYS[1], 0, -1)) do
 local raw = redis.call('GET', ARGV[2]..id)
 if not raw then return redis.error_reply('legacy queued job missing') end
 local job = cjson.decode(raw)
 if type(job.job_id)~='string' or job.job_id~=id or type(job.site_id)~='string' then return redis.error_reply('invalid legacy job identity') end
 if job.status~='pending' and job.status~='running' and job.status~='completed' and job.status~='failed' then return redis.error_reply('unknown legacy job status') end
 if not seen[id] and (job.status=='pending' or job.status=='running') then
  if sites[job.site_id] then return redis.error_reply('multiple legacy jobs for one site; reconcile before upgrade') end
  sites[job.site_id] = id
  table.insert(rows, job)
 end
 seen[id] = true
end
redis.call('DEL', KEYS[1])
for _,job in ipairs(rows) do
 redis.call('HSET', KEYS[6], job.site_id, job.job_id)
 if job.status=='pending' then redis.call('RPUSH', KEYS[1], job.job_id)
 else
  redis.call('SADD', KEYS[2], job.job_id)
  redis.call('HSET', KEYS[5], job.job_id, 'uncertain')
 end
end
redis.call('SET', KEYS[7], ARGV[1])
redis.call('SET', KEYS[8], '2')
return 1
`)

func (r *RedisBackend) InitializeDispatch(ctx context.Context, limit int) error {
	if limit < 1 || limit > 128 {
		return fmt.Errorf("invalid dispatch concurrency")
	}
	return initializeDispatch.Run(ctx, r.client, append(r.dispatchKeys(), r.queueKey+":format"), limit, r.jobKeyPrefix).Err()
}

var claimJob = redis.NewScript(dispatchPrelude + `
local limit = redis.call('GET', KEYS[7])
if limit and limit ~= ARGV[1] then return redis.error_reply('dispatch limits disagree; drain before changing limit') end
-- At most the configured number of claims can exist. Never reclaim intent.
local expired = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now)
for _,id in ipairs(expired) do
 if redis.call('HGET', KEYS[5], id) == 'claimed' then
  redis.call('RPUSH', KEYS[1], id)
  redis.call('SREM', KEYS[2], id)
  redis.call('HDEL', KEYS[4], id)
  redis.call('HDEL', KEYS[5], id)
 end
 redis.call('ZREM', KEYS[3], id)
end
redis.call('SETNX', KEYS[7], ARGV[1])
if redis.call('SCARD', KEYS[2]) >= tonumber(ARGV[1]) then return false end
-- Peek/validate before removal. Corrupt or legacy entries fail closed, not lost.
local id = redis.call('LINDEX', KEYS[1], 0)
if not id then return false end
local raw = redis.call('GET', ARGV[4]..id)
if not raw then return redis.error_reply('queued job missing') end
local job = cjson.decode(raw)
if job.status ~= 'pending' then
 redis.call('LPOP', KEYS[1])
 -- A legacy running job is never dispatched again.
 if job.status == 'running' then
  redis.call('SADD', KEYS[2], id)
  redis.call('HSET', KEYS[5], id, 'uncertain')
  redis.call('HSET', KEYS[6], job.site_id, id)
 end
 return false
end
redis.call('LPOP', KEYS[1])
redis.call('SADD', KEYS[2], id)
redis.call('ZADD', KEYS[3], now+tonumber(ARGV[2]), id)
redis.call('HSET', KEYS[4], id, ARGV[3])
redis.call('HSET', KEYS[5], id, 'claimed')
return {raw, ARGV[3]}
`)

func (r *RedisBackend) Claim(ctx context.Context, limit int, lease time.Duration) (*queue.Claim, error) {
	if limit < 1 || limit > 128 || lease < time.Second || lease > time.Minute {
		return nil, fmt.Errorf("invalid dispatch limit or lease")
	}
	result, err := claimJob.Run(ctx, r.client, r.dispatchKeys(), limit, lease.Milliseconds(), uuid.NewString(), r.jobKeyPrefix).Slice()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var job types.Job
	if err := json.Unmarshal([]byte(result[0].(string)), &job); err != nil {
		return nil, err
	}
	return &queue.Claim{Job: &job, Token: result[1].(string)}, nil
}

var beginDispatch = redis.NewScript(dispatchPrelude + `
local id = ARGV[1]
local expires = redis.call('ZSCORE', KEYS[3], id)
if redis.call('HGET', KEYS[4], id) ~= ARGV[2] or not expires or tonumber(expires)<=now then return false end
if redis.call('HGET', KEYS[5], id) ~= 'claimed' then return false end
local raw = redis.call('GET', ARGV[3]..id)
if not raw then return false end
local job = cjson.decode(raw)
if job.status ~= 'pending' then return false end
job.status = 'running'
job.lock_token = ARGV[4]
job.fencing_token = tonumber(ARGV[5])
job.updated_at = ARGV[6]
local data = cjson.encode(job)
redis.call('SET', ARGV[3]..id, data, 'XX', 'KEEPTTL')
redis.call('ZREM', KEYS[3], id)
redis.call('HSET', KEYS[5], id, 'launching')
return data
`)

func (r *RedisBackend) BeginDispatch(ctx context.Context, c *queue.Claim, lock string, fence int64) (*types.Job, error) {
	data, err := beginDispatch.Run(ctx, r.client, r.dispatchKeys(), c.Job.JobID, c.Token, r.jobKeyPrefix, lock, fence, time.Now().UTC().Format(time.RFC3339Nano)).Text()
	if err == redis.Nil {
		return nil, queue.ErrClaimLost
	}
	if err != nil {
		return nil, err
	}
	var job types.Job
	err = json.Unmarshal([]byte(data), &job)
	return &job, err
}

var resolveDispatch = redis.NewScript(dispatchPrelude + `
local id = ARGV[1]
if redis.call('HGET', KEYS[4], id) ~= ARGV[2] then return false end
local raw = redis.call('GET', ARGV[3]..id)
if not raw then return false end
local job = cjson.decode(raw)
if ARGV[4] ~= '' then job.worker_id = ARGV[4] end
-- A fast terminal callback wins over late dispatch metadata.
if job.status == 'running' then
 redis.call('HSET', KEYS[5], id, ARGV[5])
 if ARGV[5] == 'not_started' then
  job.status = 'failed'
  job.error_code = 'spawn_failed'
  job.error_message = 'Worker was not started'
  job.updated_at = ARGV[6]
  redis.call('SREM', KEYS[2], id)
  if redis.call('HGET', KEYS[6], job.site_id) == id then redis.call('HDEL', KEYS[6], job.site_id) end
 end
end
redis.call('SET', ARGV[3]..id, cjson.encode(job), 'XX', 'KEEPTTL')
return 1
`)

func (r *RedisBackend) ResolveDispatch(ctx context.Context, c *queue.Claim, workerID, outcome string) error {
	if outcome != "started" && outcome != "uncertain" && outcome != "not_started" {
		return fmt.Errorf("invalid dispatch outcome")
	}
	_, err := resolveDispatch.Run(ctx, r.client, r.dispatchKeys(), c.Job.JobID, c.Token, r.jobKeyPrefix, workerID, outcome, time.Now().UTC().Format(time.RFC3339Nano)).Int()
	if err == redis.Nil {
		return queue.ErrClaimLost
	}
	return err
}
