//go:build integration

package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

func writeFixture(j *types.Job) *types.WriteCommit {
	return &types.WriteCommit{JobID: j.JobID, SiteID: j.SiteID, OwnerID: j.OwnerID, SourceVersion: j.SourceVersion, TargetVersion: j.TargetVersion, LockToken: j.LockToken, FencingToken: j.FencingToken, Part: "artifact", SHA256: strings.Repeat("a", 64), Size: 10}
}

func TestFencedWriteAndTerminalCommit(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	enqueue(t, b, base)
	j, err := beginTestAttempt(t, b, claim(t, b, 1))
	if err != nil {
		t.Fatal(err)
	}
	u := writeFixture(j)
	for _, mutate := range []func(*types.WriteCommit){
		func(u *types.WriteCommit) { u.LockToken = "old-attempt" },
		func(u *types.WriteCommit) { u.FencingToken++ },
		func(u *types.WriteCommit) { u.OwnerID = "other-owner" },
		func(u *types.WriteCommit) { u.TargetVersion = "other-target" },
		func(u *types.WriteCommit) { u.SourceVersion = "other-source" },
		func(u *types.WriteCommit) { u.Part = "manifest" },
	} {
		bad := *u
		mutate(&bad)
		if err := b.AuthorizeWrite(ctx, &bad); !errors.Is(err, queue.ErrFenced) {
			t.Fatalf("bad commit accepted: %v", err)
		}
	}
	if err := b.AuthorizeWrite(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := b.AuthorizeWrite(ctx, u); err != nil {
		t.Fatalf("identical active object retry: %v", err)
	}
	bad := *u
	bad.SHA256 = strings.Repeat("b", 64)
	if err := b.AuthorizeWrite(ctx, &bad); !errors.Is(err, queue.ErrFenced) {
		t.Fatalf("different bytes accepted: %v", err)
	}
	completed := *j
	completed.Status = types.JobStatusCompleted
	completed.ManifestPath = "/sites/" + j.SiteID + "/artifacts/" + j.TargetVersion + "/manifest"
	if err := b.UpdateJob(ctx, &completed); !errors.Is(err, queue.ErrFenced) {
		t.Fatalf("completed without manifest grant: %v", err)
	}
	// Match the existing artifact reservation, then approve remaining parts.
	for _, part := range []string{"logs", "manifest"} {
		u.Part = part
		if err := b.AuthorizeWrite(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.UpdateJob(ctx, &completed); err != nil {
		t.Fatal(err)
	}
	before := b.client.Get(ctx, b.jobKeyPrefix+j.JobID).Val()
	for _, update := range []*types.Job{&completed, j} {
		if err := b.UpdateJob(ctx, update); !errors.Is(err, queue.ErrFenced) {
			t.Fatalf("terminal duplicate/regression accepted: %v", err)
		}
	}
	if before != b.client.Get(ctx, b.jobKeyPrefix+j.JobID).Val() {
		t.Fatal("terminal snapshot changed")
	}
	if b.client.Exists(ctx, "lock:site:"+j.SiteID).Val() != 0 || b.client.SCard(ctx, b.queueKey+":active").Val() != 0 {
		t.Fatal("atomic terminal release failed")
	}
	if err := b.AuthorizeWrite(ctx, u); !errors.Is(err, queue.ErrFenced) {
		t.Fatalf("post-terminal write accepted: %v", err)
	}
}

func TestConcurrentJobsOutliveOriginalLockTTL(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	locks := newDispatchLocks(t, b)
	const ttl = 500 * time.Millisecond
	var jobs []*types.Job
	for i := 0; i < 2; i++ {
		enqueue(t, b, base)
		c := claim(t, b, 2)
		token, fence, err := locks.Acquire(ctx, c.Job.SiteID, ttl)
		if err != nil {
			t.Fatal(err)
		}
		j, err := b.BeginDispatch(ctx, c, token, fence)
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, j)
	}
	renewCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); b.MaintainLeases(renewCtx, ttl, 50*time.Millisecond, 5*time.Second) }()
	t.Cleanup(func() { stop(); <-done })
	// Two concurrently running jobs cross more than two original TTLs.
	time.Sleep(1200 * time.Millisecond)
	for _, j := range jobs {
		if got := b.client.Get(ctx, "lock:site:"+j.SiteID).Val(); got != j.LockToken {
			t.Fatalf("long job lost lease: %s", j.JobID)
		}
		if _, _, err := locks.Acquire(ctx, j.SiteID, ttl); err == nil {
			t.Fatal("competing worker acquired renewed lease")
		}
		if err := b.AuthorizeWrite(ctx, writeFixture(j)); err != nil {
			t.Fatal(err)
		}
	}
	stop()
	<-done
	// Do not allow a restarted renewer to resurrect an expired lease.
	time.Sleep(650 * time.Millisecond)
	if err := b.RenewActiveLocks(ctx, ttl, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if b.client.Exists(ctx, "lock:site:"+j.SiteID).Val() != 0 {
			t.Fatal("expired lease resurrected")
		}
		if err := b.AuthorizeWrite(ctx, writeFixture(j)); !errors.Is(err, queue.ErrFenced) {
			t.Fatalf("expired write accepted: %v", err)
		}
		j.Status = types.JobStatusFailed
		j.ErrorMessage = "late failure"
		if err := b.UpdateJob(ctx, j); !errors.Is(err, queue.ErrFenced) {
			t.Fatalf("expired callback accepted: %v", err)
		}
		_, nextFence, err := locks.Acquire(ctx, j.SiteID, ttl)
		if err != nil || nextFence <= j.FencingToken {
			t.Fatalf("fence not monotonic: %d %v", nextFence, err)
		}
		if err := b.AuthorizeWrite(ctx, writeFixture(j)); !errors.Is(err, queue.ErrFenced) {
			t.Fatalf("superseded attempt accepted: %v", err)
		}
	}
}
