package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
)

func TestExpiredSessionCannotReachHandlerButFreshSessionCan(t *testing.T) {
	manager := auth.NewJWTManager("session-test-key", time.Minute)
	expired, err := auth.NewJWTManager("session-test-key", -time.Minute).GenerateToken("owner", "owner@example.test")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := manager.GenerateToken("owner", "owner@example.test")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	handler := AuthMiddleware(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		user, ok := GetUserFromContext(r)
		if !ok || user.UserID != "owner" {
			t.Fatal("lost owner")
		}
		w.WriteHeader(204)
	}))
	for _, tc := range []struct {
		token  string
		status int
	}{{expired, 401}, {fresh, 204}} {
		req := httptest.NewRequest("GET", "/sites/owned/jobs", nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("session status %d", res.Code)
		}
	}
	if calls != 1 {
		t.Fatal("expired session reached protected work")
	}
}
