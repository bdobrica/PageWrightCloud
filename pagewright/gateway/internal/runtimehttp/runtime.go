// Package runtimehttp bounds request work and separates readiness from liveness.
package runtimehttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

type Check func(context.Context) error

// Budget propagates a deadline; it does not claim a timed-out mutation rolled back.
func Budget(limit time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), limit)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Ready runs only explicitly registered, bounded checks. No dependency details,
// credentials, provider calls, or mutating application requests are exposed.
func Ready(checks ...Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" || r.URL.RawQuery != "" {
			http.Error(w, "not found", 404)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		results := make(chan error, len(checks))
		for _, check := range checks {
			go func(c Check) { results <- c(ctx) }(check)
		}
		for range checks {
			select {
			case err := <-results:
				if err != nil {
					http.Error(w, "not ready", 503)
					return
				}
			case <-ctx.Done():
				http.Error(w, "not ready", 503)
				return
			}
		}
		if ctx.Err() != nil {
			http.Error(w, "not ready", 503)
			return
		}
		w.Write([]byte("ready"))
	}
}

func Transport() *http.Transport {
	return &http.Transport{Proxy: http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second, IdleConnTimeout: 60 * time.Second,
		MaxIdleConns: 32, MaxIdleConnsPerHost: 8, MaxConnsPerHost: 16}
}

var probeClient = &http.Client{Transport: Transport(), Timeout: 2 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func HTTP(endpoint string) Check {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return err
		}
		resp, err := probeClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return errors.New("dependency unavailable")
		}
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return err
	}
}

// Directory checks the already configured local volume without modifying it.
// It is not a guarantee of free disk space or a future successful write.
func Directory(path string) Check {
	return func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("not a directory")
		}
		_, err = f.Readdirnames(1)
		if err == io.EOF {
			return nil
		}
		return err
	}
}
