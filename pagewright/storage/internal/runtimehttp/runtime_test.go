package runtimehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBudgetPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	Budget(time.Hour, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !errors.Is(r.Context().Err(), context.Canceled) {
			t.Fatal("lost cancellation")
		}
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil).WithContext(ctx))
	Budget(time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
			t.Error("deadline missing")
		}
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}
func TestReadyFailureRecoveryAndLiveness(t *testing.T) {
	var failed atomic.Bool
	ready := Ready(func(context.Context) error {
		if failed.Load() {
			return errors.New("private credential")
		}
		return nil
	})
	for _, status := range []int{200, 503, 200} {
		failed.Store(status == 503)
		w := httptest.NewRecorder()
		ready(w, httptest.NewRequest("GET", "/ready", nil))
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("response: %d %s", w.Code, w.Body)
		}
	}
	for _, r := range []*http.Request{httptest.NewRequest("POST", "/ready", nil), httptest.NewRequest("GET", "/ready?x=1", nil)} {
		w := httptest.NewRecorder()
		ready(w, r)
		if w.Code != 404 {
			t.Fatal(w.Code)
		}
	}
}
func TestReadyDeadlineCancelsDependency(t *testing.T) {
	exited := make(chan struct{})
	ready := Ready(func(ctx context.Context) error { <-ctx.Done(); close(exited); return ctx.Err() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	ready(w, httptest.NewRequest("GET", "/ready", nil).WithContext(ctx))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("dependency not cancelled")
	}
}
func TestHTTPProbeDoesNotFollowRedirects(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Redirect(w, r, "/secret", 302) }))
	defer s.Close()
	if HTTP(s.URL)(context.Background()) == nil || calls.Load() != 1 {
		t.Fatal("redirect followed")
	}
}
func TestDirectory(t *testing.T) {
	if err := Directory(t.TempDir())(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := Directory("/does-not-exist-pagewright")(context.Background()); err == nil {
		t.Fatal("missing volume ready")
	}
}
