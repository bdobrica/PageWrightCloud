package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

func TestDeliveryAmbiguityAndBounds(t *testing.T) {
	for _, scenario := range []string{"lost_ack", "duplicate", "retry", "exhausted", "wrong_attempt", "different_terminal", "oversized", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			j := contractJob()
			result := types.JobResult{JobID: j.JobID, SiteID: j.SiteID, OwnerID: j.OwnerID, SourceVersion: j.SourceVersion, TargetVersion: j.TargetVersion, LockToken: j.LockToken, FencingToken: j.FencingToken, Status: "failed", ErrorMessage: "execution failed"}
			terminal := j
			terminal.Status = "failed"
			terminal.ErrorMessage = result.ErrorMessage
			var posts, gets atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					n := posts.Add(1)
					var got types.JobResult
					if json.NewDecoder(r.Body).Decode(&got) != nil || got != result {
						t.Error("retry changed payload")
					}
					switch scenario {
					case "lost_ack":
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						conn.Close()
					case "duplicate":
						w.WriteHeader(409)
					case "retry":
						if n == 2 {
							json.NewEncoder(w).Encode(terminal)
						} else {
							w.WriteHeader(503)
						}
					case "oversized":
						w.Write(make([]byte, 65537))
					case "redirect":
						w.Header().Set("Location", "/unexpected")
						w.WriteHeader(307)
					default:
						w.WriteHeader(503)
					}
					return
				}
				gets.Add(1)
				if r.URL.Path == "/unexpected" {
					t.Error("followed redirect")
				}
				switch scenario {
				case "lost_ack", "duplicate":
					json.NewEncoder(w).Encode(terminal)
				case "wrong_attempt":
					terminal.FencingToken++
					json.NewEncoder(w).Encode(terminal)
				case "different_terminal":
					terminal.ErrorMessage = "other failure"
					json.NewEncoder(w).Encode(terminal)
				default:
					json.NewEncoder(w).Encode(j)
				}
			}))
			defer srv.Close()
			err := deliverResult(context.Background(), srv.URL, result, 3, time.Millisecond)
			wantSuccess := scenario == "lost_ack" || scenario == "duplicate" || scenario == "retry"
			if (err == nil) != wantSuccess {
				t.Fatalf("result %v", err)
			}
			want := int32(3)
			if scenario == "lost_ack" || scenario == "duplicate" || scenario == "wrong_attempt" || scenario == "different_terminal" {
				want = 1
			}
			if scenario == "retry" {
				want = 2
			}
			if posts.Load() != want || gets.Load() > 3 {
				t.Fatalf("unbounded or incorrect attempts: %d %d", posts.Load(), gets.Load())
			}
		})
	}
}

func TestDeliveryDeadline(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer srv.Close()
	defer close(done)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := deliverResult(ctx, srv.URL, types.JobResult{}, 4, time.Second); err == nil {
		t.Fatal("cancellation ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("delivery exceeded deadline")
	}
}
