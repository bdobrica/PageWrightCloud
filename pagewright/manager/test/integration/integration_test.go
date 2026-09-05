//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const timeout = 30 * time.Second

var baseURL string

func TestMain(m *testing.M) {
	baseURL = os.Getenv("TEST_MANAGER_URL")
	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "TEST_MANAGER_URL is required; use the dedicated integration test stack")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func waitForService(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	t.Fatal("Service did not become ready in time")
}

func TestIntegrationHealthCheck(t *testing.T) {
	waitForService(t)

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]string
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "healthy", result["status"])
}

func TestIntegrationCreateAndGetJob(t *testing.T) {
	waitForService(t)

	siteID := fmt.Sprintf("test-site-%d", time.Now().UnixNano())

	// Create job
	jobReq := types.JobRequest{
		SiteID:        siteID,
		OwnerID:       "test-owner",
		Prompt:        "Update the homepage title",
		SourceVersion: "v1",
		TargetVersion: "v2",
	}

	jsonData, err := json.Marshal(jobReq)
	require.NoError(t, err)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(baseURL+"/jobs", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var job types.Job
	err = json.NewDecoder(resp.Body).Decode(&job)
	require.NoError(t, err)

	assert.NotEmpty(t, job.JobID)
	assert.Equal(t, siteID, job.SiteID)
	assert.Equal(t, "test-owner", job.OwnerID)
	assert.Equal(t, "Update the homepage title", job.Prompt)
	assert.Equal(t, "v1", job.SourceVersion)
	assert.Equal(t, "v2", job.TargetVersion)
	assert.Equal(t, types.JobStatusRunning, job.Status)
	assert.NotEmpty(t, job.LockToken)
	assert.Greater(t, job.FencingToken, int64(0))

	// Get job
	resp, err = client.Get(baseURL + "/jobs/" + job.JobID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var fetchedJob types.Job
	err = json.NewDecoder(resp.Body).Decode(&fetchedJob)
	require.NoError(t, err)

	assert.Equal(t, job.JobID, fetchedJob.JobID)
	assert.Equal(t, job.SiteID, fetchedJob.SiteID)
}

func TestIntegrationUpdateJobStatus(t *testing.T) {
	waitForService(t)

	siteID := fmt.Sprintf("test-site-%d", time.Now().UnixNano())

	// Create job
	jobReq := types.JobRequest{
		SiteID:        siteID,
		OwnerID:       "test-owner",
		SourceVersion: "v1",
		Prompt:        "Add contact page",
	}

	jsonData, err := json.Marshal(jobReq)
	require.NoError(t, err)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(baseURL+"/jobs", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)

	var job types.Job
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&job))
	resp.Body.Close()

	// Update job status
	statusUpdate := types.JobStatusUpdate{
		JobID:         job.JobID,
		SiteID:        job.SiteID,
		OwnerID:       job.OwnerID,
		SourceVersion: job.SourceVersion,
		TargetVersion: job.TargetVersion,
		Status:        types.JobStatusCompleted,
		Result:        "Contact page added successfully",
	}

	jsonData, err = json.Marshal(statusUpdate)
	require.NoError(t, err)

	resp, err = client.Post(baseURL+"/jobs/"+job.JobID+"/status", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updatedJob types.Job
	err = json.NewDecoder(resp.Body).Decode(&updatedJob)
	require.NoError(t, err)

	assert.Equal(t, types.JobStatusCompleted, updatedJob.Status)
	assert.Equal(t, "Contact page added successfully", updatedJob.Result)
}

func TestIntegrationLockPreventsMultipleJobs(t *testing.T) {
	waitForService(t)

	siteID := fmt.Sprintf("test-site-%d", time.Now().UnixNano())

	// Create first job
	jobReq := types.JobRequest{
		SiteID:        siteID,
		OwnerID:       "test-owner",
		SourceVersion: "v1",
		Prompt:        "First job",
	}

	jsonData, err := json.Marshal(jobReq)
	require.NoError(t, err)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(baseURL+"/jobs", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Try to create second job for same site (should fail due to lock)
	jobReq.Prompt = "Second job"
	jsonData, err = json.Marshal(jobReq)
	require.NoError(t, err)

	resp, err = client.Post(baseURL+"/jobs", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestIntegrationJobWithoutTargetVersion(t *testing.T) {
	waitForService(t)

	siteID := fmt.Sprintf("test-site-%d", time.Now().UnixNano())

	// Target version may be omitted; source identity is required.
	jobReq := types.JobRequest{
		SiteID:        siteID,
		OwnerID:       "test-owner",
		SourceVersion: "v1",
		Prompt:        "Create new page",
	}

	jsonData, err := json.Marshal(jobReq)
	require.NoError(t, err)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(baseURL+"/jobs", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var job types.Job
	err = json.NewDecoder(resp.Body).Decode(&job)
	require.NoError(t, err)

	// Target version should be auto-generated
	assert.NotEmpty(t, job.TargetVersion)
}
