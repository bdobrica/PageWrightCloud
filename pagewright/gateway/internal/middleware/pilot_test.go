package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
)

type fakeRate struct {
	keys []string
	err  error
}

func (f *fakeRate) TakePilotRate(_ context.Context, key string, _ int) error {
	f.keys = append(f.keys, key)
	return f.err
}
func TestRegistrationGateClosedBeforeSideEffects(t *testing.T) {
	calls := 0
	next := func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(201) }
	for _, open := range []bool{false, true} {
		w := httptest.NewRecorder()
		RegistrationGate(open, next)(w, httptest.NewRequest("POST", "/auth/register", nil))
		want := 403
		if open {
			want = 201
		}
		if w.Code != want {
			t.Fatal(w.Code)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestPilotThrottleFailsClosedAndIgnoresForwardedIP(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	store := &fakeRate{}
	handler := PilotThrottle(store, false)(next)
	for _, forwarded := range []string{"a", "b"} {
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", forwarded)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
	if store.keys[1] != store.keys[3] {
		t.Fatal("forwarded header bypass")
	}
	for _, tc := range []struct {
		err  error
		code int
	}{{database.ErrPilotLimit, 429}, {errors.New("down"), 503}} {
		store.err = tc.err
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/sites", nil))
		if w.Code != tc.code {
			t.Fatal(w.Code)
		}
	}
}
