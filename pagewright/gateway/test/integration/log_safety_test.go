//go:build integration

package integration

import (
	"bytes"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/google/uuid"
)

func TestPasswordResetDoesNotLogTokenOrAccount(t *testing.T) {
	email := "reset-private-" + uuid.NewString() + "@example.test"
	user, err := testDB.CreateUser(email, "private-password-hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	w := httptest.NewRecorder()
	handlers.NewAuthHandler(testDB, testJWTManager, nil).ForgotPassword(w, httptest.NewRequest("POST", "/auth/forgot-password", strings.NewReader(`{"email":"`+email+`"}`)))
	if w.Code != 200 {
		t.Fatalf("reset status %d", w.Code)
	}
	var token string
	if err := testDB.Get(&token, "SELECT token FROM password_reset_tokens WHERE user_id=$1", user.ID); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{token, email, "private-password-hash"} {
		if strings.Contains(logs.String(), private) || strings.Contains(w.Body.String(), private) {
			t.Fatal("reset leaked private material")
		}
	}
}
