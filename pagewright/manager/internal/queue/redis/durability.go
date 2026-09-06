package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/redis/go-redis/v9"
)

// ValidateDurability is a production startup gate, not a runtime configuration
// mutation. Dedicated Redis must fail writes rather than evict deduplication.
func (r *RedisBackend) ValidateDurability(ctx context.Context) error {
	expected := map[string]string{"appendonly": "yes", "appendfsync": "always", "maxmemory-policy": "noeviction", "no-appendfsync-on-rewrite": "no", "aof-load-truncated": "no"}
	for key, want := range expected {
		values, err := r.client.ConfigGet(ctx, key).Result()
		if err != nil {
			return fmt.Errorf("Redis durability configuration unavailable: %w", err)
		}
		if values[key] != want {
			return fmt.Errorf("Redis durability requires %s=%s", key, want)
		}
	}
	info, err := r.client.Info(ctx, "persistence").Result()
	if err != nil {
		return err
	}
	if !strings.Contains(info, "aof_last_write_status:ok") {
		return fmt.Errorf("Redis AOF writes are not healthy")
	}
	return nil
}

// Before exposing admission, remove inherited TTLs from surviving authoritative
// records. Never touch expiring site leases. An already-lost key is not rebuilt.
// The bound deliberately fails startup on oversized migrations for operator review.
func (r *RedisBackend) ProtectReservations(ctx context.Context) error {
	total := 0
	for _, pattern := range []string{r.jobKeyPrefix + "*", r.queueKey + ":commits:*", "fence:site:*"} {
		var cursor uint64
		for {
			keys, next, err := r.client.Scan(ctx, cursor, pattern, 256).Result()
			if err != nil {
				return err
			}
			total += len(keys)
			if total > 100000 {
				return fmt.Errorf("reservation migration exceeds 100000 records; offline migration required")
			}
			pipe := r.client.Pipeline()
			for _, key := range keys {
				pipe.Persist(ctx, key)
			}
			if _, err := pipe.Exec(ctx); err != nil {
				return err
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	pipe := r.client.Pipeline()
	for _, key := range append(r.dispatchKeys(), r.queueKey+":format") {
		pipe.Persist(ctx, key)
	}
	_, err := pipe.Exec(ctx)
	return err
}

var trimTerminalMetadata = redis.NewScript(dispatchPrelude + `
local raw=redis.call('GET',KEYS[8])
if not raw or raw~=ARGV[2] then return 0 end
local job=cjson.decode(raw)
if job.job_id~=ARGV[1] or (job.status~='completed' and job.status~='failed') then return 0 end
if redis.call('SISMEMBER',KEYS[2],job.job_id)==1 or redis.call('HGET',KEYS[6],job.site_id)==job.job_id then return 0 end
redis.call('HDEL',KEYS[4],job.job_id)
redis.call('HDEL',KEYS[5],job.job_id)
redis.call('ZREM',KEYS[3],job.job_id)
return 1
`)

// Retain permanent canonical job/history/receipt identities. Only disposable
// terminal dispatch bookkeeping ages out after 30 days, in incremental batches.
func (r *RedisBackend) TrimHistoryMetadata(ctx context.Context, cursor uint64, now time.Time) (uint64, error) {
	keys, next, err := r.client.Scan(ctx, cursor, r.jobKeyPrefix+"*", 128).Result()
	if err != nil {
		return cursor, err
	}
	for _, key := range keys {
		raw, err := r.client.Get(ctx, key).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return next, err
		}
		var job types.Job
		if json.Unmarshal([]byte(raw), &job) != nil {
			return next, fmt.Errorf("invalid retained job")
		}
		if job.UpdatedAt.IsZero() || job.UpdatedAt.After(now.Add(-30*24*time.Hour)) {
			continue
		}
		if _, err := trimTerminalMetadata.Run(ctx, r.client, append(r.dispatchKeys(), key), job.JobID, raw).Int(); err != nil {
			return next, err
		}
	}
	return next, nil
}

func (r *RedisBackend) MaintainHistory(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var cursor uint64
	for {
		operation, cancel := context.WithTimeout(ctx, 5*time.Second)
		next, err := r.TrimHistoryMetadata(operation, cursor, time.Now().UTC())
		cursor = next
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Print("Terminal metadata retention pending")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
