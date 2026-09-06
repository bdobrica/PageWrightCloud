package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/redis/go-redis/v9"
)

type AuditIssue struct {
	JobID string `json:"job_id,omitempty"`
	Code  string `json:"code"`
}
type AuditReport struct {
	Jobs    int          `json:"jobs"`
	Pending int          `json:"pending"`
	Active  int          `json:"active"`
	Issues  []AuditIssue `json:"issues"`
}

// Audit is deliberately read-only and omits prompts, lock tokens and credentials.
// It is an offline consistency aid, not proof that missing jobs never executed.
func (r *RedisBackend) Audit(ctx context.Context) (AuditReport, error) {
	out := AuditReport{Issues: []AuditIssue{}}
	length, err := r.client.LLen(ctx, r.queueKey).Result()
	if err != nil {
		return out, err
	}
	if length > 10000 {
		return out, fmt.Errorf("audit queue exceeds 10000 entries")
	}
	queued, err := r.client.LRange(ctx, r.queueKey, 0, -1).Result()
	if err != nil {
		return out, err
	}
	active, err := r.client.SMembers(ctx, r.queueKey+":active").Result()
	if err != nil {
		return out, err
	}
	if len(active) > 128 {
		return out, fmt.Errorf("audit active bound exceeded")
	}
	siteCount, err := r.client.HLen(ctx, r.queueKey+":sites").Result()
	if err != nil {
		return out, err
	}
	if siteCount > 10000 {
		return out, fmt.Errorf("audit site reservation bound exceeded")
	}
	sites, err := r.client.HGetAll(ctx, r.queueKey+":sites").Result()
	if err != nil {
		return out, err
	}
	out.Pending = len(queued)
	out.Active = len(active)
	pending, working, seen := map[string]bool{}, map[string]bool{}, map[string]bool{}
	issue := func(id, code string) { out.Issues = append(out.Issues, AuditIssue{JobID: id, Code: code}) }
	for _, id := range queued {
		if pending[id] {
			issue(id, "duplicate_queue_entry")
		}
		pending[id] = true
	}
	for _, id := range active {
		working[id] = true
	}
	var cursor uint64
	for {
		keys, next, err := r.client.Scan(ctx, cursor, r.jobKeyPrefix+"*", 256).Result()
		if err != nil {
			return out, err
		}
		for _, key := range keys {
			id := strings.TrimPrefix(key, r.jobKeyPrefix)
			if seen[id] {
				continue
			}
			seen[id] = true
			out.Jobs++
			if out.Jobs > 100000 {
				return out, fmt.Errorf("audit exceeds 100000 jobs")
			}
			raw, err := r.client.Get(ctx, key).Bytes()
			if err != nil {
				return out, err
			}
			var j struct {
				types.Job
				Started int64 `json:"dispatch_started_ms"`
			}
			if json.Unmarshal(raw, &j) != nil || j.JobID != id {
				issue(id, "invalid_job_record")
				continue
			}
			ttl, err := r.client.PTTL(ctx, key).Result()
			if err != nil {
				return out, err
			}
			if ttl > 0 {
				issue(id, "expiring_deduplication")
			}
			site, err := r.client.HGet(ctx, r.queueKey+":sites", j.SiteID).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				return out, err
			}
			switch j.Status {
			case types.JobStatusPending:
				if !pending[id] && !working[id] {
					issue(id, "pending_without_queue_or_claim")
				}
				if site != id {
					issue(id, "missing_site_reservation")
				}
			case types.JobStatusRunning:
				if !working[id] {
					issue(id, "running_without_active_slot")
				}
				if site != id {
					issue(id, "missing_site_reservation")
				}
				if j.Started <= 0 || j.LockToken == "" || j.FencingToken <= 0 {
					issue(id, "legacy_attempt_requires_operator")
				}
			case types.JobStatusCompleted, types.JobStatusFailed:
				if working[id] || site == id {
					issue(id, "terminal_reservation_requires_operator")
				}
			default:
				issue(id, "unknown_status")
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	for id := range pending {
		if !seen[id] {
			issue(id, "queued_job_missing")
		}
	}
	for id := range working {
		if !seen[id] {
			issue(id, "active_job_missing")
		}
	}
	for _, id := range sites {
		if !seen[id] {
			issue(id, "site_reserved_job_missing")
		}
	}
	return out, nil
}
