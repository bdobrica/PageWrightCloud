// Package pilot implements the restricted, prepaid provider boundary for the pilot.
package pilot

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/google/uuid"
)

type Store interface {
	ReservePilotProvider(context.Context, string, int, int) error
	FinishPilotProvider(context.Context, string) error
}
type Provider struct {
	store                  Store
	token, key, upstream   string
	allowance, concurrency int
	client                 *http.Client
}

func NewProvider(store Store, token, key, upstream string, allowance, concurrency int, development bool) (*Provider, error) {
	u, err := url.Parse(upstream)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/v1" || (u.String() != "https://api.openai.com/v1" && !development) {
		return nil, errors.New("pilot provider must use the reviewed OpenAI /v1 endpoint")
	}
	if allowance > 0 && (len(token) < 32 || key == "") {
		return nil, errors.New("provider allowance requires a provider key and separate internal token")
	}
	return &Provider{store: store, token: token, key: key, upstream: strings.TrimSuffix(upstream, "/"), allowance: allowance, concurrency: concurrency,
		client: &http.Client{Timeout: 120 * time.Second, Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}}, nil
}
func reject(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status == 429 {
		w.Header().Set("Retry-After", "60")
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]string{"message": message, "type": "pilot_limit"}})
}
func (p *Provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || (r.URL.Path != "/v1/responses" && r.URL.Path != "/v1/chat/completions") || r.URL.RawQuery != "" {
		reject(w, 404, "Unsupported provider endpoint")
		return
	}
	if p.token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+p.token)) != 1 {
		reject(w, 401, "Unauthorized")
		return
	}
	if p.allowance < 100 {
		reject(w, 429, "AI allowance exhausted or disabled; contact the operator")
		return
	}
	if r.Header.Get("Content-Encoding") != "" {
		reject(w, 400, "Encoded requests are unsupported")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		reject(w, 400, "Provider request too large")
		return
	}
	body, err := Constrain(r.URL.Path, raw)
	if err != nil {
		log.Printf("Pilot provider policy rejection: %v", err) // Fixed categories only; no request data.
		reject(w, 400, "Provider request outside reviewed policy")
		return
	}
	id := uuid.NewString()
	if err = p.store.ReservePilotProvider(r.Context(), id, p.allowance, p.concurrency); err != nil {
		if errors.Is(err, database.ErrPilotLimit) {
			reject(w, 429, "AI allowance or concurrency limit reached; contact the operator")
		} else {
			reject(w, 503, "AI allowance checks unavailable")
		}
		return
	}
	// Retain active reservation on any uncertain upstream outcome. A crash or
	// timeout cannot open an overlapping slot while the provider may still work.
	completed := false
	defer func() {
		if completed {
			ctx, cancel := database.PilotCleanupContext()
			defer cancel()
			_ = p.store.FinishPilotProvider(ctx, id)
		}
	}()
	req, err := http.NewRequestWithContext(r.Context(), "POST", p.upstream+strings.TrimPrefix(r.URL.Path, "/v1"), bytes.NewReader(body))
	if err != nil {
		reject(w, 502, "Provider request failed")
		return
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	req.Header.Set("Content-Type", "application/json")
	result, err := p.client.Do(req)
	if err != nil {
		reject(w, 502, "Provider outcome uncertain; operator review required")
		return
	}
	defer result.Body.Close()
	response, err := io.ReadAll(io.LimitReader(result.Body, (4<<20)+1))
	if err != nil || len(response) > 4<<20 {
		reject(w, 502, "Provider outcome uncertain; operator review required")
		return
	}
	if result.StatusCode != 200 {
		reject(w, 502, "Provider rejected request; allowance retained")
		return
	}
	if !terminalResponse(r.URL.Path, response) {
		log.Print("Pilot provider completion unverified")
		reject(w, 502, "Provider completion unverified; operator review required")
		return
	}
	completed = true
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/v1/responses" {
		w.Header().Set("Content-Type", "text/event-stream")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Write(response)
}

