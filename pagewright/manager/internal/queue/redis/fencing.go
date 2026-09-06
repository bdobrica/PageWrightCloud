package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/redis/go-redis/v9"
)

// Used within one atomic Redis operation, never a pre-check outside the commit.
const fenceCheck = `
local function validAttempt(job, update)
 if job.status ~= 'running' or not job.lock_token or job.lock_token == '' or not job.fencing_token or job.fencing_token <= 0 then return false end
 for _,key in ipairs({'job_id','site_id','owner_id','source_version','target_version','lock_token','fencing_token'}) do
  if job[key] ~= update[key] then return false end
 end
 return redis.call('GET', KEYS[10]) == job.lock_token
  and tonumber(redis.call('GET', KEYS[11])) == job.fencing_token
  and redis.call('HGET', KEYS[6], job.site_id) == job.job_id
end
`

func (r *RedisBackend) receiptKey(site, version string) string {
	// Encode identities to avoid delimiter collisions.
	b, _ := json.Marshal([]string{site, version})
	return r.queueKey + ":commits:" + string(b)
}

var authorizeWrite = redis.NewScript(dispatchPrelude + fenceCheck + `
local raw = redis.call('GET', KEYS[8])
if not raw then return false end
local job = cjson.decode(raw)
local update = cjson.decode(ARGV[1])
if not validAttempt(job, update) then return false end
local receipt = redis.call('GET', KEYS[9])
local record = {job_id=job.job_id, lock_token=job.lock_token, fencing_token=job.fencing_token}
if receipt then
 record = cjson.decode(receipt)
 if record.job_id ~= job.job_id or record.lock_token ~= job.lock_token or record.fencing_token ~= job.fencing_token then return false end
end
local fingerprint = update.sha256..':'..tostring(update.size)
if record[update.part] and record[update.part] ~= fingerprint then return false end
if update.part == 'manifest' and (not record.artifact or not record.logs) then return false end
record[update.part] = fingerprint
redis.call('SET', KEYS[9], cjson.encode(record))
return 1
`)

func (r *RedisBackend) AuthorizeWrite(ctx context.Context, update *types.WriteCommit) error {
	if update == nil || update.Size < 0 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(update.SHA256) || (update.Part != "artifact" && update.Part != "logs" && update.Part != "manifest") {
		return queue.ErrFenced
	}
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}
	result, err := authorizeWrite.Run(ctx, r.client, append(r.dispatchKeys(), r.jobKeyPrefix+update.JobID, r.receiptKey(update.SiteID, update.TargetVersion), "lock:site:"+update.SiteID, "fence:site:"+update.SiteID), string(data)).Int()
	if err == redis.Nil || (err == nil && result != 1) {
		return queue.ErrFenced
	}
	return err
}

var renewActive = redis.NewScript(dispatchPrelude + `
local id = ARGV[1]
if redis.call('SISMEMBER',KEYS[2],id) ~= 1 then return 0 end
local raw = redis.call('GET', KEYS[8])
 if raw then
  local job = cjson.decode(raw)
  if job.site_id == ARGV[4] and job.status == 'running' and job.dispatch_started_ms and now-job.dispatch_started_ms < tonumber(ARGV[3])
   and job.lock_token and job.lock_token ~= '' and job.fencing_token
   and redis.call('HGET', KEYS[6],job.site_id) == id
   and redis.call('GET',KEYS[9]) == job.lock_token
   and tonumber(redis.call('GET',KEYS[10])) == job.fencing_token then
   redis.call('PEXPIRE',KEYS[9],math.min(tonumber(ARGV[2]),tonumber(ARGV[3])-(now-job.dispatch_started_ms)))
   return 1
  end
 end
return 0
`)

func (r *RedisBackend) RenewActiveLocks(ctx context.Context, ttl, lifetime time.Duration) error {
	if ttl < time.Millisecond || lifetime < ttl {
		return fmt.Errorf("invalid lease duration")
	}
	ids, err := r.client.SMembers(ctx, r.queueKey+":active").Result()
	if err != nil {
		return err
	}
	if len(ids) > 128 {
		return fmt.Errorf("active jobs exceed renewal bound")
	}
	for _, id := range ids {
		job, err := r.GetJob(ctx, id)
		if err != nil {
			return err
		}
		keys := append(r.dispatchKeys(), r.jobKeyPrefix+id, "lock:site:"+job.SiteID, "fence:site:"+job.SiteID)
		if err := renewActive.Run(ctx, r.client, keys, id, ttl.Milliseconds(), lifetime.Milliseconds(), job.SiteID).Err(); err != nil {
			return err
		}
	}
	return nil
}

// All replicas can renew the same authoritative attempts. No process-local
// ownership or reacquisition after expiry; uncertain jobs retain their slots.
func (r *RedisBackend) MaintainLeases(ctx context.Context, ttl, interval, lifetime time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		operation, cancel := context.WithTimeout(ctx, interval)
		if err := r.RenewActiveLocks(operation, ttl, lifetime); err != nil && ctx.Err() == nil {
			log.Printf("Site lease renewal unavailable")
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
