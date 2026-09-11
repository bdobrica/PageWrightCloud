package middleware

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
)

type RateStore interface {
	TakePilotRate(context.Context, string, int) error
}

func PilotThrottle(store RateStore, authenticated bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "OPTIONS" || (r.URL.Path == "/health" || r.URL.Path == "/ready") {
				next.ServeHTTP(w, r)
				return
			}
			key, limit := "global", 600
			if authenticated {
				user, ok := GetUserFromContext(r)
				if !ok || user == nil {
					http.Error(w, "authentication required", 401)
					return
				}
				key, limit = "owner:"+user.UserID, 120
			}
			if err := store.TakePilotRate(r.Context(), key, limit); err != nil {
				pilotRateError(w, err)
				return
			}
			if !authenticated {
				// Never trust user-supplied forwarding headers. Proxy trust is a separate gate.
				ip, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					ip = r.RemoteAddr
				}
				category := "request"
				limit = 120
				if len(r.URL.Path) >= 6 && r.URL.Path[:6] == "/auth/" {
					category = "auth"
					limit = 10
				}
				key = fmt.Sprintf("%s:%x", category, sha256.Sum256([]byte(ip)))
				if err := store.TakePilotRate(r.Context(), key, limit); err != nil {
					pilotRateError(w, err)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			next.ServeHTTP(w, r)
		})
	}
}
func pilotRateError(w http.ResponseWriter, err error) {
	w.Header().Set("Cache-Control", "no-store")
	if errors.Is(err, database.ErrPilotLimit) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Request limit reached. Wait a minute and retry.", 429)
	} else {
		http.Error(w, "Usage checks unavailable. Retry later.", 503)
	}
}

func RegistrationGate(development bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !development {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(403)
			w.Write([]byte(`{"error":"registration_closed","message":"Accounts are provisioned by the operator. Contact the site administrator."}`))
			return
		}
		next(w, r)
	}
}
