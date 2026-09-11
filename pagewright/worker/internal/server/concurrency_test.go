package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/codex"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

func TestConcurrentStatusAndCancellation(t *testing.T) {
	s := NewServer(0, codex.NewExecutor("", "", "", ""))
	var cancelled atomic.Int32
	cancel := func() { cancelled.Add(1); s.SetError(fmt.Errorf("cancelled")) }
	s.SetJobCancel(cancel) // Callback reenters status mutation: no mutex held by caller.
	s.UpdateStatus("running", "step-0", 0)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 50; i++ {
				s.SetJobCancel(cancel)
				s.UpdateStatus("running", fmt.Sprintf("step-%d", i), i)
				w := httptest.NewRecorder()
				s.GetStatus(w, httptest.NewRequest("GET", "/status", nil))
				var status types.WorkerStatus
				if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
					t.Error(err)
					return
				}
				if status.CurrentStep != "Execution cancelled by manager" && status.CurrentStep != fmt.Sprintf("step-%d", status.Progress) {
					t.Errorf("torn status snapshot: %+v", status)
				}
				w = httptest.NewRecorder()
				s.KillCodex(w, httptest.NewRequest("POST", "/kill", nil))
				if w.Code != 200 {
					t.Errorf("cancel status: %d", w.Code)
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	if cancelled.Load() != 800 {
		t.Fatalf("lost cancellation callbacks: %d", cancelled.Load())
	}
}
