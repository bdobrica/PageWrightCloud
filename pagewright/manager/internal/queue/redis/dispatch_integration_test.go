//go:build integration

package redis

import (
	"context"
	"encoding/json"
	"errors"
	lockRedis "github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/lock/redis"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/dispatcher"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

type dispatchLocks struct {
	released atomic.Int32
	real     *lockRedis.RedisLockManager
	b        *RedisBackend
	t        *testing.T
}

func newDispatchLocks(t *testing.T, b *RedisBackend) *dispatchLocks {
	l, err := lockRedis.NewRedisLockManager(b.client.Options().Addr, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return &dispatchLocks{real: l, b: b, t: t}
}
func (l *dispatchLocks) Acquire(ctx context.Context, site string, ttl time.Duration) (string, int64, error) {
	l.t.Cleanup(func() { l.b.client.Del(context.Background(), "lock:site:"+site, "fence:site:"+site) })
	return l.real.Acquire(ctx, site, ttl)
}
func (l *dispatchLocks) Release(ctx context.Context, site, token string) error {
	l.released.Add(1)
	return l.real.Release(ctx, site, token)
}
func (l *dispatchLocks) Renew(context.Context, string, string, time.Duration) error { return nil }
func (l *dispatchLocks) Close() error                                               { return nil }

type dispatchSpawner func(context.Context, *types.Job, string) (string, error)

func (s dispatchSpawner) Spawn(c context.Context, j *types.Job, u string) (string, error) {
	return s(c, j, u)
}
func (s dispatchSpawner) Close() error { return nil }

func enqueue(t *testing.T, b *RedisBackend, base *types.Job) *types.Job {
	t.Helper()
	j := *base
	j.JobID = uuid.NewString()
	j.SiteID = uuid.NewString()
	j.TargetVersion = uuid.NewString()
	got, _, err := b.CreateJob(context.Background(), &j)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func claim(t *testing.T, b *RedisBackend, limit int) *queue.Claim {
	t.Helper()
	c, err := b.Claim(context.Background(), limit, time.Second)
	if err != nil || c == nil {
		t.Fatalf("claim: %+v %v", c, err)
	}
	return c
}
func terminal(t *testing.T, b *RedisBackend, id string) {
	t.Helper()
	j, err := b.GetJob(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	j.Status = types.JobStatusCompleted
	j.Result = "done"
	j.ManifestPath = "/sites/" + j.SiteID + "/artifacts/" + j.TargetVersion + "/manifest"
	approveParts(t, b, j)
	if err := b.UpdateJob(context.Background(), j); err != nil {
		t.Fatal(err)
	}
}

func approveParts(t *testing.T, b *RedisBackend, j *types.Job) {
	t.Helper()
	for _, part := range []string{"artifact", "logs", "manifest"} {
		u := &types.WriteCommit{JobID: j.JobID, SiteID: j.SiteID, OwnerID: j.OwnerID, SourceVersion: j.SourceVersion, TargetVersion: j.TargetVersion, LockToken: j.LockToken, FencingToken: j.FencingToken, Part: part, SHA256: strings.Repeat("a", 64), Size: 1}
		if err := b.AuthorizeWrite(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
}

func beginTestAttempt(t *testing.T, b *RedisBackend, c *queue.Claim) (*types.Job, error) {
	t.Helper()
	token, fence, err := newDispatchLocks(t, b).Acquire(context.Background(), c.Job.SiteID, time.Minute)
	if err != nil {
		return nil, err
	}
	return b.BeginDispatch(context.Background(), c, token, fence)
}

func TestDispatchClaimRecoveryAndIntentBoundary(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	if err := b.InitializeDispatch(ctx, 1); err != nil {
		t.Fatal(err)
	}
	j := enqueue(t, b, base)
	first := claim(t, b, 1)
	// Simulate a crashed manager; Redis's authoritative deadline expires.
	b.client.ZAdd(ctx, b.queueKey+":claims", goredis.Z{Score: 0, Member: j.JobID})
	recovered := claim(t, b, 1)
	if recovered.Token == first.Token || recovered.Job.JobID != j.JobID {
		t.Fatal("claim not recovered")
	}
	if _, err := b.BeginDispatch(ctx, first, "old-lock", 1); !errors.Is(err, queue.ErrClaimLost) {
		t.Fatalf("stale owner retained authority: %v", err)
	}
	running, err := beginTestAttempt(t, b, recovered)
	if err != nil || running.Status != types.JobStatusRunning {
		t.Fatalf("begin %v %v", running, err)
	}
	// Crash after intent: even a restarted dispatcher may not launch again.
	if err := b.InitializeDispatch(ctx, 1); err != nil {
		t.Fatal(err)
	}
	enqueue(t, b, base)
	if c, err := b.Claim(ctx, 1, time.Second); err != nil || c != nil {
		t.Fatalf("intent was redispatched: %v %v", c, err)
	}
	terminal(t, b, j.JobID)
	claim(t, b, 1)
}

func TestDispatchFastCallbackAndSpawnOutcomes(t *testing.T) {
	for _, outcome := range []string{"started", "not_started", "uncertain", "fast_callback"} {
		t.Run(outcome, func(t *testing.T) {
			b, base := isolatedBackend(t)
			j := enqueue(t, b, base)
			c := claim(t, b, 1)
			locks := newDispatchLocks(t, b)
			d := &dispatcher.Dispatcher{Queue: b, Locks: locks, LockTTL: time.Minute, Spawner: dispatchSpawner(func(_ context.Context, job *types.Job, _ string) (string, error) {
				if outcome == "fast_callback" {
					terminal(t, b, job.JobID)
				}
				if outcome == "not_started" {
					return "", spawner.ErrNotStarted
				}
				if outcome == "uncertain" {
					return "container", errors.New("lost acknowledgement")
				}
				return "container", nil
			})}
			d.Process(context.Background(), c)
			stored, err := b.GetJob(context.Background(), j.JobID)
			if err != nil {
				t.Fatal(err)
			}
			active := b.client.SCard(context.Background(), b.queueKey+":active").Val()
			if outcome == "not_started" {
				if stored.Status != types.JobStatusFailed || stored.ErrorCode != "spawn_failed" || active != 0 || locks.released.Load() != 0 || b.client.Exists(context.Background(), "lock:site:"+stored.SiteID).Val() != 0 {
					t.Fatalf("bad rejection: %+v active=%d", stored, active)
				}
			} else if outcome == "fast_callback" {
				if stored.Status != types.JobStatusCompleted || stored.WorkerID != "container" || active != 0 {
					t.Fatalf("callback overwritten: %+v", stored)
				}
			} else {
				if stored.Status != types.JobStatusRunning || stored.WorkerID != "container" || active != 1 || locks.released.Load() != 0 {
					t.Fatalf("lost running slot: %+v", stored)
				}
			}
			// Reprocessing an old claim never invokes the spawner twice.
			d.Spawner = dispatchSpawner(func(context.Context, *types.Job, string) (string, error) {
				t.Error("duplicate execution")
				return "", nil
			})
			d.Process(context.Background(), c)
		})
	}
}

func TestDispatchGlobalLimitAcrossReplicasAndRestart(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	if err := b.InitializeDispatch(ctx, 2); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		enqueue(t, b, base)
	}
	started := make(chan string, 10)
	var mu sync.Mutex
	seen := map[string]bool{}
	spawn := dispatchSpawner(func(_ context.Context, j *types.Job, _ string) (string, error) {
		mu.Lock()
		if seen[j.JobID] {
			t.Error("duplicate launch")
		}
		seen[j.JobID] = true
		mu.Unlock()
		started <- j.JobID
		return "worker-" + j.JobID, nil
	})
	start := func() (context.CancelFunc, chan struct{}) {
		replica, err := NewRedisBackend(os.Getenv("TEST_REDIS_ADDR"), "", 0)
		if err != nil {
			t.Fatal(err)
		}
		replica.queueKey, replica.jobKeyPrefix = b.queueKey, b.jobKeyPrefix
		run, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		d := &dispatcher.Dispatcher{Queue: replica, Locks: newDispatchLocks(t, replica), Spawner: spawn, Limit: 2, Lease: time.Second, LockTTL: time.Minute}
		go func() { defer close(done); defer replica.Close(); d.Run(run) }()
		return cancel, done
	}
	take := func() string {
		t.Helper()
		select {
		case id := <-started:
			return id
		case <-time.After(5 * time.Second):
			t.Fatal("no dispatch")
			return ""
		}
	}
	cancel1, done1 := start()
	cancel2, done2 := start()
	defer func() { cancel1(); cancel2(); <-done1; <-done2 }()
	first := take()
	take()
	select {
	case <-started:
		t.Fatal("exceeded global live-job limit")
	case <-time.After(250 * time.Millisecond):
	}
	cancel1()
	<-done1
	cancel3, done3 := start()
	defer func() { cancel3(); <-done3 }()
	select {
	case <-started:
		t.Fatal("restart forgot active slots")
	case <-time.After(250 * time.Millisecond):
	}
	terminal(t, b, first)
	take()
	if n := b.client.SCard(ctx, b.queueKey+":active").Val(); n != 2 {
		t.Fatalf("active count=%d", n)
	}
	if _, err := b.Claim(ctx, 3, time.Second); err == nil {
		t.Fatal("replicas accepted inconsistent limits")
	}
}

func TestQueuedSiteAdmissionAndTerminalMerge(t *testing.T) {
	b, j := isolatedBackend(t)
	ctx := context.Background()
	b.CreateJob(ctx, j)
	other := *j
	other.JobID = uuid.NewString()
	got, _, err := b.CreateJob(ctx, &other)
	if err != nil || got.ErrorCode != "job_busy" || got.Status != types.JobStatusFailed {
		t.Fatalf("pending site not guarded: %+v %v", got, err)
	}
	c := claim(t, b, 1)
	running, err := beginTestAttempt(t, b, c)
	if err != nil {
		t.Fatal(err)
	}
	terminal(t, b, j.JobID)
	if err := b.UpdateJob(ctx, running); err == nil {
		t.Fatal("late running callback reopened terminal slot")
	}
	if n := b.client.SCard(ctx, b.queueKey+":active").Val(); n != 0 {
		t.Fatal("terminal capacity not freed")
	}
	other.JobID = uuid.NewString()
	got, _, err = b.CreateJob(ctx, &other)
	if err != nil || got.Status != types.JobStatusPending {
		t.Fatalf("site not released: %+v %v", got, err)
	}
}

func TestDispatchMigratesLegacyQueueWithoutRelaunch(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	jobs := []types.Job{}
	for _, status := range []types.JobStatus{types.JobStatusPending, types.JobStatusRunning, types.JobStatusCompleted} {
		j := *base
		j.JobID = uuid.NewString()
		j.SiteID = uuid.NewString()
		j.Status = status
		raw, _ := json.Marshal(j)
		if err := b.client.Set(ctx, b.jobKeyPrefix+j.JobID, raw, 0).Err(); err != nil {
			t.Fatal(err)
		}
		if err := b.client.RPush(ctx, b.queueKey, j.JobID).Err(); err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, j)
	}
	if err := b.InitializeDispatch(ctx, 2); err != nil {
		t.Fatal(err)
	}
	c := claim(t, b, 2)
	if c.Job.JobID != jobs[0].JobID {
		t.Fatal("legacy running or terminal job relaunched")
	}
	if err := b.InitializeDispatch(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if n := b.client.SCard(ctx, b.queueKey+":active").Val(); n != 2 {
		t.Fatalf("restart lost capacity: %d", n)
	}
	if _, err := b.GetJob(ctx, jobs[2].JobID); err != nil {
		t.Fatal("migration deleted history")
	}
	if c, err := b.Claim(ctx, 2, time.Second); err != nil || c != nil {
		t.Fatal("migration duplicated dispatch")
	}
}

func TestDispatchCorruptMetadataDoesNotLoseQueuedJob(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	j := enqueue(t, b, base)
	if err := b.client.Set(ctx, b.queueKey+":active", "bad type", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Claim(ctx, 1, time.Second); err == nil {
		t.Fatal("accepted corrupt metadata")
	}
	if n := b.client.LLen(ctx, b.queueKey).Val(); n != 1 {
		t.Fatal("lost queued ID")
	}
	if got, err := b.GetJob(ctx, j.JobID); err != nil || got.Status != types.JobStatusPending {
		t.Fatal("lost reservation")
	}
}
