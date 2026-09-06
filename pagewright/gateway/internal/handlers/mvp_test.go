package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
)

func TestMVPDisabledActionsHaveNoDependenciesOrRedirects(t *testing.T) {
	aliases := NewAliasesHandler(nil, nil)
	authHandler := NewAuthHandler(nil, nil, nil)
	sites := NewSitesHandler(nil, nil, nil, 25)
	for name, handler := range map[string]http.HandlerFunc{
		"list aliases": aliases.ListAliases, "add alias": aliases.AddAlias,
		"delete alias": aliases.DeleteAlias, "google login": authHandler.GoogleLogin,
		"google callback": authHandler.GoogleCallback,
		"site deletion":   sites.DeleteSite,
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/?code=unused&state=unused", strings.NewReader("{}"))
			w := httptest.NewRecorder()
			handler(w, r) // nil dependencies must never be touched.
			if w.Code != 501 || w.Header().Get("Cache-Control") != "no-store" ||
				w.Header().Get("Location") != "" || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("unexpected disabled response: %d %v", w.Code, w.Header())
			}
		})
	}
}

func TestMVPDomainPolicy(t *testing.T) {
	h := NewSitesHandler(nil, nil, nil, 25)
	if err := h.SetSiteDomain(" EXAMPLE.TEST "); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.example.test", "a-b.example.test", "1.example.test"} {
		if !h.supportedSiteName(name) {
			t.Fatal(name)
		}
	}
	for _, name := range []string{
		"example.test", "outside.test", "a.example.test.evil", "aexample.test",
		"nested.one.example.test", "preview.example.test", "app.example.test",
		"api.example.test", "www.example.test", "-a.example.test", "a-.example.test",
		"a.example.test.", "a..example.test", "../one.example.test",
		strings.Repeat("a", 64) + ".example.test",
	} {
		t.Run(name, func(t *testing.T) {
			if h.supportedSiteName(name) {
				t.Fatal("accepted unsupported name")
			}
			body, _ := json.Marshal(map[string]string{"fqdn": name, "template_id": "starter"})
			w := httptest.NewRecorder()
			h.CreateSite(w, httptest.NewRequest("POST", "/sites", strings.NewReader(string(body))))
			if w.Code != 400 {
				t.Fatalf("got %d", w.Code)
			} // before nil DB/storage
		})
	}
	for _, domain := range []string{"", "localhost", "https://example.test", "example.test:8084", "a..test", "*.test", "preview.example.test"} {
		if h.SetSiteDomain(domain) == nil {
			t.Fatalf("accepted config %q", domain)
		}
	}
	w := httptest.NewRecorder()
	h.Capabilities(w, httptest.NewRequest("GET", "/capabilities", nil))
	var got map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["site_domain"] != "example.test" || got["mode"] != "mvp" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(got)
	}
	for _, key := range []string{"attachments", "custom_domains", "aliases", "oauth", "site_deletion"} {
		if got[key] != false {
			t.Fatal(key)
		}
	}
}

func TestMVPBuildRejectsAttachmentsBeforeSideEffects(t *testing.T) {
	h := NewBuildHandler(nil, nil, nil, nil)
	for _, tc := range []struct {
		media, body string
		status      int
	}{
		{"multipart/form-data; boundary=test", "upload", 415},
		{"multipart/form-data; boundary=test", "{\"message\":\"edit\"}", 415},
		{"application/octet-stream", "upload", 415},
		{"text/plain", "{\"message\":\"edit\"}", 415},
		{"application/json; broken", "{}", 415},
		{"application/json", "{\"message\":\"edit\",\"files\":[]}", 400},
		{"application/json", "{\"message\":\"edit\",\"attachments\":[]}", 400},
		{"application/json", "{\"message\":\"  \"}", 400},
		{"application/json; charset=utf-8", "{}", 400},
		{"", "{}", 400},
	} {
		r := httptest.NewRequest("POST", "/sites/one.example.test/build", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.media)
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &auth.Claims{UserID: "owner"}))
		w := httptest.NewRecorder()
		h.Build(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: got %d", tc.media, tc.body, w.Code)
		}
	}
}
