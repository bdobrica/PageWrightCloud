//go:build integration

package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

type cleanupWorkers struct {
	state                    spawner.WorkerState
	err                      error
	kills, removes, inspects int
}

func (w *cleanupWorkers) Inspect(context.Context, *types.Job) (spawner.WorkerState, error) {
	w.inspects++
	return w.state, w.err
}
func (w *cleanupWorkers) Kill(context.Context, string) error   { w.kills++; return nil }
func (w *cleanupWorkers) Remove(context.Context, string) error { w.removes++; return nil }

func TestTerminalCleanupSafetyAndDiagnosticRetention(t *testing.T) {
	for _, scenario := range []string{"exited", "created", "running", "unverified", "recent", "active_guard", "log_failure"} {
		t.Run(scenario, func(t *testing.T) {
			b, base := isolatedBackend(t)
			ctx := context.Background()
			enqueue(t, b, base)
			c := claim(t, b, 1)
			j, err := beginTestAttempt(t, b, c)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.ResolveDispatch(ctx, c, "worker", "started"); err != nil {
				t.Fatal(err)
			}
			terminal(t, b, j.JobID)
			before := b.client.Get(ctx, b.jobKeyPrefix+j.JobID).Val()
			receiptKey := b.receiptKey(j.SiteID, j.TargetVersion)
			receipt := b.client.Get(ctx, receiptKey).Val()
			workers := &cleanupWorkers{state: spawner.WorkerState{ID: strings.Repeat("a", 64), Exists: true, Exited: true, ExitCode: 137, OOMKilled: true}}
			now := time.Now().Add(2 * time.Hour)
			switch scenario {
			case "created":
				workers.state.Exited = false
				workers.state.Created = true
			case "running":
				workers.state.Exited = false
				workers.state.Running = true
			case "unverified":
				workers.err = errors.New("wrong ownership")
			case "recent":
				now = time.Now()
			case "active_guard":
				b.client.SAdd(ctx, b.queueKey+":active", j.JobID)
			case "log_failure":
				b.client.LPush(ctx, b.queueKey+":diagnostics:"+j.JobID, "bad-type")
			}
			_, err = b.CleanupWorkers(ctx, workers, 0, now)
			if scenario == "log_failure" {
				if err == nil {
					t.Fatal("failed diagnostic write accepted")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			wantRemove := scenario == "exited" || scenario == "created"
			if (workers.removes == 1) != wantRemove || (workers.kills == 1) != (scenario == "running") {
				t.Fatalf("unsafe mutation: %+v", workers)
			}
			if wantRemove {
				key := b.queueKey + ":diagnostics:" + j.JobID
				log := b.client.Get(ctx, key).Val()
				if !strings.Contains(log, j.JobID) || !strings.Contains(log, j.SiteID) || !strings.Contains(log, j.TargetVersion) || strings.Contains(log, j.LockToken) || len(log) > 4096 {
					t.Fatal("diagnostic not bounded/redacted/correlated")
				}
				ttl := b.client.TTL(ctx, key).Val()
				if ttl <= 0 || ttl > 7*24*time.Hour {
					t.Fatalf("invalid retention: %v", ttl)
				}
				b.client.Expire(ctx, key, time.Minute)
				if _, err := b.CleanupWorkers(ctx, workers, 0, now); err != nil {
					t.Fatal(err)
				}
				if ttl := b.client.TTL(ctx, key).Val(); ttl <= 0 || ttl > time.Minute {
					t.Fatal("retry extended diagnostic retention")
				}
			}
			if b.client.Get(ctx, b.jobKeyPrefix+j.JobID).Val() != before || b.client.Get(ctx, receiptKey).Val() != receipt {
				t.Fatal("cleanup changed canonical history/receipt")
			}
		})
	}
}
