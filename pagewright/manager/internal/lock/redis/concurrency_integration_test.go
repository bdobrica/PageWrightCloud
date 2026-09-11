//go:build integration

package redis

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConcurrentLeaseOwnershipAndStaleTokens(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR required; use disposable integration stack")
	}
	m, err := NewRedisLockManager(addr, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	site := uuid.NewString()
	defer m.client.Del(context.Background(), lockKeyPrefix+site, fenceKeyPrefix+site)
	type lease struct {
		token string
		fence int64
	}
	winners := make(chan lease, 32)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			token, fence, err := m.Acquire(ctx, site, time.Minute)
			if err == nil {
				winners <- lease{token, fence}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(winners)
	var first lease
	count := 0
	for got := range winners {
		first = got
		count++
	}
	if count != 1 || first.fence != 1 {
		t.Fatalf("expected one winner/fence 1: %d %+v", count, first)
	}
	if err := m.Release(ctx, site, first.token); err != nil {
		t.Fatal(err)
	}
	next, fence, err := m.Acquire(ctx, site, time.Minute)
	if err != nil || fence != first.fence+1 {
		t.Fatalf("next lease: %d %v", fence, err)
	}
	start = make(chan struct{})
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if m.Release(ctx, site, first.token) == nil {
				t.Error("stale release succeeded")
			}
			if m.Renew(ctx, site, first.token, time.Minute) == nil {
				t.Error("stale renewal succeeded")
			}
			if err := m.Renew(ctx, site, next, time.Minute); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := m.client.Get(ctx, lockKeyPrefix+site).Val(); got != next {
		t.Fatal("successor lease lost")
	}
	if err := m.Release(ctx, site, next); err != nil {
		t.Fatal(err)
	}
	if got := m.client.Exists(ctx, lockKeyPrefix+site).Val(); got != 0 {
		t.Fatal("lease not released")
	}
}
