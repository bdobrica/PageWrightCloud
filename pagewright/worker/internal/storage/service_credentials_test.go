//go:build integration

package storage

import (
	"net/http"
	"os"
	"strings"
)

type fixtureServiceTransport struct{ base http.RoundTripper }

func (t fixtureServiceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	for _, key := range []string{"TEST_MANAGER_URL", "TEST_STORAGE_URL", "TEST_SERVING_URL"} {
		origin := strings.TrimRight(os.Getenv(key), "/")
		if origin != "" && r.URL.Scheme+"://"+r.URL.Host == origin {
			copy := r.Clone(r.Context())
			copy.Header = r.Header.Clone()
			copy.Header.Set("Authorization", "Bearer "+os.Getenv("PAGEWRIGHT_SERVICE_TOKEN"))
			return t.base.RoundTrip(copy)
		}
	}
	return t.base.RoundTrip(r)
}
func init() { http.DefaultTransport = fixtureServiceTransport{http.DefaultTransport} }
