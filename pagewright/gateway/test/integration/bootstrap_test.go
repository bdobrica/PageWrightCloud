//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func TestSiteBootstrapHTTPRecovery(t *testing.T) {
	storageURL := os.Getenv("TEST_STORAGE_URL")
	if storageURL == "" {
		t.Fatal("TEST_STORAGE_URL required")
	}
	for _, failure := range []string{"artifact", "logs", "manifest", "lost_response", "confirmation", "none"} {
		t.Run(failure, func(t *testing.T) {
			user, err := testDB.CreateUser(uuid.NewString()+"@bootstrap.test", "hash", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			token, err := testJWTManager.GenerateToken(user.ID, user.Email)
			if err != nil {
				t.Fatal(err)
			}
			fqdn := "bootstrap-" + uuid.NewString() + ".example.test"
			target, err := url.Parse(storageURL)
			if err != nil {
				t.Fatal(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(target)
			var fail atomic.Bool
			fail.Store(failure != "none")
			proxy.ModifyResponse = func(resp *http.Response) error {
				if failure == "lost_response" && fail.Load() && strings.HasSuffix(resp.Request.URL.Path, "/manifest") {
					resp.Body.Close()
					return errors.New("simulated lost manifest acknowledgement")
				}
				return nil
			}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				parts := strings.Split(r.URL.Path, "/")
				var reserved int
				if len(parts) < 5 || testDB.Get(&reserved, `SELECT count(*) FROM site_bootstraps WHERE site_id=$1`, parts[2]) != nil || reserved != 1 {
					t.Error("upload before reservation")
					w.WriteHeader(500)
					return
				}
				stage := "artifact"
				if strings.HasSuffix(r.URL.Path, "/logs") {
					stage = "logs"
				}
				if strings.HasSuffix(r.URL.Path, "/manifest") {
					stage = "manifest"
				}
				if fail.Load() && stage == failure {
					w.WriteHeader(500)
					return
				}
				proxy.ServeHTTP(w, r)
			}))
			defer upstream.Close()
			post := func(ownerToken, domain, template string) (int, []byte) {
				t.Helper()
				// Fresh handler/router on every call: no in-memory retry state.
				h := handlers.NewSitesHandler(testDB, nil, clients.NewStorageClient(upstream.URL), 25)
				if err := h.SetSiteDomain("example.test"); err != nil {
					t.Fatal(err)
				}
				router := mux.NewRouter()
				router.Handle("/sites", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(h.CreateSite)))
				body, _ := json.Marshal(types.CreateSiteRequest{FQDN: domain, TemplateID: template})
				req := httptest.NewRequest("POST", "/sites", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+ownerToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				return w.Code, w.Body.Bytes()
			}
			if failure == "confirmation" {
				_, err = testDB.Exec(`CREATE FUNCTION reject_bootstrap_ready() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.initialization_status='ready' THEN RAISE EXCEPTION 'test outcome failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_bootstrap_ready BEFORE UPDATE ON sites FOR EACH ROW EXECUTE FUNCTION reject_bootstrap_ready()`)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					testDB.Exec(`DROP TRIGGER IF EXISTS reject_bootstrap_ready ON sites; DROP FUNCTION IF EXISTS reject_bootstrap_ready()`)
				})
			}
			status, body := post(token, fqdn, "template-1")
			if failure != "none" {
				if status != 503 {
					t.Fatalf("failure status %d: %s", status, body)
				}
				site, err := testDB.GetSiteByFQDN(fqdn)
				if err != nil || site == nil || site.InitializationStatus != "pending" {
					t.Fatalf("missing pending site: %v %v", site, err)
				}
				build := handlers.NewBuildHandler(testDB, nil, nil, nil)
				router := mux.NewRouter()
				router.Handle("/sites/{fqdn}/build", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(build.Build)))
				req := httptest.NewRequest("POST", "/sites/"+fqdn+"/build", strings.NewReader(`{"message":"edit"}`))
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Idempotency-Key", uuid.NewString())
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code != 409 {
					t.Fatalf("pending build status %d", w.Code)
				}
				var versions int
				if err := testDB.Get(&versions, `SELECT count(*) FROM versions WHERE site_id=$1`, site.ID); err != nil || versions != 0 {
					t.Fatalf("premature version %d %v", versions, err)
				}
				if failure == "confirmation" {
					if _, err := testDB.Exec(`DROP TRIGGER reject_bootstrap_ready ON sites; DROP FUNCTION reject_bootstrap_ready()`); err != nil {
						t.Fatal(err)
					}
				}
				fail.Store(false)
				status, body = post(token, fqdn, "starter")
			}
			if status != 201 {
				t.Fatalf("create status %d: %s", status, body)
			}
			var site types.Site
			if err := json.Unmarshal(body, &site); err != nil {
				t.Fatal(err)
			}
			if site.InitializationStatus != "ready" || site.TemplateID != "starter" || site.Enabled || site.LiveVersionID != nil {
				t.Fatalf("unexpected initialized site %+v", site)
			}
			archive, err := clients.NewStorageClient(storageURL).FetchArtifact(site.ID, "initial")
			if err != nil {
				t.Fatal(err)
			}
			var saved []byte
			if err := testDB.Get(&saved, `SELECT archive FROM site_bootstraps WHERE site_id=$1`, site.ID); err != nil || !bytes.Equal(saved, archive) {
				t.Fatalf("source mismatch: %v", err)
			}
			var versionStatus string
			if err := testDB.Get(&versionStatus, `SELECT status FROM versions WHERE site_id=$1 AND build_id='initial'`, site.ID); err != nil || versionStatus != "completed" {
				t.Fatalf("initial version: %s %v", versionStatus, err)
			}
			before := calls.Load()
			status, body = post(token, strings.ToUpper(fqdn), "starter")
			if status != 201 || calls.Load() != before {
				t.Fatal("ready retry performed writes")
			}
			other, err := testDB.CreateUser(uuid.NewString()+"@bootstrap.test", "hash", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			otherToken, _ := testJWTManager.GenerateToken(other.ID, other.Email)
			if status, _ := post(otherToken, fqdn, "starter"); status != 409 {
				t.Fatalf("owner conflict %d", status)
			}
			for _, bad := range [][2]string{{fqdn, "unknown"}, {"../outside", "starter"}, {"bad..example.test", "starter"}} {
				if status, _ := post(token, bad[0], bad[1]); status != 400 {
					t.Fatalf("invalid site status %d", status)
				}
			}
			if calls.Load() != before {
				t.Fatal("rejected request reached storage")
			}
		})
	}
}

func TestConcurrentSiteBootstrap(t *testing.T) {
	user, err := testDB.CreateUser(uuid.NewString()+"@bootstrap.test", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := testJWTManager.GenerateToken(user.ID, user.Email)
	fqdn := "concurrent-" + uuid.NewString() + ".example.test"
	h := handlers.NewSitesHandler(testDB, nil, clients.NewStorageClient(os.Getenv("TEST_STORAGE_URL")), 25)
	if err := h.SetSiteDomain("example.test"); err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	router.Handle("/sites", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(h.CreateSite)))
	server := httptest.NewServer(router)
	defer server.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", server.URL+"/sites", strings.NewReader(`{"fqdn":"`+fqdn+`","template_id":"starter"}`))
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 201 {
				t.Errorf("concurrent status %d: %s", resp.StatusCode, body)
			}
		}()
	}
	wg.Wait()
	site, err := testDB.GetSiteByFQDN(fqdn)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := testDB.Get(&count, `SELECT count(*) FROM versions WHERE site_id=$1`, site.ID); err != nil || count != 1 {
		t.Fatalf("versions %d %v", count, err)
	}
}
