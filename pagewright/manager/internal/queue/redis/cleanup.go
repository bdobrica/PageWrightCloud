package redis

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/redis/go-redis/v9"
)

// Diagnostics intentionally exclude raw Docker/CLI output, prompts, environment
// and lease tokens. Only allowlisted operational fields are retained for 7 days.
type CleanupDiagnostic struct {
	JobID       string          `json:"job_id"`
	SiteID      string          `json:"site_id"`
	Version     string          `json:"target_version"`
	ContainerID string          `json:"container_id"`
	Status      types.JobStatus `json:"status"`
	ExitCode    int             `json:"exit_code"`
	OOMKilled   bool            `json:"oom_killed"`
	Running     bool            `json:"running"`
	ObservedAt  time.Time       `json:"observed_at"`
}

var saveCleanupDiagnostic = redis.NewScript(dispatchPrelude + `
local raw=redis.call('GET',KEYS[8])
if not raw or raw~=ARGV[1] then return 0 end
local j=cjson.decode(raw)
if j.status~='failed' and j.status~='completed' then return 0 end
if redis.call('SISMEMBER',KEYS[2],j.job_id)==1 or redis.call('HGET',KEYS[6],j.site_id)==j.job_id then return 0 end
local prior=redis.call('GET',KEYS[9])
if prior then
 if cjson.decode(prior).container_id~=cjson.decode(ARGV[2]).container_id then return 0 end
 redis.call('SET',KEYS[9],ARGV[2],'XX','KEEPTTL')
else
 redis.call('SET',KEYS[9],ARGV[2],'EX',604800)
end
return 1
`)

func (r *RedisBackend) CleanupWorkers(ctx context.Context, workers spawner.Cleaner, cursor uint64, now time.Time) (uint64, error) {
	keys, next, err := r.client.Scan(ctx, cursor, r.jobKeyPrefix+"*", 128).Result()
	if err != nil {
		return cursor, err
	}
	var first error
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return next, err
		}
		raw, err := r.client.Get(ctx, key).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		var j types.Job
		if json.Unmarshal([]byte(raw), &j) != nil || key != r.jobKeyPrefix+j.JobID {
			continue
		}
		if (j.Status != types.JobStatusCompleted && j.Status != types.JobStatusFailed) || j.LockToken == "" || j.FencingToken <= 0 || j.UpdatedAt.IsZero() || j.UpdatedAt.After(now.Add(-time.Hour)) {
			continue
		}
		state, err := workers.Inspect(ctx, &j)
		if err != nil {
			log.Print("Worker cleanup quarantined: inspection unavailable or identity mismatch")
			continue
		}
		if !state.Exists {
			continue
		}
		diagnostic := CleanupDiagnostic{JobID: j.JobID, SiteID: j.SiteID, Version: j.TargetVersion, ContainerID: state.ID, Status: j.Status, ExitCode: state.ExitCode, OOMKilled: state.OOMKilled, ObservedAt: now.UTC()}
		diagnostic.Running = state.Running
		data, _ := json.Marshal(diagnostic)
		if len(data) > 4096 {
			continue
		}
		approved, err := saveCleanupDiagnostic.Run(ctx, r.client, append(r.dispatchKeys(), key, r.queueKey+":diagnostics:"+j.JobID), raw, string(data)).Int()
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if approved != 1 {
			continue
		}
		if state.Running {
			// A terminal result revoked this attempt's write authority. Stop this exact
			// lingering worker, then require a fresh inspection on a later pass.
			if err := workers.Kill(ctx, state.ID); err != nil {
				log.Print("Terminal worker termination unconfirmed")
			}
			continue
		}
		if !state.Exited && !state.Created {
			continue
		}
		if err := workers.Remove(ctx, state.ID); err != nil {
			log.Print("Terminal worker removal unconfirmed")
		}
	}
	return next, first
}

func (r *RedisBackend) MaintainWorkerCleanup(ctx context.Context, workers spawner.Cleaner) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var cursor uint64
	for {
		operation, cancel := context.WithTimeout(ctx, 20*time.Second)
		next, err := r.CleanupWorkers(operation, workers, cursor, time.Now())
		cursor = next
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Print("Worker cleanup pending")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
