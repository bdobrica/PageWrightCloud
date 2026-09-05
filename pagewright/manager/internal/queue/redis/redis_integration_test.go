//go:build integration

package redis

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
)

func isolatedBackend(t *testing.T) (*RedisBackend, *types.Job) {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR is required; use the dedicated integration stack")
	}
	backend, err := NewRedisBackend(addr, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Unique keys only: never flush a database or touch the manager's queue.
	prefix := "pagewright:contract-test:" + uuid.NewString() + ":"
	backend.queueKey = prefix + "queue"
	backend.jobKeyPrefix = prefix + "job:"
	job := &types.Job{JobID: uuid.NewString(), SiteID: "site", OwnerID: "owner", Prompt: "edit", SourceVersion: "v1", TargetVersion: "v2", Status: types.JobStatusPending, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	t.Cleanup(func() {
		backend.client.Del(context.Background(), backend.queueKey, backend.jobKeyPrefix+job.JobID)
		backend.Close()
	})
	return backend, job
}

func TestRedisAtomicReservationAndConcurrentReplay(t *testing.T) {
	b, j := isolatedBackend(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var createdCount atomic.Int32
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stored, created, err := b.CreateJob(ctx, j)
			if err != nil {
				t.Error(err)
				return
			}
			if stored.JobID != j.JobID || stored.TargetVersion != j.TargetVersion {
				t.Error("identity lost")
			}
			if created {
				createdCount.Add(1)
			}
		}()
	}
	wg.Wait()
	if createdCount.Load() != 1 {
		t.Fatalf("created %d times", createdCount.Load())
	}
	if n, err := b.client.LLen(ctx, b.queueKey).Result(); err != nil || n != 1 {
		t.Fatalf("queue length %d: %v", n, err)
	}
	if ttl, err := b.client.TTL(ctx, b.jobKeyPrefix+j.JobID).Result(); err != nil || ttl != -1 {
		t.Fatalf("reservation must not expire: %v %v", ttl, err)
	}
	// A lost response does not lose the reservation or enqueue again.
	stored, created, err := b.CreateJob(ctx, j)
	if err != nil || created || stored.JobID != j.JobID {
		t.Fatalf("lost response retry: %+v %v %v", stored, created, err)
	}
	popped, err := b.Pop(ctx)
	if err != nil || popped.JobID != j.JobID {
		t.Fatalf("pop: %+v %v", popped, err)
	}
	if n := b.client.LLen(ctx, b.queueKey).Val(); n != 0 {
		t.Fatalf("duplicate queue entry: %d", n)
	}
}

func TestRedisQueueTypeFailureDoesNotReserveJob(t *testing.T) {
	b, j := isolatedBackend(t)
	ctx := context.Background()
	if err := b.client.Set(ctx, b.queueKey, "not-a-list", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.CreateJob(ctx, j); err == nil {
		t.Fatal("invalid queue accepted")
	}
	if n := b.client.Exists(ctx, b.jobKeyPrefix+j.JobID).Val(); n != 0 {
		t.Fatal("partial reservation on Lua error")
	}
	b.client.Del(ctx, b.queueKey)
	if _, created, err := b.CreateJob(ctx, j); err != nil || !created {
		t.Fatalf("retry after repair: %v %v", created, err)
	}
	// Existing job replay must not attempt another queue append even if its queue broke.
	b.client.Del(ctx, b.queueKey)
	b.client.Set(ctx, b.queueKey, "not-a-list", 0)
	if _, created, err := b.CreateJob(ctx, j); err != nil || created {
		t.Fatalf("existing reservation replay: %v %v", created, err)
	}
}

func TestRedisUpdatesPreserveTTLAndDoNotResurrectJobs(t *testing.T) {
	b, j := isolatedBackend(t)
	ctx := context.Background()
	if err := b.UpdateJob(ctx, j); !errors.Is(err, queue.ErrJobNotFound) {
		t.Fatalf("missing update: %v", err)
	}
	if _, err := b.SetWorkerID(ctx, j.JobID, "worker"); !errors.Is(err, queue.ErrJobNotFound) {
		t.Fatalf("missing metadata: %v", err)
	}
	if _, err := b.GetJob(ctx, j.JobID); !errors.Is(err, queue.ErrJobNotFound) {
		t.Fatalf("missing lookup: %v", err)
	}
	if _, _, err := b.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	j.Status = types.JobStatusCompleted
	j.Result = "done"
	j.ManifestPath = "manifest"
	if err := b.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if ttl := b.client.TTL(ctx, b.jobKeyPrefix+j.JobID).Val(); ttl != -1 {
		t.Fatalf("update introduced expiry: %v", ttl)
	}
	if err := b.client.Expire(ctx, b.jobKeyPrefix+j.JobID, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if err := b.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	got, err := b.SetWorkerID(ctx, j.JobID, "worker")
	if err != nil || got.Status != types.JobStatusCompleted || got.ManifestPath != "manifest" || got.WorkerID != "worker" {
		t.Fatalf("outcome overwritten: %+v %v", got, err)
	}
	if ttl := b.client.TTL(ctx, b.jobKeyPrefix+j.JobID).Val(); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("existing expiry replaced: %v", ttl)
	}
	b.client.Del(ctx, b.jobKeyPrefix+j.JobID)
	if err := b.UpdateJob(ctx, j); !errors.Is(err, queue.ErrJobNotFound) {
		t.Fatalf("deleted update: %v", err)
	}
	if b.client.Exists(ctx, b.jobKeyPrefix+j.JobID).Val() != 0 {
		t.Fatal("deleted job resurrected")
	}
}

func TestRedisBackendFailureIsNotJobNotFound(t *testing.T) {
	b, j := isolatedBackend(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.GetJob(ctx, j.JobID); err == nil || errors.Is(err, queue.ErrJobNotFound) {
		t.Fatalf("backend error misclassified: %v", err)
	}
}
