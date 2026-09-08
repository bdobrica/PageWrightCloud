//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/google/uuid"
)

type resetDelivery func(context.Context, string, string) error

func (f resetDelivery) SendReset(ctx context.Context, email, token string) error {
	return f(ctx, email, token)
}

func TestPasswordResetAtomicRollbackAndSiblingInvalidation(t *testing.T) {
	u, err := testDB.CreateUser(uuid.NewString()+"@reset-rollback.test", "original-hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, b := strings.Repeat("1", 64), strings.Repeat("2", 64)
	for _, token := range []string{a, b} {
		if _, err := testDB.CreatePasswordResetToken(u.ID, token, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	// Inject a DB update failure inside this disposable schema only. The token
	// update must roll back with the password; no partial consumption is allowed.
	_, err = testDB.Exec(fmt.Sprintf(`CREATE FUNCTION fail_reset_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test update failure'; END $$;
CREATE TRIGGER fail_reset_update BEFORE UPDATE ON users FOR EACH ROW WHEN (OLD.id='%s'::uuid) EXECUTE FUNCTION fail_reset_update()`, u.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Exec(`DROP TRIGGER IF EXISTS fail_reset_update ON users; DROP FUNCTION IF EXISTS fail_reset_update()`)
	if err := testDB.ConsumePasswordReset(context.Background(), a, "new-hash"); err == nil {
		t.Fatal("injected failure ignored")
	}
	r, err := testDB.GetPasswordResetToken(a)
	if err != nil || r.Used {
		t.Fatal("failed password update consumed token")
	}
	user, err := testDB.GetUserByID(u.ID)
	if err != nil || user.PasswordHash != "original-hash" {
		t.Fatal("failed transaction changed password")
	}
	if _, err := testDB.Exec(`DROP TRIGGER fail_reset_update ON users; DROP FUNCTION fail_reset_update()`); err != nil {
		t.Fatal(err)
	}
	if err := testDB.ConsumePasswordReset(context.Background(), a, "new-hash"); err != nil {
		t.Fatal(err)
	}
	if err := testDB.ConsumePasswordReset(context.Background(), b, "other-hash"); !errors.Is(err, database.ErrResetToken) {
		t.Fatal("sibling reset link still usable")
	}
}

func TestPasswordResetDeliveryConsumptionAndThrottle(t *testing.T) {
	email := uuid.NewString() + "@reset.test"
	original, err := auth.HashPassword("original-password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := testDB.CreateUser(email, original, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := handlers.NewAuthHandler(testDB, testJWTManager, nil)
	var token string
	h.SetResetSender(resetDelivery(func(_ context.Context, to, value string) error {
		if to != email {
			t.Error("wrong recipient")
		}
		token = value
		return nil
	}))
	call := func(handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/auth/test", strings.NewReader(string(data)))
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	known := call(h.ForgotPassword, map[string]string{"email": email})
	unknown := call(h.ForgotPassword, map[string]string{"email": uuid.NewString() + "@reset.test"})
	if known.Code != 200 || unknown.Code != 200 || known.Body.String() != unknown.Body.String() || !strings.Contains(known.Header().Get("Content-Type"), "application/json") {
		t.Fatal("reset account enumeration contract")
	}
	if len(token) != 64 || strings.Contains(known.Body.String(), token) {
		t.Fatal("missing or leaked token")
	}
	var saved string
	if err := testDB.Get(&saved, `SELECT token FROM password_reset_tokens WHERE user_id=$1`, user.ID); err != nil || saved == token || len(saved) != 64 {
		t.Fatal("plaintext token storage")
	}
	if r := call(h.ForgotPassword, map[string]string{"email": email}); r.Code != 429 || r.Header().Get("Retry-After") != "60" {
		t.Fatal("reset throttle missing")
	}
	// Invalid passwords cannot consume a valid token.
	if r := call(h.ResetPassword, map[string]string{"token": token, "password": "short"}); r.Code != 400 {
		t.Fatal("weak password accepted")
	}
	var wg sync.WaitGroup
	results := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- call(h.ResetPassword, map[string]string{"token": token, "password": "replacement-password"}).Code
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for status := range results {
		if status == 200 {
			winners++
		} else if status != 400 {
			t.Fatalf("unexpected reset result %d", status)
		}
	}
	if winners != 1 {
		t.Fatalf("token had %d successful consumers", winners)
	}
	current, err := testDB.GetUserByID(user.ID)
	if err != nil || !auth.CheckPasswordHash("replacement-password", current.PasswordHash) {
		t.Fatal("password not committed")
	}
	if r := call(h.Login, map[string]string{"email": email, "password": "original-password"}); r.Code != 401 {
		t.Fatal("old password still works")
	}
	if r := call(h.Login, map[string]string{"email": email, "password": "replacement-password"}); r.Code != 200 {
		t.Fatal("new password cannot log in")
	}
	expired := strings.Repeat("e", 64)
	if _, err := testDB.CreatePasswordResetToken(user.ID, expired, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if r := call(h.ResetPassword, map[string]string{"token": expired, "password": "another-password"}); r.Code != 400 {
		t.Fatal("expired token accepted")
	}
	sibling := strings.Repeat("b", 64)
	if _, err := testDB.CreatePasswordResetToken(user.ID, sibling, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := testDB.UpdateUserPassword(user.ID, original); err != nil {
		t.Fatal(err)
	}
	if r := call(h.ResetPassword, map[string]string{"token": sibling, "password": "another-password"}); r.Code != 400 {
		t.Fatal("password change did not revoke pending link")
	}
}

func TestResetDeliveryFailureAndDisabled(t *testing.T) {
	email := uuid.NewString() + "@failed-reset.test"
	user, err := testDB.CreateUser(email, "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := handlers.NewAuthHandler(testDB, testJWTManager, nil)
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ForgotPassword(w, httptest.NewRequest("POST", "/auth/forgot-password", strings.NewReader(`{"email":"`+email+`"}`)))
		return w
	}
	if request().Code != 503 {
		t.Fatal("unconfigured delivery advertised success")
	}
	h.SetResetSender(resetDelivery(func(context.Context, string, string) error { return errors.New("PRIVATE provider response") }))
	w := request()
	if w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatal("delivery failure leaked")
	}
	var usable int
	if err := testDB.Get(&usable, `SELECT count(*) FROM password_reset_tokens WHERE user_id=$1 AND NOT used`, user.ID); err != nil || usable != 0 {
		t.Fatal("failed delivery left usable token")
	}
}
