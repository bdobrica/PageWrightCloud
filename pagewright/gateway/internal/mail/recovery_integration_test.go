//go:build integration

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Join the real TLS SMTP transport to the real reset handlers/database: the
// consumed secret must come from the delivered message, never a direct DB read.
func TestDeliveredEmailPasswordRecovery(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("TEST_DATABASE_URL required; use disposable integration stack")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	admin, err := database.NewDB(raw)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer admin.Close()
	schema := "mail_release_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := database.NewDB(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = db.RunMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"tls", "starttls"} {
		t.Run(mode, func(t *testing.T) {
			sender, received := smtpFixture(t, mode)
			email := uuid.NewString() + "@mail-release.test"
			hash, err := auth.HashPassword("old-release-password")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.CreateUser(email, hash, nil, nil); err != nil {
				t.Fatal(err)
			}
			h := handlers.NewAuthHandler(db, auth.NewJWTManager("release-only-jwt-secret", time.Minute), nil)
			h.SetResetSender(&sender)
			routes := http.NewServeMux()
			routes.HandleFunc("/forgot", h.ForgotPassword)
			routes.HandleFunc("/reset", h.ResetPassword)
			routes.HandleFunc("/login", h.Login)
			server := httptest.NewServer(routes)
			defer server.Close()
			client := &http.Client{Timeout: 10 * time.Second}
			call := func(path string, body map[string]string, want int) {
				t.Helper()
				data, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				response, err := client.Post(server.URL+path, "application/json", bytes.NewReader(data))
				if err != nil {
					t.Fatal("HTTP request failed")
				}
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if response.StatusCode != want {
					t.Fatalf("%s status %d, want %d", path, response.StatusCode, want)
				}
			}
			call("/forgot", map[string]string{"email": email}, 200)
			var message string
			select {
			case message = <-received:
			case <-time.After(10 * time.Second):
				t.Fatal("no SMTP delivery")
			}
			if !strings.Contains(message, "To: "+email) {
				t.Fatal("wrong delivered recipient")
			}
			matches := regexp.MustCompile(`https://app\.example\.test/reset-password#token=([a-f0-9]{64})`).FindAllStringSubmatch(message, -1)
			if len(matches) != 1 {
				t.Fatal("expected one valid delivered reset link")
			}
			body := map[string]string{"token": matches[0][1], "password": "new-release-password"}
			call("/reset", body, 200)
			call("/reset", body, 400)
			call("/login", map[string]string{"email": email, "password": "old-release-password"}, 401)
			call("/login", map[string]string{"email": email, "password": "new-release-password"}, 200)
		})
	}
}
