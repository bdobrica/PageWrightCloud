package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ServingClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewServingClient(baseURL string) *ServingClient {
	return &ServingClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DeployArtifact deploys an artifact to the serving infrastructure
func (c *ServingClient) DeployArtifact(fqdn, siteID, versionID string) error {
	url := fmt.Sprintf("%s/sites/%s/artifacts", c.baseURL, fqdn)

	req := map[string]string{
		"site_id":    siteID,
		"version_id": versionID,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal deploy request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to deploy artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to deploy artifact: status %d", resp.StatusCode)
	}

	return nil
}

// ActivateVersion activates a version as live
func (c *ServingClient) ActivateVersion(fqdn, versionID string) error {
	url := fmt.Sprintf("%s/sites/%s/activate", c.baseURL, fqdn)

	req := map[string]string{
		"version_id": versionID,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal activate request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to activate version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to activate version: status %d", resp.StatusCode)
	}

	return nil
}

// ActivatePreview activates a version as preview
func (c *ServingClient) ActivatePreview(fqdn, versionID string) error {
	url := fmt.Sprintf("%s/sites/%s/preview", c.baseURL, fqdn)

	req := map[string]string{
		"version_id": versionID,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal preview request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to activate preview: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to activate preview: status %d", resp.StatusCode)
	}

	return nil
}

// EnableSite enables a site
func (c *ServingClient) EnableSite(fqdn string) error {
	url := fmt.Sprintf("%s/sites/%s/enable", c.baseURL, fqdn)

	resp, err := c.httpClient.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to enable site: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to enable site: status %d", resp.StatusCode)
	}

	return nil
}

// DisableSite disables a site
func (c *ServingClient) DisableSite(fqdn string) error {
	url := fmt.Sprintf("%s/sites/%s/disable", c.baseURL, fqdn)

	resp, err := c.httpClient.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to disable site: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to disable site: status %d", resp.StatusCode)
	}

	return nil
}

// UpdateAliases updates the aliases for a site
func (c *ServingClient) UpdateAliases(fqdn string, aliases []string) error {
	url := fmt.Sprintf("%s/sites/%s/aliases", c.baseURL, fqdn)

	req := map[string][]string{
		"aliases": aliases,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal aliases request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to update aliases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update aliases: status %d", resp.StatusCode)
	}

	return nil
}

// DeleteSite deletes a site from serving infrastructure
func (c *ServingClient) DeleteSite(fqdn string) error {
	url := fmt.Sprintf("%s/sites/%s", c.baseURL, fqdn)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete site: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete site: status %d", resp.StatusCode)
	}

	return nil
}
