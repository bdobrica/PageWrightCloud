//go:build integration

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestLegacyTTLProtectionAndSafeRetention(t *testing.T) {
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
	approveParts(t, b, j)
	receiptKey := b.receiptKey(j.SiteID, j.TargetVersion)
	before := b.client.Get(ctx, receiptKey).Val()
	for _, key := range []string{b.jobKeyPrefix + j.JobID, receiptKey, "fence:site:" + j.SiteID, b.queueKey + ":active"} {
		b.client.Expire(ctx, key, time.Minute)
	}
	if err := b.ProtectReservations(ctx); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{b.jobKeyPrefix + j.JobID, receiptKey, "fence:site:" + j.SiteID, b.queueKey + ":active"} {
		if b.client.PTTL(ctx, key).Val() != -1 {
			t.Fatalf("reservation still expires: %s", key)
		}
	}
	if b.client.PTTL(ctx, "lock:site:"+j.SiteID).Val() <= 0 {
		t.Fatal("lease made permanent")
	}
	terminal(t, b, j.JobID)
	// Exercise age cutoff with an explicit test clock; never expire canonical data.
	var cursor uint64
	for {
		next, err := b.TrimHistoryMetadata(ctx, cursor, time.Now().Add(31*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if b.client.HExists(ctx, b.queueKey+":tokens", j.JobID).Val() || b.client.HExists(ctx, b.queueKey+":states", j.JobID).Val() {
		t.Fatal("terminal dispatch metadata retained")
	}
	if before != b.client.Get(ctx, receiptKey).Val() {
		t.Fatal("receipt changed")
	}
	replay, created, err := b.CreateJob(ctx, c.Job)
	if err != nil || created || replay.JobID != j.JobID || string(replay.Status) != "completed" {
		t.Fatalf("retention lost deduplication: %+v %v", replay, err)
	}
	if b.client.LLen(ctx, b.queueKey).Val() != 0 {
		t.Fatal("retained job replayed")
	}
}

func TestAuditIsReadOnlyAndFindsMissingEvidence(t *testing.T) {
	b, base := isolatedBackend(t)
	ctx := context.Background()
	j := enqueue(t, b, base)
	before := b.client.Get(ctx, b.jobKeyPrefix+j.JobID).Val()
	report, err := b.Audit(ctx)
	if err != nil || len(report.Issues) != 0 {
		t.Fatalf("healthy audit: %+v %v", report, err)
	}
	b.client.LRem(ctx, b.queueKey, 0, j.JobID)
	report, err = b.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "pending_without_queue_or_claim" {
		t.Fatalf("missing index not identified: %+v", report)
	}
	data, _ := json.Marshal(report)
	if string(data) == "" || before != b.client.Get(ctx, b.jobKeyPrefix+j.JobID).Val() {
		t.Fatal("audit mutated job")
	}
	if b.client.LLen(ctx, b.queueKey).Val() != 0 {
		t.Fatal("audit requeued job")
	}
	b.client.HSet(ctx, b.queueKey+":sites", "orphan-site", "missing-job")
	report, err = b.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range report.Issues {
		if issue.Code == "site_reserved_job_missing" && issue.JobID == "missing-job" {
			found = true
		}
	}
	if !found {
		t.Fatal("orphan site reservation not identified")
	}
}

func TestDurabilityRejectsEphemeralRedis(t *testing.T) {
	b, _ := isolatedBackend(t)
	// The ordinary protocol-test Redis deliberately has AOF disabled.
	if err := b.ValidateDurability(context.Background()); err == nil {
		t.Fatal("ephemeral Redis passed production durability gate")
	}
}
