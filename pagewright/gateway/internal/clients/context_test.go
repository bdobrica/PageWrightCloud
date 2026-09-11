package clients

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownstreamCancellation(t *testing.T) {
	for _, name := range []string{"evaluation", "instructions", "bootstrap", "versions", "serving", "manager", "stream"} {
		t.Run(name, func(t *testing.T) {
			entered, exited := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.Copy(io.Discard, r.Body)
				if name == "stream" {
					w.Header().Set("Content-Type", "application/gzip")
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				close(entered)
				<-r.Context().Done()
				close(exited)
			}))
			defer s.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				switch name {
				case "evaluation":
					_, err = NewLLMClient("fixture", s.URL).EvaluateRequestContext(ctx, "test")
				case "instructions":
					_, err = NewLLMClient("fixture", s.URL).GenerateJobInstructionsContext(ctx, "test", "")
				case "bootstrap":
					err = NewStorageClient(s.URL).WithContext(ctx).InitializeSource("site", "initial", []byte("archive"), nil, nil)
				case "versions":
					_, err = NewStorageClient(s.URL).ListVersionsContext(ctx, "site")
				case "serving":
					err = NewServingClient(s.URL).WithContext(ctx).EnableSite("example.test")
				case "manager":
					_, err = NewManagerClient(s.URL).EnqueueJobContext(ctx, ManagerJobRequest{})
				case "stream":
					var body io.ReadCloser
					body, err = NewStorageClient(s.URL).WithContext(ctx).OpenArtifact("site", "v1")
					if err == nil {
						_, err = io.Copy(io.Discard, body)
						body.Close()
					}
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("request never arrived")
			}
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Error("cancelled request succeeded")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("client ignored cancellation")
			}
			select {
			case <-exited:
			case <-time.After(2 * time.Second):
				t.Fatal("server did not observe cancellation")
			}
			if calls.Load() != 1 {
				t.Fatal("unexpected retry")
			}
		})
	}
}

func TestContextualClientRetainsOwnTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	c := contextualClient(&http.Client{Timeout: 20 * time.Millisecond}, context.Background())
	start := time.Now()
	_, err := c.Get(s.URL)
	if err == nil || time.Since(start) > time.Second {
		t.Fatal("client timeout lost", err)
	}
}
