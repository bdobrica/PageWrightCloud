package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginConfiguration(t *testing.T) {
	for _, raw := range []string{"", "*", "null", "https://*.pagewright.io", "https://app.pagewright.io/", "https://user:pass@app.pagewright.io", "https://app.pagewright.io?x", "https://app.pagewright.io?", "https://app.pagewright.io#x", "https://demo.pagewright.io", "https://preview.demo.pagewright.io", "http://localhost:99999", "http://localhost:0", "http://localhost:03000", "http://bad..test"} {
		if _, err := OriginPolicy(raw, "pagewright.io"); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	if _, err := OriginPolicy("https://app.pagewright.io,https://api.pagewright.io,http://localhost:3000", "pagewright.io"); err != nil {
		t.Fatal(err)
	}
}

func TestOriginDenialBeforeSideEffects(t *testing.T) {
	policy, err := OriginPolicy("https://app.pagewright.io", "pagewright.io")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	h := policy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	for _, origin := range []string{"https://evil.test", "null", "", "https://app.pagewright.io.evil.test", "https://demo.pagewright.io", "https://preview.demo.pagewright.io", "http://app.pagewright.io", "https://app.pagewright.io:444"} {
		for _, method := range []string{"POST", "GET", "OPTIONS"} {
			r := httptest.NewRequest(method, "http://api.pagewright.io/ws", nil)
			r.Header.Set("Origin", origin)
			r.Header.Set("Authorization", "Bearer must-not-reach-handler")
			r.Header.Set("X-Forwarded-Host", "app.pagewright.io")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("accepted %q: %d", origin, w.Code)
			}
		}
	}
	if calls != 0 {
		t.Fatal("rejected request reached handler")
	}
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal("non-browser request denied")
	}
	r.Header.Set("Origin", "https://app.pagewright.io")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "https://app.pagewright.io" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("allowed origin policy wrong")
	}
	r.Header.Add("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("duplicate Origin accepted")
	}
}

func TestPreflightPolicy(t *testing.T) {
	for _, header := range []string{"Authorization, Content-Type, Idempotency-Key", "X-Admin"} {
		r := httptest.NewRequest("OPTIONS", "/not-a-route", nil)
		r.Header.Set("Origin", "http://localhost:3000")
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", header)
		w := httptest.NewRecorder()
		CORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("preflight reached handler") })).ServeHTTP(w, r)
		want := 200
		if header == "X-Admin" {
			want = 403
		}
		if w.Code != want || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("wrong preflight %d", w.Code)
		}
	}
}
