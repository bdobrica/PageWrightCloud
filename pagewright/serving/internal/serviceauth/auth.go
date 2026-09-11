// Package serviceauth implements the internal service and scoped worker boundary.
// Kept identical across independent service modules; scripts/check-service-auth.mjs verifies it.
package serviceauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Scope struct {
	Job     string
	Site    string
	Source  string
	Target  string
	Lock    string
	Fence   int64
	Expires int64
}

func Key() string { return os.Getenv("PAGEWRIGHT_SERVICE_TOKEN") }
func Validate() error {
	if len(Key()) < 32 || strings.TrimSpace(Key()) != Key() {
		return errors.New("PAGEWRIGHT_SERVICE_TOKEN requires an explicit secret of at least 32 bytes")
	}
	for _, name := range []string{"PAGEWRIGHT_PROVIDER_TOKEN", "PAGEWRIGHT_WORKER_LLM_KEY", "PAGEWRIGHT_JWT_SECRET"} {
		if other := os.Getenv(name); other != "" && other == Key() {
			return errors.New("internal service credential must be independent of provider and JWT credentials")
		}
	}
	if password := os.Getenv("PAGEWRIGHT_REDIS_PASSWORD"); password != "" {
		if len(password) < 16 || password == Key() || password == os.Getenv("PAGEWRIGHT_WORKER_LLM_KEY") || password == os.Getenv("PAGEWRIGHT_PROVIDER_TOKEN") {
			return errors.New("Redis requires an independent secret of at least 16 bytes")
		}
	}
	return nil
}
func Sign(key string, s Scope) string {
	if len(key) < 32 {
		return ""
	}
	raw, _ := json.Marshal(s)
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("worker-v1." + body))
	return "worker-v1." + body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func Verify(key, token string) (Scope, bool) {
	var s Scope
	parts := strings.Split(token, ".")
	if len(key) < 32 || len(token) > 4096 || len(parts) != 3 || parts[0] != "worker-v1" {
		return s, false
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return s, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(raw, &s) != nil || s.Job == "" || s.Site == "" || s.Source == "" || s.Target == "" || s.Lock == "" || s.Fence <= 0 || s.Expires <= time.Now().Unix() {
		return Scope{}, false
	}
	return s, true
}
func Allowed(s Scope, service, method, path string) bool {
	job := "/jobs/" + url.PathEscape(s.Job)
	if service == "manager" {
		return (method == "GET" && path == job) || (method == "POST" && (path == job+"/status" || path == job+"/result"))
	}
	if service == "storage" {
		base := "/sites/" + url.PathEscape(s.Site) + "/artifacts/"
		if method == "GET" && path == base+url.PathEscape(s.Source) {
			return true
		}
		target := base + url.PathEscape(s.Target)
		return s.Target != "initial" && ((method == "PUT" && path == target) || (method == "POST" && (path == target+"/logs" || path == target+"/manifest")))
	}
	return false
}

// Wrap gates all internal API reads/writes; only GET /health and GET /ready are public.
// Callback and write bodies still pass the existing identity/fencing validators.
func Wrap(service string, next http.Handler) http.Handler {
	key := Key()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && (r.URL.Path == "/health" || r.URL.Path == "/ready") && r.URL.RawQuery == "" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		r.Header.Del("X-Pagewright-Verified-Job")
		r.Header.Del("X-Pagewright-Verified-Lock")
		r.Header.Del("X-Pagewright-Verified-Fence")
		header := r.Header.Get("Authorization")
		if len(key) >= 32 && subtle.ConstantTimeCompare([]byte(header), []byte("Bearer "+key)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		s, ok := Verify(key, token)
		if !strings.HasPrefix(header, "Bearer ") || !ok {
			http.Error(w, "internal authentication required", 401)
			return
		}
		if r.URL.RawQuery != "" || !Allowed(s, service, r.Method, r.URL.EscapedPath()) {
			http.Error(w, "worker scope denied", 403)
			return
		}
		// Bind callback/attempt identity to signed claims, not just the URL.
		r.Header.Set("X-Pagewright-Verified-Job", s.Job)
		r.Header.Set("X-Pagewright-Verified-Lock", s.Lock)
		r.Header.Set("X-Pagewright-Verified-Fence", fmtFence(s.Fence))
		next.ServeHTTP(w, r)
	})
}
func fmtFence(f int64) string { b, _ := json.Marshal(f); return string(b) }

type Transport struct {
	Base          http.RoundTripper
	Origin, Token string
}

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	u, err := url.Parse(t.Origin)
	if err != nil || u.Scheme != r.URL.Scheme || u.Host != r.URL.Host {
		return nil, errors.New("internal credential destination mismatch")
	}
	copy := r.Clone(r.Context())
	copy.Header = r.Header.Clone()
	if t.Token != "" {
		copy.Header.Set("Authorization", "Bearer "+t.Token)
	}
	return base.RoundTrip(copy)
}
