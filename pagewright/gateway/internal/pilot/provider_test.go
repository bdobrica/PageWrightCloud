package pilot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
)

type fakeStore struct {
	reserved, finished int
	err                error
}

func (s *fakeStore) ReservePilotProvider(context.Context, string, int, int) error {
	if s.err != nil {
		return s.err
	}
	s.reserved++
	return nil
}
func (s *fakeStore) FinishPilotProvider(context.Context, string) error { s.finished++; return nil }
func TestProviderPolicyRejectsBillableExtensions(t *testing.T) {
	for _, raw := range []string{
		`{"model":"gpt-5.6-sol","input":"hi"}`,
		`{"model":"gpt-5.6-luna","input":"hi","previous_response_id":"x"}`,
		`{"model":"gpt-5.6-luna","input":"hi","background":true}`,
		`{"model":"gpt-5.6-luna","input":[{"type":"input_image","image_url":"x"}]}`,
		`{"model":"gpt-5.6-luna","input":"hi","tools":[{"type":"web_search"}]}`,
		`{"model":"gpt-5.6-luna","input":"hi","tools":[{"type":"namespace","tools":[{"type":"mcp"}]}]}`,
	} {
		if _, err := Constrain("/v1/responses", []byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	raw, err := Constrain("/v1/responses", []byte(`{"model":"gpt-5.6-luna","input":"hi","max_output_tokens":999999,"service_tier":"fast","store":true,"tools":[{"type":"function","name":"exec_command"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	json.Unmarshal(raw, &got)
	if got["max_output_tokens"] != float64(8192) || got["service_tier"] != "default" || got["store"] != false {
		t.Fatal(string(raw))
	}
	if _, err := Constrain("/v1/chat/completions", []byte(`{"model":"gpt-3.5-turbo","messages":[{"content":[{"type":"image_url"}]}]}`)); err == nil {
		t.Fatal("media chat")
	}
	if _, err := Constrain("/v1/chat/completions", []byte(`{"model":"gpt-3.5-turbo","messages":[{"content":"hi"}],"n":2}`)); err == nil {
		t.Fatal("multiple completions")
	}
}

func TestDeferredLocalDefinitions(t *testing.T) {
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{`{"type":"additional_tools","id":"at_fixture","role":"developer","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"exec_command","parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}}]}]}`, true},
		{`{"type":"additional_tools","tools":[{"type":"web_search"}]}`, false},
		{`{"type":"additional_tools","tools":[{"type":"namespace","tools":[{"type":"mcp"}]}]}`, false},
		{`{"type":"additional_tools","tools":[],"file_id":"remote"}`, false},
		{`{"type":"additional_tools","tools":{}}`, false},
	} {
		_, err := Constrain("/v1/responses", []byte(`{"model":"gpt-5.6-luna","input":[`+tc.input+`]}`))
		if (err == nil) != tc.valid {
			t.Fatalf("valid=%v error=%v input=%s", tc.valid, err, tc.input)
		}
	}
}
func TestProviderReservesBeforeIOAndNeverRefunds(t *testing.T) {
	store := &fakeStore{}
	calls := 0
	status := 200
	payload := `{"choices":[{"finish_reason":"stop"}]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if store.reserved != calls {
			t.Error("network before durable reservation")
		}
		if r.Header.Get("Authorization") != "Bearer upstream-key" || r.Header.Get("X-Injected") != "" {
			t.Error("credential/header boundary")
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"max_tokens":500`) {
			t.Error(string(raw))
		}
		w.WriteHeader(status)
		io.WriteString(w, payload)
	}))
	defer upstream.Close()
	p, err := NewProvider(store, strings.Repeat("t", 32), "upstream-key", upstream.URL+"/v1", 1000, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	run := func(token string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Injected", "secret")
		p.ServeHTTP(w, r)
		return w.Code
	}
	if run("wrong") != 401 || calls != 0 {
		t.Fatal("unauthenticated call")
	}
	if run(strings.Repeat("t", 32)) != 200 || store.finished != 1 {
		t.Fatal("valid call")
	}
	store.err = database.ErrPilotLimit
	if run(strings.Repeat("t", 32)) != 429 || calls != 1 {
		t.Fatal("budget bypass")
	}
	store.err = nil
	status = 500
	if run(strings.Repeat("t", 32)) != 502 || store.reserved != 2 || store.finished != 1 {
		t.Fatal("failed call refunded or released")
	}
	status = 200
	payload = `{}`
	if run(strings.Repeat("t", 32)) != 502 || store.finished != 1 {
		t.Fatal("missing completion released")
	}
	if _, err := NewProvider(store, strings.Repeat("t", 32), "key", upstream.URL+"/v1", 100, 2, false); err == nil {
		t.Fatal("production custom endpoint")
	}
}
func TestProviderTerminalStream(t *testing.T) {
	valid := `data: {"type":"response.completed","response":{"model":"gpt-5.6-luna","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n"
	if !terminalResponse("/v1/responses", []byte(valid)) {
		t.Fatal("valid completion")
	}
	for _, raw := range []string{"", valid + valid, strings.Replace(valid, "gpt-5.6-luna", "other", 1), strings.Replace(valid, `"output_tokens":1`, `"output_tokens":99999`, 1)} {
		if terminalResponse("/v1/responses", []byte(raw)) {
			t.Fatal("invalid completion")
		}
	}
}
