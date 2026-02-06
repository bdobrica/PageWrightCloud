//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	baseURL = "http://localhost:8080"
	timeout = 30 * time.Second
)

// waitForService waits for the service to be ready
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
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

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

func TestIntegrationStoreAndFetchArtifact(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	waitForService(t)

	siteID := fmt.Sprintf("test-site-%d", time.Now().Unix())
	buildID := "build-123"
	content := []byte("test artifact content for integration test")

	// Store artifact
	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/sites/%s/artifacts/%s", baseURL, siteID, buildID), bytes.NewReader(content))
	require.NoError(t, err)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Fetch artifact
	resp, err = http.Get(fmt.Sprintf("%s/sites/%s/artifacts/%s", baseURL, siteID, buildID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/gzip", resp.Header.Get("Content-Type"))

	fetchedContent, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, content, fetchedContent)
}

func TestIntegrationWriteLogAndListVersions(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	waitForService(t)

	siteID := fmt.Sprintf("test-site-%d", time.Now().Unix())

	// Write multiple log entries
	logEntries := []map[string]interface{}{
		{
			"build_id": "build-1",
			"action":   "build",
			"status":   "success",
			"metadata": map[string]string{"user": "alice"},
		},
		{
			"build_id": "build-2",
			"action":   "deploy",
			"status":   "success",
			"metadata": map[string]string{"user": "bob"},
		},
		{
			"build_id": "build-3",
			"action":   "build",
			"status":   "failed",
			"metadata": map[string]string{"error": "compilation error"},
		},
	}

	client := &http.Client{Timeout: timeout}

	for _, logEntry := range logEntries {
		body, err := json.Marshal(logEntry)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", fmt.Sprintf("%s/sites/%s/logs", baseURL, siteID), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Small delay to ensure timestamps differ
		time.Sleep(10 * time.Millisecond)
	}

	// List versions
	resp, err := http.Get(fmt.Sprintf("%s/sites/%s/versions", baseURL, siteID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, siteID, result["site_id"])
	assert.Equal(t, float64(3), result["count"])

	versions := result["versions"].([]interface{})
	assert.Len(t, versions, 3)

	// Verify versions are sorted (newest first)
	firstVersion := versions[0].(map[string]interface{})
	assert.Equal(t, "build-3", firstVersion["build_id"])
}

func TestIntegrationFetchNonExistentArtifact(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	waitForService(t)

	resp, err := http.Get(fmt.Sprintf("%s/sites/non-existent/artifacts/non-existent", baseURL))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestIntegrationListVersionsForNonExistentSite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	waitForService(t)

	siteID := fmt.Sprintf("non-existent-%d", time.Now().Unix())
	resp, err := http.Get(fmt.Sprintf("%s/sites/%s/versions", baseURL, siteID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, float64(0), result["count"])
	versions := result["versions"].([]interface{})
	assert.Empty(t, versions)
}

func TestIntegrationConcurrentWrites(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	waitForService(t)

	siteID := fmt.Sprintf("test-site-%d", time.Now().Unix())
	numWrites := 10

	// Concurrent artifact uploads
	done := make(chan bool, numWrites)
	for i := 0; i < numWrites; i++ {
		go func(idx int) {
			buildID := fmt.Sprintf("build-%d", idx)
			content := []byte(fmt.Sprintf("content-%d", idx))

			req, err := http.NewRequest("PUT", fmt.Sprintf("%s/sites/%s/artifacts/%s", baseURL, siteID, buildID), bytes.NewReader(content))
			if err != nil {
				done <- false
				return
			}

			client := &http.Client{Timeout: timeout}
			resp, err := client.Do(req)
			if err != nil {
				done <- false
				return
			}
			resp.Body.Close()

			done <- resp.StatusCode == http.StatusCreated
		}(i)
	}

	// Wait for all writes
	successCount := 0
	for i := 0; i < numWrites; i++ {
		if <-done {
			successCount++
		}
	}

	assert.Equal(t, numWrites, successCount, "All concurrent writes should succeed")
}
