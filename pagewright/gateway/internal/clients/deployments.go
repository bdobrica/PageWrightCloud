package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
)

func (c *ServingClient) ApplyDeployment(ctx context.Context, d *database.Deployment) (string, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/sites/"+url.PathEscape(d.FQDN)+"/deployment", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deployment receipt unavailable: %d", resp.StatusCode)
	}
	var receipt database.Deployment
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4097))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return "", err
	}
	if decoder.Decode(new(any)) != io.EOF || receipt.SiteID != d.SiteID || receipt.FQDN != d.FQDN || receipt.Sequence != d.Sequence || receipt.Version != d.Version || receipt.Target != d.Target || (receipt.Status != "completed" && receipt.Status != "failed") {
		return "", fmt.Errorf("invalid deployment receipt")
	}
	return receipt.Status, nil
}
