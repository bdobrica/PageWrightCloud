package clients

import (
	"context"
	"io"
	"net/http"
)

// Request-local clients retain the original transport, timeout and redirect
// policy. The context lives through streamed response consumption, not just headers.
type contextualTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t contextualTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	// Retain http.Client's own deadline as well as the caller's cancellation.
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	if t.ctx.Err() != nil {
		cancel()
	}
	cleanup := func() { stop(); cancel() }
	resp, err := base.RoundTrip(r.Clone(ctx))
	if err != nil {
		cleanup()
		return nil, err
	}
	resp.Body = &contextBody{ReadCloser: resp.Body, cleanup: cleanup}
	return resp, nil
}

type contextBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *contextBody) Close() error { defer b.cleanup(); return b.ReadCloser.Close() }
func contextualClient(client *http.Client, ctx context.Context) *http.Client {
	c := *client
	c.Transport = contextualTransport{ctx: ctx, base: client.Transport}
	return &c
}
func (c *StorageClient) WithContext(ctx context.Context) *StorageClient {
	copy := *c
	copy.httpClient = contextualClient(c.httpClient, ctx)
	return &copy
}
func (c *ServingClient) WithContext(ctx context.Context) *ServingClient {
	copy := *c
	copy.httpClient = contextualClient(c.httpClient, ctx)
	return &copy
}
func (c *StorageClient) ListVersionsContext(ctx context.Context, site string) ([]StorageVersion, error) {
	return c.WithContext(ctx).ListVersions(site)
}
