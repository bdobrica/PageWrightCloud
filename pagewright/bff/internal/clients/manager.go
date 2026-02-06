package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
		},
	}
}

// EnqueueJob submits a new job to the manager
func (c *ManagerClient) EnqueueJob(req ManagerJobRequest) (*ManagerJobResponse, error) {
	url := fmt.Sprintf("%s/jobs", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal job request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to enqueue job: status %d", resp.StatusCode)
	}

	var result ManagerJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode job response: %w", err)
	}

	return &result, nil
}

// GetJobStatus retrieves the status of a job
func (c *ManagerClient) GetJobStatus(jobID string) (*ManagerJobStatus, error) {
	url := fmt.Sprintf("%s/jobs/%s", c.baseURL, jobID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get job status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get job status: status %d", resp.StatusCode)
	}

	var status ManagerJobStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode job status: %w", err)
	}

	return &status, nil
}

type ManagerJobRequest struct {
	SiteID          string            `json:"site_id"`
	BaseBuildID     string            `json:"base_build_id"`
	RequestedAction string            `json:"requested_action"`
	UserText        string            `json:"user_text"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type ManagerJobResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

type ManagerJobStatus struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	BuildID   string    `json:"build_id,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
