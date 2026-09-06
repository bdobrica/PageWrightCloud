package clients

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServingVersionWireContract(t *testing.T) {
	for _, action := range []string{"artifacts", "activate", "preview"} {
		t.Run(action, func(t *testing.T) {
			for _, status := range []int{200, 201, 400, 500, 307} {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" || r.URL.Path != "/sites/example.test/"+action || r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("unexpected request %s %s", r.Method, r.URL)
					}
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					want := 1
					if action == "artifacts" {
						want = 2
						if body["site_id"] != "site" {
							t.Error("missing site identity")
						}
					}
					if len(body) != want || body["version"] != "version" {
						t.Errorf("wrong wire body: %v", body)
					}
					w.Header().Set("Location", "/redirect")
					w.WriteHeader(status)
				}))
				client := NewServingClient(server.URL + "/")
				var err error
				switch action {
				case "artifacts":
					err = client.DeployArtifact("example.test", "site", "version")
				case "activate":
					err = client.ActivateVersion("example.test", "version")
				case "preview":
					err = client.ActivatePreview("example.test", "version")
				}
				success := status == 200 || (action == "artifacts" && status == 201)
				if (err == nil) != success {
					t.Errorf("status %d: %v", status, err)
				}
				server.Close()
			}
		})
	}
}
