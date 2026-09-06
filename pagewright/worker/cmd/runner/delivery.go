package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

var errDeliveryUncertain = errors.New("result delivery requires manager reconciliation")

// Identical bounded retries; lookup resolves lost acknowledgements and terminal
// 409s without asking the manager to weaken its terminal fencing rule.
func deliverResult(ctx context.Context, managerURL string, result types.JobResult, attempts int, delay time.Duration) error {
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(managerURL, "/") + "/jobs/" + url.PathEscape(result.JobID)
	request := func(method, endpoint string, body []byte) (int, *types.Job, error) {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
		if err != nil || len(data) > 65536 {
			return resp.StatusCode, nil, errDeliveryUncertain
		}
		var job types.Job
		if json.Unmarshal(data, &job) != nil {
			return resp.StatusCode, nil, errDeliveryUncertain
		}
		return resp.StatusCode, &job, nil
	}
	matches := func(j *types.Job) bool {
		return j != nil && j.JobID == result.JobID && j.SiteID == result.SiteID && j.OwnerID == result.OwnerID && j.SourceVersion == result.SourceVersion && j.TargetVersion == result.TargetVersion && j.LockToken == result.LockToken && j.FencingToken == result.FencingToken
	}
	accepted := func(j *types.Job) bool {
		return matches(j) && j.Status == result.Status && j.ManifestPath == result.ManifestPath && j.ErrorMessage == result.ErrorMessage && j.Result == result.Result
	}
	for n := 0; n < attempts; n++ {
		code, job, err := request("POST", endpoint+"/result", payload)
		if err == nil && code == 200 && accepted(job) {
			return nil
		}
		// Even a 200 can have a truncated/missing body. Always look up ambiguity.
		lookupCode, stored, lookupErr := request("GET", endpoint, nil)
		if lookupErr == nil && lookupCode == 200 {
			if accepted(stored) {
				return nil
			}
			if !matches(stored) || stored.Status == "completed" || stored.Status == "failed" {
				return fmt.Errorf("%w: stored outcome differs", errDeliveryUncertain)
			}
		}
		if code >= 400 && code < 500 && code != 408 && code != 429 {
			return fmt.Errorf("%w: callback rejected (%d)", errDeliveryUncertain, code)
		}
		if n+1 < attempts {
			timer := time.NewTimer(delay * time.Duration(1<<n))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return errDeliveryUncertain
}