func terminalResponse(path string, raw []byte) bool {
	if path == "/v1/chat/completions" {
		var result struct {
			Choices []struct {
				Finish string `json:"finish_reason"`
			} `json:"choices"`
		}
		return json.Unmarshal(raw, &result) == nil && len(result.Choices) == 1 && (result.Choices[0].Finish == "stop" || result.Choices[0].Finish == "length")
	}
	count := 0
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		var event struct {
			Type     string `json:"type"`
			Response struct {
				Model  string `json:"model"`
				Status string `json:"status"`
				Usage  struct {
					Input  int `json:"input_tokens"`
					Output int `json:"output_tokens"`
					Total  int `json:"total_tokens"`
				} `json:"usage"`
			} `json:"response"`
		}
		if json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &event) != nil {
			return false
		}
		if event.Type == "response.completed" {
			count++
			u := event.Response.Usage
			if event.Response.Model != "gpt-5.6-luna" || event.Response.Status != "completed" || u.Input < 0 || u.Input > 1050000 || u.Output < 0 || u.Output > 8192 || u.Total != u.Input+u.Output || u.Total == 0 {
				return false
			}
		}
	}
	return count == 1
}

// A $1 reservation exceeds full 1.05M-context Luna input at the reviewed highest
// standard cache-write rate ($0.50/M), plus 8192 output tokens at $1.80/M.
// Chat uses only text gpt-3.5-turbo, one completion and <=500 output tokens.
// See docs/PILOT_LIMITS.md for dated pricing assumptions and operational limits.
func Constrain(path string, raw []byte) ([]byte, error) {
	var body map[string]interface{}
	if json.Unmarshal(raw, &body) != nil || body == nil {
		return nil, errors.New("invalid JSON")
	}
	allowed := ""
	if path == "/v1/chat/completions" {
		if body["model"] != "gpt-3.5-turbo" {
			return nil, errors.New("model")
		}
		allowed = "model messages max_tokens temperature"
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) == 0 {
			return nil, errors.New("messages")
		}
		for _, v := range messages {
			m, ok := v.(map[string]interface{})
			if !ok {
				return nil, errors.New("message")
			}
			if _, ok = m["content"].(string); !ok {
				return nil, errors.New("text required")
			}
			for k := range m {
				if k != "role" && k != "content" {
					return nil, errors.New("message field")
				}
			}
		}
		body["max_tokens"] = 500
	} else if path == "/v1/responses" {
		if body["model"] != "gpt-5.6-luna" {
			return nil, errors.New("model")
		}
		delete(body, "client_metadata") // CLI-local attribution; never forwarded.
		allowed = "model input instructions tools tool_choice parallel_tool_calls reasoning text stream store include prompt_cache_key max_output_tokens service_tier"
		if ts, exists := body["tools"]; exists {
			tools, ok := ts.([]interface{})
			if !ok {
				return nil, errors.New("tools")
			}
			for _, tool := range tools {
				if !localTool(tool) {
					return nil, errors.New("hosted tool")
				}
			}
		}
		if !localInput(body["input"]) {
			return nil, errors.New("nonlocal input")
		}
		body["max_output_tokens"] = 8192
		body["service_tier"] = "default"
		body["store"] = false
		body["stream"] = true
		body["reasoning"] = map[string]string{"effort": "low"}
	} else {
		return nil, errors.New("endpoint")
	}
	for k := range body {
		if !strings.Contains(" "+allowed+" ", " "+k+" ") {
			return nil, errors.New("field")
		}
	}
	return json.Marshal(body)
}
func localTool(v interface{}) bool {
	tool, ok := v.(map[string]interface{})
	if !ok {
		return false
	}
	switch tool["type"] {
	case "function", "custom", "local_shell":
		return true
	case "namespace":
		children, ok := tool["tools"].([]interface{})
		if !ok {
			return false
		}
		for _, child := range children {
			if !localTool(child) {
				return false
			}
		}
		return true
	}
	return false
}
func localInput(v interface{}) bool {
	switch value := v.(type) {
	case string, nil:
		return true
	case []interface{}:
		for _, child := range value {
			if !localInput(child) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		if kind, ok := value["type"]; ok {
			switch kind {
			case "additional_tools":
				// The pinned CLI supplies deferred local definitions as input.
				// Validate definitions, not their JSON-schema parameter types.
				children, ok := value["tools"].([]interface{})
				if !ok {
					return false
				}
				for _, child := range children {
					if !localTool(child) {
						return false
					}
				}
				for key := range value {
					if key == "id" || key == "role" {
						if _, ok := value[key].(string); !ok {
							return false
						}
						continue
					}
					if key != "type" && key != "tools" {
						return false
					}
				}
				return true
			case "message", "input_text", "output_text", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "reasoning", "summary_text", "refusal":
			default:
				return false
			}
		}
		for key, child := range value {
			if key == "file_id" || key == "image_url" {
				return false
			}
			switch child.(type) {
			case map[string]interface{}, []interface{}:
				if !localInput(child) {
					return false
				}
			}
		}
		return true
	default:
		return false
	}
}
