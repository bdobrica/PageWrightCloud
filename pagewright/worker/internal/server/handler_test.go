package server

import (
	"net/http/httptest"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/codex"
)

func TestKillCancelsJobOutsideCLIPhase(t *testing.T) {
	s := NewServer(0, codex.NewExecutor("", "", "", ""))
	cancelled := false
	s.SetJobCancel(func() { cancelled = true })
	w := httptest.NewRecorder()
	s.SetupRoutes().ServeHTTP(w, httptest.NewRequest("POST", "/kill", nil))
	if w.Code != 200 || !cancelled {
		t.Fatal("job cancellation was not delivered")
	}
}
