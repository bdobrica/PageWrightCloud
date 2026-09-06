package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/reconciler"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/redis/go-redis/v9"
)

var recoverySnapshot = redis.NewScript(dispatchPrelude + `
if redis.call('SISMEMBER',KEYS[2],ARGV[1]) ~= 1 then return false end
local raw=redis.call('GET',KEYS[8])
if not raw then return false end
local job=cjson.decode(raw)
if job.job_id~=ARGV[1] or job.site_id~=ARGV[2] or job.target_version~=ARGV[3] or job.status~='running' or not job.dispatch_started_ms or not job.lock_token or job.lock_token=='' or not job.fencing_token then return false end
local expired=now-job.dispatch_started_ms>=tonumber(ARGV[4])
local state=redis.call('HGET',KEYS[5],job.job_id)
if state~='started' and state~='uncertain' and not (state=='launching' and expired) then return false end
return {raw,redis.call('GET',KEYS[9]) or '',expired and 1 or 0}
`)

func (r *RedisBackend) Candidates(ctx context.Context, lifetime time.Duration) ([]reconciler.Candidate, error) {
	ids, err := r.client.SMembers(ctx, r.queueKey+":active").Result()
	if err != nil {
		return nil, err
	}
	if len(ids) > 128 {
		return nil, fmt.Errorf("recovery active bound exceeded")
	}
	var out []reconciler.Candidate
	var failures []error
	for _, id := range ids {
		job, err := r.GetJob(ctx, id)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		values, err := recoverySnapshot.Run(ctx, r.client, append(r.dispatchKeys(), r.jobKeyPrefix+id, r.receiptKey(job.SiteID, job.TargetVersion)), id, job.SiteID, job.TargetVersion, lifetime.Milliseconds()).Slice()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		var stored types.Job
		if err := json.Unmarshal([]byte(values[0].(string)), &stored); err != nil {
			failures = append(failures, err)
			continue
		}
		out = append(out, reconciler.Candidate{Job: stored, Receipt: values[1].(string), Expired: values[2].(int64) == 1})
	}
	return out, errors.Join(failures...)
}

// Manager-only recovery path, never exposed to workers. It does not reacquire a
// lease. CAS both the attempt and exact receipt inspected outside Redis. A late
// callback or new reservation makes this observation obsolete.
var recoverOutcome = redis.NewScript(dispatchPrelude + `
local raw=redis.call('GET',KEYS[8])
if not raw then return 0 end
local job=cjson.decode(raw)
local expected=cjson.decode(ARGV[1])
if job.status~='running' or redis.call('SISMEMBER',KEYS[2],job.job_id)~=1 or redis.call('HGET',KEYS[6],job.site_id)~=job.job_id then return 0 end
for _,key in ipairs({'job_id','site_id','owner_id','source_version','target_version','lock_token','fencing_token'}) do
 if job[key]~=expected[key] then return 0 end
end
if not job.lock_token or job.lock_token=='' or not job.fencing_token or job.fencing_token<=0 then return 0 end
if (redis.call('GET',KEYS[9]) or '')~=ARGV[2] then return 0 end
local lease=redis.call('GET',KEYS[10])
if lease and lease~=job.lock_token then return 0 end
if tonumber(redis.call('GET',KEYS[11]))~=job.fencing_token then return 0 end
local status=ARGV[3]
if status~='completed' and status~='failed' then return 0 end
job.result=nil
job.manifest_path=nil
job.error_code=nil
job.error_message=nil
if status=='completed' then
 if ARGV[2]=='' then return 0 end
 local receipt=cjson.decode(ARGV[2])
 if receipt.job_id~=job.job_id or receipt.lock_token~=job.lock_token or receipt.fencing_token~=job.fencing_token or not receipt.artifact or not receipt.logs or not receipt.manifest then return 0 end
 job.manifest_path='/sites/'..job.site_id..'/artifacts/'..job.target_version..'/manifest'
 job.result='Successfully processed site '..job.site_id
else
 job.error_code=ARGV[4]
 job.error_message=ARGV[5]
end
job.status=status
job.updated_at=ARGV[6]
redis.call('SET',KEYS[8],cjson.encode(job),'XX','KEEPTTL')
if lease==job.lock_token then redis.call('DEL',KEYS[10]) end
redis.call('SREM',KEYS[2],job.job_id)
redis.call('ZREM',KEYS[3],job.job_id)
redis.call('HSET',KEYS[5],job.job_id,'terminal')
redis.call('HDEL',KEYS[6],job.site_id)
return 1
`)

func (r *RedisBackend) Recover(ctx context.Context, c reconciler.Candidate, status, code, message string) error {
	data, err := json.Marshal(c.Job)
	if err != nil {
		return err
	}
	n, err := recoverOutcome.Run(ctx, r.client, append(r.dispatchKeys(), r.jobKeyPrefix+c.Job.JobID, r.receiptKey(c.Job.SiteID, c.Job.TargetVersion), "lock:site:"+c.Job.SiteID, "fence:site:"+c.Job.SiteID), string(data), c.Receipt, status, code, message, time.Now().UTC().Format(time.RFC3339Nano)).Int()
	if err != nil {
		return err
	}
	if n != 1 {
		return queue.ErrFenced
	}
	return nil
}
