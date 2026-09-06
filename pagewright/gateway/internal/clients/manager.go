package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

type ManagerClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewManagerClient(baseURL string) *ManagerClient {
	return &ManagerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			// A redirect could deliver POST before a later dial failure, making
			// that error unsafe to classify as definitely not submitted.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// EnqueueJob submits a new job to the manager
func (c *ManagerClient) EnqueueJob(req ManagerJobRequest) (*ManagerJobResponse, error) {
	return c.EnqueueJobContext(context.Background(), req)
}

func (c *ManagerClient) EnqueueJobContext(ctx context.Context, req ManagerJobRequest) (*ManagerJobResponse, error) {
	url := fmt.Sprintf("%s/jobs", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal job request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, managerHTTPError(resp)
	}

	var result ManagerJobResponse
	if err := decodeManagerJob(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode job response: %w", err)
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manager job response: %w", err)
	}
	if (req.JobID != "" && result.JobID != req.JobID) || result.SiteID != req.SiteID || result.OwnerID != req.OwnerID || result.SourceVersion != req.SourceVersion || result.Prompt != req.Prompt || (req.TargetVersion != "" && result.TargetVersion != req.TargetVersion) {
		return nil, fmt.Errorf("manager response job association mismatch")
	}

	return &result, nil
}

// GetJobStatus retrieves the status of a job
func (c *ManagerClient) GetJobStatus(jobID string) (*ManagerJobStatus, error) {
	return c.GetJobStatusContext(context.Background(), jobID)
}

func (c *ManagerClient) GetJobStatusContext(ctx context.Context, jobID string) (*ManagerJobStatus, error) {
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	url := fmt.Sprintf("%s/jobs/%s", c.baseURL, url.PathEscape(jobID))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to get job status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, managerHTTPError(resp)
	}

	var status ManagerJobStatus
	if err := decodeManagerJob(resp.Body, &status); err != nil {
		return nil, fmt.Errorf("failed to decode job status: %w", err)
	}
	if err := status.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manager job status: %w", err)
	}
	if status.JobID != jobID {
		return nil, fmt.Errorf("manager response job_id mismatch")
	}

	return &status, nil
}

func decodeManagerJob(body io.Reader, job *types.Job) error {
	data, err := io.ReadAll(io.LimitReader(body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("manager response too large")
	}
	return json.Unmarshal(data, job)
}

// Aliases keep the gateway's wire definitions in one place.
type ManagerJobRequest = types.ManagerJobRequest
type ManagerJobResponse = types.Job
type ManagerJobStatus = types.Job

type ManagerError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *ManagerError) Error() string {
	return fmt.Sprintf("manager status %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

func managerHTTPError(resp *http.Response) error {
	result := &ManagerError{StatusCode: resp.StatusCode, Code: "invalid_response", Message: http.StatusText(resp.StatusCode)}
	var envelope types.ErrorResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope); err == nil && envelope.Error != "" {
		result.Code, result.Message = envelope.Error, envelope.Message
	}
	return result
}
