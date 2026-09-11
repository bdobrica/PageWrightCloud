//go:build integration

package database

import (
	"context"
	"testing"
	"time"
)

func TestBoundedPoolAndRequestCancellation(t *testing.T) {
	db := migrationDB(t)
	if got := db.Stats().MaxOpenConnections; got != 10 {
		t.Fatalf("pool limit %d", got)
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	var value int
	err = db.WithContext(ctx).Get(&value, "SELECT 1")
	cancel()
	conn.Close()
	if err == nil {
		t.Fatal("pool wait ignored deadline")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err = db.WithContext(ctx).Exec("SELECT pg_sleep(5)"); err == nil || time.Since(start) > time.Second {
		t.Fatal("query did not cancel", err)
	}
	if err = db.Get(&value, "SELECT 1"); err != nil || value != 1 {
		t.Fatal("request context contaminated shared pool", err)
	}
}
