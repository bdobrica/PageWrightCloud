package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type StorageClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewStorageClient(baseURL string) *StorageClient {
	return &StorageClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchArtifact downloads an artifact tar.gz file
func (c *StorageClient) FetchArtifact(siteID, versionID string) ([]byte, error) {
	url := fmt.Sprintf("%s/artifacts/%s/%s", c.baseURL, siteID, versionID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch artifact: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// ListVersions retrieves all versions for a site from storage service
func (c *StorageClient) ListVersions(siteID string) ([]StorageVersion, error) {
	url := fmt.Sprintf("%s/sites/%s/versions", c.baseURL, siteID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list versions: status %d", resp.StatusCode)
	}

	var result struct {
		Versions []StorageVersion `json:"versions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode versions response: %w", err)
	}

	return result.Versions, nil
}

// DeleteVersion deletes a version from storage
func (c *StorageClient) DeleteVersion(siteID, versionID string) error {
	url := fmt.Sprintf("%s/artifacts/%s/%s", c.baseURL, siteID, versionID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete version: status %d", resp.StatusCode)
	}

	return nil
}

type StorageVersion struct {
	BuildID   string    `json:"build_id"`
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size"`
}
