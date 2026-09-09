//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func TestPreviewDeploymentOrderFailureAndOwnership(t *testing.T) {
	user, err := testDB.CreateUser(uuid.NewString()+"@preview.test", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	site, err := testDB.CreateSite(user.ID, uuid.NewString()+".preview.test", "starter")
	if err != nil {
		t.Fatal(err)
	}
	token, err := testJWTManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := testJWTManager.GenerateToken(uuid.NewString(), "other@preview.test")
	if err != nil {
		t.Fatal(err)
	}
	live := "unchanged-live"
	if err := testDB.UpdateSiteVersions(site.FQDN, &live, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                                   string
		artifactStatus, activateStatus, status int
		auth                                   string
		calls                                  int
	}{
		{"anonymous", 200, 200, 401, "", 0}, {"foreign", 200, 200, 403, foreign, 0},
		{"tls-provisioning", 200, 200, 503, token, 0},
		{"artifact-failure", 500, 200, 500, token, 1}, {"activation-failure", 200, 500, 500, token, 1},
		{"success", 200, 200, 200, token, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := []string{}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["version"] != "draft" {
					t.Errorf("invalid serving payload %v %v", payload, err)
				}
				if tc.activateStatus != 200 {
					w.WriteHeader(tc.activateStatus)
					return
				}
				payload["status"] = "completed"
				if tc.artifactStatus != 200 {
					payload["status"] = "failed"
				}
				json.NewEncoder(w).Encode(payload)
			}))
			defer upstream.Close()
			h := handlers.NewVersionsHandler(testDB, nil, clients.NewServingClient(upstream.URL), 25)
			h.SetHostingAddress("https", "443")
			if tc.name == "tls-provisioning" {
				h.SetTLSStatePath(filepath.Join(t.TempDir(), "missing.json"))
			}
			router := mux.NewRouter()
			router.Handle("/sites/{fqdn}/versions/{version_id}/deploy", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(h.DeployVersion)))
			router.Handle("/sites/{fqdn}/deployment", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(h.DeploymentStatus)))
			r := httptest.NewRequest("POST", "/sites/"+site.FQDN+"/versions/draft/deploy", strings.NewReader(`{"target":"preview"}`))
			if tc.auth != "" {
				r.Header.Set("Authorization", "Bearer "+tc.auth)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if tc.name == "tls-provisioning" && w.Header().Get("Retry-After") != "60" {
				t.Fatal("missing provisioning retry guidance")
			}
			if w.Code != tc.status || len(paths) != tc.calls {
				t.Fatalf("response %d %s paths %v", w.Code, w.Body.String(), paths)
			}
			if len(paths) == 1 && !strings.HasSuffix(paths[0], "/deployment") {
				t.Fatal(paths)
			}
			if tc.status == 200 {
				var result map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["url"] != "https://"+strings.Replace(site.FQDN, ".", ".preview.", 1)+"/" || result["version_id"] != "draft" {
					t.Fatalf("invalid response %v %v", result, err)
				}
			} else if strings.Contains(w.Body.String(), `"url"`) {
				t.Fatal("failure advertised preview URL")
			}
			saved, err := testDB.GetSiteByFQDN(site.FQDN)
			if err != nil || saved.LiveVersionID == nil || *saved.LiveVersionID != live {
				t.Fatalf("live changed %+v %v", saved, err)
			}
			if tc.status != 200 && saved.PreviewVersionID != nil {
				t.Fatal("failed activation changed preview DB")
			}
			statusRequest := httptest.NewRequest("GET", "/sites/"+site.FQDN+"/deployment", nil)
			if tc.auth != "" {
				statusRequest.Header.Set("Authorization", "Bearer "+tc.auth)
			}
			statusResponse := httptest.NewRecorder()
			router.ServeHTTP(statusResponse, statusRequest)
			want := 200
			if tc.auth == "" {
				want = 401
			} else if tc.auth == foreign {
				want = 403
			}
			if statusResponse.Code != want {
				t.Fatalf("status authorization: %d", statusResponse.Code)
			}
			if want == 200 && statusResponse.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("deployment status must not be cached")
			}
		})
	}
}
