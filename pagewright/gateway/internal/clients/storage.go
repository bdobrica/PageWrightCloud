package clients

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var ErrBootstrapConflict = errors.New("initial source conflicts with stored bytes")

// InitializeSource sends the persisted bootstrap bytes, never reserializing JSON
// on retry. The final manifest is the immutable storage completion record.
func (c *StorageClient) InitializeSource(siteID, versionID string, archive, log, manifest []byte) error {
	base, err := c.artifactURL(siteID, versionID)
	if err != nil {
		return err
	}
	for _, part := range []struct {
		method, suffix, media string
		data                  []byte
	}{
		{"PUT", "", "application/gzip", archive},
		{"POST", "/logs", "application/json", log},
		{"POST", "/manifest", "application/json", manifest},
	} {
		req, err := http.NewRequest(part.method, base+part.suffix, bytes.NewReader(part.data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", part.media)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusConflict {
			return ErrBootstrapConflict
		}
		if resp.StatusCode != 201 {
			return fmt.Errorf("bootstrap storage write %s returned %d", part.suffix, resp.StatusCode)
		}
	}
	return nil
}

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

type StorageVersion struct {
	BuildID   string    `json:"build_id"`
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size"`
}
