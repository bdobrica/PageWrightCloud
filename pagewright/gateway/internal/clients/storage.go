package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type StorageClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewStorageClient(baseURL string) *StorageClient {
	return &StorageClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// FetchArtifact downloads an artifact tar.gz file
func (c *StorageClient) FetchArtifact(siteID, versionID string) ([]byte, error) {
	reader, err := c.OpenArtifact(siteID, versionID)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

var storageID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

func (c *StorageClient) artifactURL(siteID, versionID string) (string, error) {
	if !storageID.MatchString(siteID) || !storageID.MatchString(versionID) {
		return "", fmt.Errorf("invalid storage site or version ID")
	}
	return fmt.Sprintf("%s/sites/%s/artifacts/%s", c.baseURL, siteID, versionID), nil
}

// OpenArtifact streams the gzip file unchanged. Caller must close the reader.
func (c *StorageClient) OpenArtifact(siteID, versionID string) (io.ReadCloser, error) {
	url, err := c.artifactURL(siteID, versionID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// This is a compressed file, not HTTP Content-Encoding. Avoid Go's automatic
	// gzip negotiation/decompression so all clients see the identical archive.
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch artifact: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to fetch artifact: status %d", resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	encoding := strings.TrimSpace(strings.Join(resp.Header.Values("Content-Encoding"), ","))
	if err != nil || mediaType != "application/gzip" || (encoding != "" && !strings.EqualFold(encoding, "identity")) {
		resp.Body.Close()
		return nil, fmt.Errorf("invalid storage artifact media type or content encoding")
	}
	return resp.Body, nil
}

// ListVersions retrieves all versions for a site from storage service
func (c *StorageClient) ListVersions(siteID string) ([]StorageVersion, error) {
	if !storageID.MatchString(siteID) {
		return nil, fmt.Errorf("invalid storage site ID")
	}
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
	// Route aligned, but storage DELETE semantics remain M1.5. Never treat a
	// missing/unsupported endpoint as successful deletion.
	url, err := c.artifactURL(siteID, versionID)
	if err != nil {
		return err
	}

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
