// Package reconciler resolves lost worker callbacks without replaying execution
// or granting new write authority. Redis owns the final compare-and-swap.
package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

type Candidate struct {
	Job     types.Job
	Receipt string
	Expired bool
}
type Backend interface {
	Candidates(context.Context, time.Duration) ([]Candidate, error)
	Recover(context.Context, Candidate, string, string, string) error
}
type Reconciler struct {
	Queue      Backend
	Workers    spawner.Inspector
	StorageURL string
	Lifetime   time.Duration
}

func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := r.Once(ctx); err != nil && ctx.Err() == nil {
			log.Print("Worker reconciliation pending; private dependency diagnostics withheld")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Reconciler) Once(ctx context.Context) error {
	listing, cancel := context.WithTimeout(ctx, 5*time.Second)
	candidates, err := r.Queue.Candidates(listing, r.Lifetime)
	cancel()
	// A slow/broken candidate does not prevent independent attempts from recovery.
	first := err
	var mu sync.Mutex
	var workers sync.WaitGroup
	slots := make(chan struct{}, 4)
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			workers.Wait()
			return ctx.Err()
		case slots <- struct{}{}:
		}
		workers.Add(1)
		go func(candidate Candidate) {
			defer workers.Done()
			defer func() { <-slots }()
			operation, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			if err := r.reconcile(operation, candidate); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}(candidate)
	}
	workers.Wait()
	return first
}

func (r *Reconciler) reconcile(ctx context.Context, c Candidate) error {
	state, inspectErr := r.Workers.Inspect(ctx, &c.Job)
	if !c.Expired {
		if inspectErr != nil {
			return inspectErr
		}
		if !state.Exited {
			return nil
		}
	}
	// Timeout revokes the attempt even if the daemon is unavailable. A verified
	// running worker is killed by immutable ID; orphan retention is M2.10.
	if c.Expired && inspectErr == nil && state.Running {
		killCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := r.Workers.Kill(killCtx, state.ID)
		cancel()
		if err != nil {
			log.Print("Timed-out worker kill unconfirmed; terminal fencing still required")
		}
	}
	complete, err := r.materialized(ctx, c)
	if err != nil {
		return err
	} // Storage/transport outage is not proof of missing data.
	status, code, message := "failed", "worker_exit", "Worker exited without a committed result"
	if state.OOMKilled {
		code = "worker_oom"
		message = "Worker exceeded its memory limit"
	}
	if c.Receipt != "" {
		code = "artifact_incomplete"
		message = "Reserved output was not fully materialized; receipts retained"
	}
	if c.Expired {
		code = "worker_timeout"
		message = "Worker lifetime exceeded without a committed result"
	}
	if complete {
		status = "completed"
		code = ""
		message = ""
	}
	return r.Queue.Recover(ctx, c, status, code, message)
}

func (r *Reconciler) materialized(ctx context.Context, c Candidate) (bool, error) {
	if c.Receipt == "" {
		return false, nil
	}
	var receipt struct {
		JobID                    string `json:"job_id"`
		LockToken                string `json:"lock_token"`
		Fence                    int64  `json:"fencing_token"`
		Artifact, Logs, Manifest string
	}
	if err := json.Unmarshal([]byte(c.Receipt), &receipt); err != nil {
		return false, fmt.Errorf("invalid recovery receipt")
	}
	if receipt.JobID != c.Job.JobID || receipt.LockToken != c.Job.LockToken || receipt.Fence != c.Job.FencingToken {
		return false, fmt.Errorf("receipt attempt mismatch")
	}
	if receipt.Artifact == "" || receipt.Logs == "" || receipt.Manifest == "" {
		return false, nil
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableCompression: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	base := strings.TrimRight(r.StorageURL, "/") + "/sites/" + url.PathEscape(c.Job.SiteID) + "/artifacts/" + url.PathEscape(c.Job.TargetVersion)
	for _, part := range []struct {
		suffix, fingerprint string
		limit               int64
	}{{"", receipt.Artifact, 64 << 20}, {"/logs", receipt.Logs, 4 << 20}, {"/manifest", receipt.Manifest, 4 << 20}} {
		fields := strings.Split(part.fingerprint, ":")
		if len(fields) != 2 {
			return false, fmt.Errorf("invalid receipt fingerprint")
		}
		size, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || size < 0 || size > part.limit {
			return false, fmt.Errorf("invalid receipt size")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", base+part.suffix, nil)
		if err != nil {
			return false, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return false, fmt.Errorf("artifact observation unavailable")
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			if resp.StatusCode == 404 {
				return false, nil
			}
			return false, fmt.Errorf("artifact observation unavailable (%d)", resp.StatusCode)
		}
		hash := sha256.New()
		count, err := io.Copy(hash, io.LimitReader(resp.Body, size+1))
		resp.Body.Close()
		if err != nil {
			return false, fmt.Errorf("artifact observation interrupted")
		}
		if count != size || fmt.Sprintf("%x", hash.Sum(nil)) != fields[0] {
			return false, nil
		}
	}
	return true, nil
}
