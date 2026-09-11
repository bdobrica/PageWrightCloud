package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type conversationFixtureStore struct{ buildStore }

func (conversationFixtureStore) GetSiteByFQDNContext(_ context.Context, fqdn string) (*types.Site, error) {
	return &types.Site{ID: fqdn, UserID: fqdn, InitializationStatus: "ready"}, nil
}
func (conversationFixtureStore) FindBuildSubmission(context.Context, string, string, string) (*database.BuildSubmission, error) {
	return nil, nil
}

type conversationExpected struct{}
type conversationFixtureProvider struct {
	t     *testing.T
	calls atomic.Int32
}

func (*conversationFixtureProvider) EvaluateRequestContext(context.Context, string) (*clients.EvaluationResponse, error) {
	return &clients.EvaluationResponse{IsClear: false, Question: "Which title?"}, nil
}
func (p *conversationFixtureProvider) GenerateJobInstructionsContext(ctx context.Context, original, clarification string) (string, error) {
	p.calls.Add(1)
	if original != ctx.Value(conversationExpected{}) || clarification != "clarified" {
		p.t.Errorf("conversation content crossed requests")
	}
	// Fail before storage/admission/dispatch. Retrying must retain this context.
	return "", errors.New("fixture provider unavailable")
}

func TestConcurrentConversationIsolationAndRetry(t *testing.T) {
	provider := &conversationFixtureProvider{t: t}
	h := NewBuildHandler(conversationFixtureStore{}, provider, nil, nil)
	ids := make(chan string, 320)
	defer func() {
		close(ids)
		conversationMu.Lock()
		defer conversationMu.Unlock()
		for id := range ids {
			delete(conversationStore, id)
		}
	}()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for j := 0; j < 10; j++ {
				owner := fmt.Sprintf("%s-%d-%d", t.Name(), i, j)
				original := "original-" + owner
				body, _ := json.Marshal(types.BuildRequest{Message: original})
				r := mux.SetURLVars(httptest.NewRequest("POST", "/build", bytes.NewReader(body)), map[string]string{"fqdn": owner})
				r.Header.Set("Idempotency-Key", uuid.NewString())
				r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &auth.Claims{UserID: owner}))
				w := httptest.NewRecorder()
				h.Build(w, r)
				var response types.BuildResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || response.ConversationID == nil {
					t.Errorf("clarification not created: %d %v", w.Code, err)
					return
				}
				ids <- *response.ConversationID
				for _, check := range []struct {
					user, site string
					want       int
				}{{"other", owner, 403}, {owner, "other", 403}, {owner, owner, 500}, {owner, owner, 500}} {
					ctx := context.WithValue(context.Background(), middleware.UserContextKey, &auth.Claims{UserID: check.user})
					ctx = context.WithValue(ctx, conversationExpected{}, original)
					r := httptest.NewRequest("POST", "/build", nil).WithContext(ctx)
					w := httptest.NewRecorder()
					h.handleClarification(w, r, &types.Site{ID: check.site}, types.BuildRequest{Message: "clarified", ConversationID: response.ConversationID}, "unused", "unused")
					if w.Code != check.want {
						t.Errorf("clarification status: got %d want %d", w.Code, check.want)
					}
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if provider.calls.Load() != 640 {
		t.Fatalf("unexpected provider calls (denials must not call): %d", provider.calls.Load())
	}
}
