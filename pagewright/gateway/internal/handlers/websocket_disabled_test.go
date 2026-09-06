package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSocketDisabledNeverUpgradesOrEchoesCredentials(t *testing.T) {
	for _, bearer := range []string{"", "Bearer private-test-token"} {
		for _, origin := range []string{"", "https://foreign.example", "http://localhost:3000"} {
			r := httptest.NewRequest(http.MethodGet, "/ws?token=private-query-token", nil)
			r.Header.Set("Authorization", bearer)
			r.Header.Set("Origin", origin)
			r.Header.Set("Connection", "Upgrade")
			r.Header.Set("Upgrade", "websocket")
			r.Header.Set("Sec-WebSocket-Version", "13")
			r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			w := httptest.NewRecorder()
			WebSocketDisabled(w, r)
			if w.Code != http.StatusNotImplemented || w.Header().Get("Upgrade") != "" || w.Header().Get("Sec-WebSocket-Accept") != "" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected upgrade response: %d %v", w.Code, w.Header())
			}
			if strings.Contains(w.Body.String(), "private-") || !strings.Contains(w.Body.String(), "polling") {
				t.Fatalf("unsafe/unhelpful response: %s", w.Body.String())
			}
		}
	}
}
