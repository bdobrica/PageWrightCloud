//go:build integration

package redis

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/reconciler"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

func TestRecoveryCASAndExpiredAttempt(t *testing.T) {
	for _, scenario := range []string{"complete", "incomplete", "receipt_changed", "terminal_race", "replacement_lease"} {
		t.Run(scenario, func(t *testing.T) {
			b, base := isolatedBackend(t)
			ctx := context.Background()
			enqueue(t, b, base)
			claim := claim(t, b, 1)
			j, err := beginTestAttempt(t, b, claim)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.ResolveDispatch(ctx, claim, "worker", "started"); err != nil {
				t.Fatal(err)
			}
			progress := *j
			progress.Result = "intermediate progress"
			progress.ErrorMessage = "intermediate diagnostic"
			if err := b.UpdateJob(ctx, &progress); err != nil {
				t.Fatal(err)
			}
			u := writeFixture(j)
			if scenario == "complete" || scenario == "terminal_race" {
				approveParts(t, b, j)
			} else {
				if err := b.AuthorizeWrite(ctx, u); err != nil {
					t.Fatal(err)
				}
			}
			candidates, err := b.Candidates(ctx, time.Minute)
			if err != nil || len(candidates) != 1 {
				t.Fatalf("candidates %v %v", candidates, err)
			}
			c := candidates[0]
			if c.Expired {
				t.Fatal("early expiry")
			}
			if scenario == "receipt_changed" {
				u.Part = "logs"
				if err := b.AuthorizeWrite(ctx, u); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "terminal_race" {
				terminal(t, b, j.JobID)
			}
			// Remove only this disposable test attempt's lease, emulating expiry.
			b.client.Del(ctx, "lock:site:"+j.SiteID)
			if scenario == "replacement_lease" {
				b.client.Set(ctx, "lock:site:"+j.SiteID, "replacement", time.Minute)
			}
			before := b.client.Get(ctx, b.receiptKey(j.SiteID, j.TargetVersion)).Val()
			status := "failed"
			if scenario == "complete" {
				status = "completed"
			}
			err = b.Recover(ctx, c, status, "artifact_incomplete", "reserved bytes missing")
			reject := scenario == "receipt_changed" || scenario == "terminal_race" || scenario == "replacement_lease"
			if reject {
				if !errors.Is(err, queue.ErrFenced) {
					t.Fatalf("unsafe recovery: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got, _ := b.GetJob(ctx, j.JobID)
				if string(got.Status) != status || b.client.SCard(ctx, b.queueKey+":active").Val() != 0 || b.client.HExists(ctx, b.queueKey+":sites", j.SiteID).Val() {
					t.Fatal("terminal release failed")
				}
				if status == "completed" && (got.ErrorMessage != "" || got.ErrorCode != "") {
					t.Fatal("completion retained stale diagnostics")
				}
				if status == "failed" && (got.Result != "" || got.ManifestPath != "") {
					t.Fatal("failure retained stale progress")
				}
				if err := b.AuthorizeWrite(ctx, u); !errors.Is(err, queue.ErrFenced) {
					t.Fatal("expired attempt revived")
				}
				if err := b.Recover(ctx, c, status, "", ""); !errors.Is(err, queue.ErrFenced) {
					t.Fatal("terminal recovery replayed")
				}
			}
			if before != b.client.Get(ctx, b.receiptKey(j.SiteID, j.TargetVersion)).Val() {
				t.Fatal("receipt changed")
			}
			if scenario == "replacement_lease" && b.client.Get(ctx, "lock:site:"+j.SiteID).Val() != "replacement" {
				t.Fatal("replacement lease deleted")
			}
		})
	}
}

func TestRecoveryLaunchIntentDeadline(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	enqueue(t, b, base)
	j, err := beginTestAttempt(t, b, claim(t, b, 1))
	if err != nil {
		t.Fatal(err)
	}
	c, err := b.Candidates(ctx, time.Minute)
	if err != nil || len(c) != 0 {
		t.Fatal("inflight launch reconciled")
	}
	// Redis-clock deadline, without waiting for the production timeout.
	b.client.Eval(ctx, `local j=cjson.decode(redis.call('GET',KEYS[1]));j.dispatch_started_ms=1;redis.call('SET',KEYS[1],cjson.encode(j))`, []string{b.jobKeyPrefix + j.JobID})
	c, err = b.Candidates(ctx, time.Minute)
	if err != nil || len(c) != 1 || !c[0].Expired {
		t.Fatalf("abandoned intent not timed out: %v %v", c, err)
	}
	if err := b.Recover(ctx, c[0], "failed", "worker_timeout", "deadline exceeded"); err != nil {
		t.Fatal(err)
	}
	if b.client.Exists(ctx, "lock:site:"+j.SiteID).Val() != 0 {
		t.Fatal("timeout retained lease")
	}
}

var _ reconciler.Backend = (*RedisBackend)(nil)

type exitedWorker struct{}

func (exitedWorker) Inspect(context.Context, *types.Job) (spawner.WorkerState, error) {
	return spawner.WorkerState{Exists: true, Exited: true}, nil
}
func (exitedWorker) Kill(context.Context, string) error { return fmt.Errorf("unexpected kill") }

func TestLostCallbackRecoveryWithReservedBytes(t *testing.T) {
	for _, materialized := range []bool{true, false} {
		t.Run(fmt.Sprintf("materialized=%v", materialized), func(t *testing.T) {
			b, base := isolatedBackend(t)
			ctx := context.Background()
			enqueue(t, b, base)
			c := claim(t, b, 1)
			j, err := beginTestAttempt(t, b, c)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.ResolveDispatch(ctx, c, "worker", "uncertain"); err != nil {
				t.Fatal(err)
			}
			for _, part := range []string{"artifact", "logs", "manifest"} {
				u := writeFixture(j)
				u.Part = part
				u.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("output")))
				u.Size = 6
				if err := b.AuthorizeWrite(ctx, u); err != nil {
					t.Fatal(err)
				}
			}
			receipt := b.client.Get(ctx, b.receiptKey(j.SiteID, j.TargetVersion)).Val()
			b.client.Del(ctx, "lock:site:"+j.SiteID)
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("recovery attempted storage write")
				}
				if !materialized {
					w.WriteHeader(404)
					return
				}
				w.Write([]byte("output"))
			}))
			defer storage.Close()
			// Restart recovery must use only durable evidence, not the admitting
			// manager's connection or in-memory launch state.
			replacement, err := NewRedisBackend(b.client.Options().Addr, "", 0)
			if err != nil {
				t.Fatal(err)
			}
			defer replacement.Close()
			replacement.queueKey, replacement.jobKeyPrefix = b.queueKey, b.jobKeyPrefix
			if err := replacement.ProtectReservations(ctx); err != nil {
				t.Fatal(err)
			}
			if err := replacement.InitializeDispatch(ctx, 1); err != nil {
				t.Fatal(err)
			}
			recovery := reconciler.Reconciler{Queue: replacement, Workers: exitedWorker{}, StorageURL: storage.URL, Lifetime: time.Minute}
			if err := recovery.Once(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := b.GetJob(ctx, j.JobID)
			if err != nil {
				t.Fatal(err)
			}
			if materialized && got.Status != types.JobStatusCompleted {
				t.Fatalf("lost completion not recovered: %+v", got)
			}
			if !materialized && (got.Status != types.JobStatusFailed || got.ErrorCode != "artifact_incomplete") {
				t.Fatalf("uncertain materialization not recorded: %+v", got)
			}
			if receipt != b.client.Get(ctx, b.receiptKey(j.SiteID, j.TargetVersion)).Val() {
				t.Fatal("recovery modified immutable receipt")
			}
			if b.client.SCard(ctx, b.queueKey+":active").Val() != 0 || b.client.HExists(ctx, b.queueKey+":sites", j.SiteID).Val() || b.client.Exists(ctx, "lock:site:"+j.SiteID).Val() != 0 {
				t.Fatal("recovery did not release terminal reservations")
			}
			if err := recovery.Once(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
