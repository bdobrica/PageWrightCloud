package clients

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLLMAllowanceErrorsAreRecognizable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"allowance exhausted","type":"pilot_limit"}}`))
	}))
	defer server.Close()
	client := NewLLMClient("dummy", server.URL)
	if _, err := client.EvaluateRequest("edit"); !errors.Is(err, ErrAIAllowance) {
		t.Fatalf("evaluate: %v", err)
	}
	if _, err := client.GenerateJobInstructions("edit", ""); !errors.Is(err, ErrAIAllowance) {
		t.Fatalf("generate: %v", err)
	}
}
