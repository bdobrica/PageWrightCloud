//go:build integration

package redis

import (
	"context"
	"testing"
	"time"
)

func TestReplacementManagerPreservesDispatchEvidence(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	if err := b.InitializeDispatch(ctx, 3); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		enqueue(t, b, base)
	}
	originals := map[string]string{}
	for _, outcome := range []string{"started", "uncertain"} {
		c := claim(t, b, 3)
		j, err := beginTestAttempt(t, b, c)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.ResolveDispatch(ctx, c, "worker-"+outcome, outcome); err != nil {
			t.Fatal(err)
		}
		approveParts(t, b, j)
		originals[j.JobID] = b.client.Get(ctx, b.receiptKey(j.SiteID, j.TargetVersion)).Val()
	}
	// A new connection and backend stand in for a replacement process; no local
	// claim/spawner state is transferred. Never flush the shared test Redis.
	replacement, err := NewRedisBackend(b.client.Options().Addr, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	replacement.queueKey = b.queueKey
	replacement.jobKeyPrefix = b.jobKeyPrefix
	if err := replacement.ProtectReservations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := replacement.InitializeDispatch(ctx, 3); err != nil {
		t.Fatal(err)
	}
	rows, err := replacement.Candidates(ctx, time.Hour)
	if err != nil || len(rows) != 2 {
		t.Fatalf("attempt recovery after reconnect: %v %v", rows, err)
	}
	for _, row := range rows {
		if row.Receipt != originals[row.Job.JobID] || row.Expired {
			t.Fatal("reconnect changed attempt/receipt")
		}
	}
	if err := replacement.RenewActiveLocks(ctx, time.Minute, time.Hour); err != nil {
		t.Fatal(err)
	}
	pending := claim(t, replacement, 3)
	if _, replayed := originals[pending.Job.JobID]; replayed {
		t.Fatal("running intent requeued")
	}
	if replacement.client.LLen(ctx, replacement.queueKey).Val() != 0 {
		t.Fatal("unexpected duplicate queue entries")
	}
	if replacement.client.SCard(ctx, replacement.queueKey+":active").Val() != 3 {
		t.Fatal("restart lost capacity reservations")
	}
	report, err := replacement.Audit(ctx)
	if err != nil || len(report.Issues) != 0 {
		t.Fatalf("reconnected state inconsistent: %+v %v", report, err)
	}
}
