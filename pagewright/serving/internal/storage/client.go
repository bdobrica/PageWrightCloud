package storage

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/serviceauth"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Transport:     serviceauth.Transport{Origin: baseURL, Token: serviceauth.Key()},
			Timeout:       5 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

var artifactID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

// FetchArtifact downloads an artifact to the specified destination
func (c *Client) FetchArtifact(siteID, versionID, destPath string) error {
	if !artifactID.MatchString(siteID) || !artifactID.MatchString(versionID) {
		return fmt.Errorf("invalid artifact site or version identifier")
	}
	url := fmt.Sprintf("%s/sites/%s/artifacts/%s", c.baseURL, siteID, versionID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create artifact request: %w", err)
	}
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
		return fmt.Errorf("artifact response must have application/gzip media type")
	}
	if encoding := strings.TrimSpace(strings.Join(resp.Header.Values("Content-Encoding"), ",")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return fmt.Errorf("unsupported artifact content encoding %q", encoding)
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create destination file
	out, err := os.CreateTemp(filepath.Dir(destPath), ".artifact-*")
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer out.Close()
	defer os.Remove(out.Name())

	// Copy content
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write artifact: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("close artifact response: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close artifact file: %w", err)
	}
	if err := os.Rename(out.Name(), destPath); err != nil {
		return fmt.Errorf("commit artifact file: %w", err)
	}

	return nil
}
