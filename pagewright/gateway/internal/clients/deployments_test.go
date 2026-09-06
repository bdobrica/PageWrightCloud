package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
)

func TestDeploymentReceiptIdentity(t *testing.T) {
	d := database.Deployment{SiteID: "site", Sequence: 42, FQDN: "site.example.test", Version: "draft", Target: "preview", Status: "pending"}
	for _, field := range []string{"valid", "site_id", "sequence", "fqdn", "version", "target", "status", "extra", "trailing", "redirect"} {
		t.Run(field, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if field == "redirect" {
					http.Redirect(w, r, "/other", 302)
					return
				}
				data, _ := json.Marshal(d)
				var v map[string]any
				json.Unmarshal(data, &v)
				v["status"] = "completed"
				if field == "sequence" {
					v[field] = 43
				} else if field != "valid" && field != "trailing" {
					v[field] = "wrong"
				}
				json.NewEncoder(w).Encode(v)
				if field == "trailing" {
					w.Write([]byte(`{}`))
				}
			}))
			defer server.Close()
			status, err := NewServingClient(server.URL).ApplyDeployment(context.Background(), &d)
			if field == "valid" {
				if err != nil || status != "completed" {
					t.Fatalf("%s %v", status, err)
				}
			} else if err == nil {
				t.Fatal("unverified receipt accepted")
			}
		})
	}
}
