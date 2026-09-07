package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// CORS is the explicit local fixture policy. Production constructs its policy
// from configuration once, before database initialization.
func CORS(next http.Handler) http.Handler {
	policy, _ := OriginPolicy("http://localhost:3000", "pagewright.dev")
	return policy(next)
}

// OriginPolicy never trusts request Host/forwarding headers or wildcard suffixes.
func OriginPolicy(raw, siteDomain string) (func(http.Handler) http.Handler, error) {
	allowed := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.String() != value || !regexp.MustCompile(`^[a-z0-9.-]+(?::[0-9]{1,5})?$`).MatchString(u.Host) {
			return nil, fmt.Errorf("invalid PAGEWRIGHT_APP_ORIGINS; require explicit HTTP(S) origins")
		}
		domain := strings.ToLower(strings.TrimSpace(siteDomain))
		host := u.Hostname()
		if len(host) > 253 {
			return nil, fmt.Errorf("invalid application origin hostname")
		}
		for _, label := range strings.Split(host, ".") {
			if !regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`).MatchString(label) {
				return nil, fmt.Errorf("invalid application origin hostname")
			}
		}
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
				return nil, fmt.Errorf("invalid application origin port")
			}
		}
		if strings.HasSuffix(host, "."+domain) {
			label := strings.TrimSuffix(host, "."+domain)
			if label != "app" && label != "api" && label != "www" {
				return nil, fmt.Errorf("application origin overlaps generated-site namespace")
			}
		}
		allowed[value] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
			w.Header().Set("Cache-Control", "no-store")
			origins := r.Header.Values("Origin")
			origin := r.Header.Get("Origin")
			if len(origins) > 1 || (len(origins) == 1 && !allowed[origin]) {
				http.Error(w, "browser origin denied", http.StatusForbidden)
				return
			}
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			if r.Method == http.MethodOptions {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				method := r.Header.Get("Access-Control-Request-Method")
				if method != "" && method != "GET" && method != "POST" && method != "PUT" && method != "DELETE" {
					http.Error(w, "preflight method denied", 403)
					return
				}
				for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
					switch strings.ToLower(strings.TrimSpace(header)) {
					case "", "authorization", "content-type", "idempotency-key":
					default:
						http.Error(w, "preflight header denied", 403)
						return
					}
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// JSON response helper
func respondJSON(w http.ResponseWriter, data interface{}) error {
	return json.NewEncoder(w).Encode(data)
}
