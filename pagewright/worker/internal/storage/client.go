package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	ctx        context.Context
	attempt    string
}

func (c *Client) WithAttempt(job *types.Job) *Client {
	copy := *c
	data, _ := json.Marshal(map[string]any{"job_id": job.JobID, "site_id": job.SiteID, "owner_id": job.OwnerID, "source_version": job.SourceVersion, "target_version": job.TargetVersion, "lock_token": job.LockToken, "fencing_token": job.FencingToken})
	copy.attempt = string(data)
	return &copy
}

// A per-job copy keeps cancellation scoped without mutating a shared client.
func (c *Client) WithContext(ctx context.Context) *Client {
	copy := *c
	copy.ctx = ctx
	return &copy
}

func (c *Client) request(method, url string, body io.Reader) (*http.Request, error) {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err == nil && method != http.MethodGet && c.attempt != "" {
		req.Header.Set("X-Pagewright-Attempt", c.attempt)
	}
	if err == nil && os.Getenv("PAGEWRIGHT_WORKER_TOKEN") != "" {
		req.Header.Set("Authorization", "Bearer "+os.Getenv("PAGEWRIGHT_WORKER_TOKEN"))
	}
	return req, err
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout:       5 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

var artifactID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

func (c *Client) artifactURL(siteID, versionID string) (string, error) {
	if !artifactID.MatchString(siteID) || !artifactID.MatchString(versionID) {
		return "", fmt.Errorf("site_id and version_id must be 1-255 ASCII identifier characters, starting with a letter or digit")
	}
	return fmt.Sprintf("%s/sites/%s/artifacts/%s", c.baseURL, siteID, versionID), nil
}

// FetchArtifact downloads an artifact to the specified destination
func (c *Client) FetchArtifact(siteID, versionID, destPath string) error {
	url, err := c.artifactURL(siteID, versionID)
	if err != nil {
		return err
	}
	req, err := c.request(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create artifact request: %w", err)
	}
	// A gzip archive is the representation, not HTTP content encoding.
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch artifact: status %d", resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/gzip" {
		return fmt.Errorf("artifact response must have Content-Type application/gzip")
	}
	if encoding := strings.TrimSpace(strings.Join(resp.Header.Values("Content-Encoding"), ",")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return fmt.Errorf("artifact response has unsupported Content-Encoding %q", encoding)
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Download to a sibling so rename publishes only a complete representation.
	out, err := os.CreateTemp(filepath.Dir(destPath), ".artifact-download-*")
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer out.Close()
	defer os.Remove(out.Name())

	// Copy content
	if n, err := io.Copy(out, io.LimitReader(resp.Body, (64<<20)+1)); err != nil {
		return fmt.Errorf("failed to write artifact: %w", err)
	} else if n > 64<<20 {
		return fmt.Errorf("compressed artifact exceeds 64 MiB")
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("failed to close artifact response: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close artifact file: %w", err)
	}
	if err := os.Rename(out.Name(), destPath); err != nil {
		return fmt.Errorf("failed to publish artifact: %w", err)
	}

	return nil
}

// UploadArtifact uploads an artifact file
func (c *Client) UploadArtifact(siteID, versionID, artifactPath string) error {
	url, err := c.artifactURL(siteID, versionID)
	if err != nil {
		return err
	}

	// Open the artifact file
	file, err := os.Open(artifactPath)
	if err != nil {
		return fmt.Errorf("failed to open artifact: %w", err)
	}
	defer file.Close()

	// Stream the file directly; the wire body is exactly the archive bytes.
	req, err := c.request(http.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/gzip")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("failed to upload artifact: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// UploadManifest uploads a manifest JSON file
func (c *Client) UploadManifest(siteID, versionID string, manifest interface{}) error {
	base, err := c.artifactURL(siteID, versionID)
	if err != nil {
		return err
	}
	url := base + "/manifest"

	jsonData, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	req, err := c.request("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("failed to upload manifest: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// UploadLog uploads execution logs
func (c *Client) UploadLog(siteID, versionID, logContent string) error {
	base, err := c.artifactURL(siteID, versionID)
	if err != nil {
		return err
	}
	url := base + "/logs"

	logData := map[string]string{
		"content": logContent,
	}

	jsonData, err := json.Marshal(logData)
	if err != nil {
		return fmt.Errorf("failed to marshal log: %w", err)
	}

	req, err := c.request("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload log: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("failed to upload log: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
