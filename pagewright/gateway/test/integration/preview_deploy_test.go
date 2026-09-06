//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{"artifact-failure", 500, 200, 500, token, 1}, {"activation-failure", 200, 500, 500, token, 2},
		{"success", 200, 200, 200, token, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := []string{}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["version"] != "draft" {
					t.Errorf("invalid serving payload %v %v", payload, err)
				}
				if strings.HasSuffix(r.URL.Path, "/artifacts") {
					w.WriteHeader(tc.artifactStatus)
				} else {
					w.WriteHeader(tc.activateStatus)
				}
			}))
			defer upstream.Close()
			h := handlers.NewVersionsHandler(testDB, nil, clients.NewServingClient(upstream.URL), 25)
			h.SetHostingAddress("https", "443")
			router := mux.NewRouter()
			router.Handle("/sites/{fqdn}/versions/{version_id}/deploy", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(h.DeployVersion)))
			r := httptest.NewRequest("POST", "/sites/"+site.FQDN+"/versions/draft/deploy", strings.NewReader(`{"target":"preview"}`))
			if tc.auth != "" {
				r.Header.Set("Authorization", "Bearer "+tc.auth)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tc.status || len(paths) != tc.calls {
				t.Fatalf("response %d %s paths %v", w.Code, w.Body.String(), paths)
			}
			if len(paths) == 2 && (!strings.HasSuffix(paths[0], "/artifacts") || !strings.HasSuffix(paths[1], "/preview")) {
				t.Fatal(paths)
			}
			if tc.status == 200 {
				var result map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["url"] != "https://"+site.FQDN+"/preview/" || result["version_id"] != "draft" {
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
		})
	}
}
