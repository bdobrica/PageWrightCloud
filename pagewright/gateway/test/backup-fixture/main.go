// Disposable backup fixture, not a production import/build endpoint.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type evidence struct {
	Owner, Other, Site, FQDN, Email, Hash string
	Artifacts                             map[string]string
	Sequence                              int64
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func check(ok bool, what string) {
	if !ok {
		panic(what)
	}
}
func put(path string, data []byte) {
	must(os.MkdirAll(filepath.Dir(path), 0755))
	must(os.WriteFile(path, data, 0644))
}
func contents(version string) map[string]string {
	return map[string]string{"manifest.json": `{"schema_version":1,"kind":"compiled","theme_id":"starter"}`,
		"content/site.json": `{"site_name":"Backup test"}`, "content/home/index.md": "# Backup fixture",
		"public/index.html": "<h1>Recovered " + version + "</h1>", "public/style.css": "/* " + version + " */",
		"public/app.js": "/* offline fixture " + version + " */", "public/nested/index.html": "Nested " + version}
}
func archive(version string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range contents(version) {
		must(tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data))}))
		_, err := tw.Write([]byte(data))
		must(err)
	}
	must(tw.Close())
	must(gz.Close())
	return buf.Bytes()
}
func deploy(db *database.DB, e evidence, version, target string) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d, err := db.ReserveDeployment(ctx, e.Site, version, target)
	must(err)
	status, err := clients.NewServingClient("http://serving:8083").ApplyDeployment(ctx, d)
	must(err)
	check(status == "completed", "not deployed")
	must(db.FinishDeployment(ctx, d, status))
	return d.Sequence
}
func seed(db *database.DB) evidence {
	must(db.RunMigrations(context.Background()))
	hash, err := auth.HashPassword("backup-fixture-password")
	must(err)
	e := evidence{Email: uuid.NewString() + "@backup.test", FQDN: "restore.example.test", Hash: hash, Artifacts: map[string]string{}}
	u, err := db.CreateUser(e.Email, hash, nil, nil)
	must(err)
	e.Owner = u.ID
	other, err := db.CreateUser(uuid.NewString()+"@backup.test", hash, nil, nil)
	must(err)
	e.Other = other.ID
	site, record, err := db.ReserveSiteBootstrap(context.Background(), u.ID, e.FQDN)
	must(err)
	e.Site = site.ID
	// Explicit deterministic fixture seeding. The real storage read and serving
	// deployment APIs validate these records; this does not claim an AI execution.
	base := filepath.Join("/nfs/sites", e.Site)
	put(filepath.Join(base, "artifacts/initial.tar.gz"), record.Archive)
	e.Artifacts["initial"] = fmt.Sprintf("%x", sha256.Sum256(record.Archive))
	put(filepath.Join(base, "metadata/initial/execution.json"), record.ExecutionLog)
	put(filepath.Join(base, "metadata/initial/manifest.json"), record.Manifest)
	must(db.CompleteSiteBootstrap(context.Background(), e.Site))
	for _, version := range []string{"v1", "v2"} {
		data := archive(version)
		put(filepath.Join(base, "artifacts", version+".tar.gz"), data)
		e.Artifacts[version] = fmt.Sprintf("%x", sha256.Sum256(data))
		put(filepath.Join(base, "metadata", version, "execution.json"), []byte(`{"fixture":true,"provider_calls":0}`))
		manifest, err := json.Marshal(map[string]any{"site_id": e.Site, "build_id": version, "kind": "compiled", "checks_passed": true, "created_at": time.Now().UTC()})
		must(err)
		put(filepath.Join(base, "metadata", version, "manifest.json"), manifest)
		_, err = db.CreateVersion(e.Site, version, "completed")
		must(err)
	}
	deploy(db, e, "v1", "live")
	e.Sequence = deploy(db, e, "v2", "preview")
	_, err = db.Exec("INSERT INTO pilot_provider_reservations (id,reserved_cents,active) VALUES ($1,100,false)", uuid.NewString())
	must(err)
	return e
}
func verify(db *database.DB, e evidence) {
	u, err := db.GetUserByID(e.Owner)
	must(err)
	check(u != nil && u.Email == e.Email && u.PasswordHash == e.Hash, "owner/password changed")
	site, err := db.GetSiteByFQDN(e.FQDN)
	must(err)
	check(site != nil && site.ID == e.Site && site.UserID == e.Owner && site.LiveVersionID != nil && *site.LiveVersionID == "v1" && site.PreviewVersionID != nil && *site.PreviewVersionID == "v2", "pointers/ownership changed")
	d, err := db.GetDeployment(context.Background(), e.Site)
	must(err)
	check(d.Sequence == e.Sequence && d.Status == "completed", "receipt identity changed")
	rows, count, err := db.GetSiteVersions(e.Site, 25, 0)
	must(err)
	check(count == 3 && len(rows) == 3, "database version history incomplete")
	seen := map[string]bool{}
	for _, v := range rows {
		check((v.BuildID == "initial" || v.BuildID == "v1" || v.BuildID == "v2") && v.Status == "completed" && !seen[v.BuildID], "database version identity changed")
		seen[v.BuildID] = true
	}
	receiptBytes, err := os.ReadFile(filepath.Join("/restored-www/example.test", e.FQDN, ".deployment.json"))
	must(err)
	var receipt database.Deployment
	must(json.Unmarshal(receiptBytes, &receipt))
	check(receipt.SiteID == e.Site && receipt.Sequence == e.Sequence && receipt.Version == "v2" && receipt.Target == "preview" && receipt.Status == "completed", "serving receipt changed")
	for version, want := range e.Artifacts {
		data, err := clients.NewStorageClient("http://storage:8080").FetchArtifact(e.Site, version)
		must(err)
		check(fmt.Sprintf("%x", sha256.Sum256(data)) == want, "stored artifact bytes changed")
	}
	var reserved int
	must(db.Get(&reserved, "SELECT sum(reserved_cents) FROM pilot_provider_reservations"))
	check(reserved == 100, "provider reservations lost")
	h := handlers.NewVersionsHandler(db, clients.NewStorageClient("http://storage:8080"), clients.NewServingClient("http://serving:8083"), 25)
	for _, owner := range []string{e.Owner, e.Other} {
		r := mux.SetURLVars(httptest.NewRequest("GET", "/versions", nil), map[string]string{"fqdn": e.FQDN})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &auth.Claims{UserID: owner}))
		w := httptest.NewRecorder()
		h.ListVersions(w, r)
		if owner == e.Other {
			check(w.Code == 403, "cross-owner versions leaked")
			continue
		}
		check(w.Code == 200, "history unavailable")
		var page struct {
			Data []struct {
				BuildID string `json:"build_id"`
			}
		}
		must(json.Unmarshal(w.Body.Bytes(), &page))
		check(len(page.Data) == 3, "restored history incomplete")
	}
	c := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, version := range []string{"v1", "v2"} {
		host := e.FQDN
		if version == "v2" {
			host = strings.Replace(host, ".", ".preview.", 1)
		}
		for name, want := range contents(version) {
			if !strings.HasPrefix(name, "public/") {
				continue
			}
			path := strings.TrimPrefix(name, "public/")
			if path == "index.html" {
				path = ""
			}
			r, err := http.NewRequest("GET", "http://nginx/"+path, nil)
			must(err)
			r.Host = host
			resp, err := c.Do(r)
			must(err)
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			must(err)
			check(resp.StatusCode == 200 && string(data) == want, "published content differs: "+version+"/"+path)
		}
	}
}
func main() {
	check(len(os.Args) == 2, "seed or check required")
	db, err := database.NewDB(os.Getenv("TEST_DATABASE_URL"))
	must(err)
	defer db.Close()
	if os.Args[1] == "seed" {
		must(json.NewEncoder(os.Stdout).Encode(seed(db)))
		return
	}
	check(os.Args[1] == "check", "unknown operation")
	var e evidence
	must(json.NewDecoder(os.Stdin).Decode(&e))
	verify(db, e)
	next := deploy(db, e, "v1", "live")
	check(next > e.Sequence, "deployment sequence regressed")
	fmt.Println("PASS restored ownership, password hash, version history, cross-owner denial, allowance, live/preview HTML/assets, and next deployment sequence")
}
