package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBoundedJSONWholeRepresentation(t *testing.T) {
	for _, body := range []string{
		"{", `{"value":"ok"} {}`, `{"unknown":true}`,
		`{"value":"` + strings.Repeat("a", 1<<20) + `"}`,
		`{"value":"ok"}` + strings.Repeat(" ", 1<<20),
	} {
		var value struct {
			Value string `json:"value"`
		}
		if boundedJSON(httptest.NewRequest("POST", "/", strings.NewReader(body)), &value) == nil {
			t.Fatal("invalid or oversized representation accepted")
		}
	}
	var value struct {
		Value string `json:"value"`
	}
	if err := boundedJSON(httptest.NewRequest("POST", "/", strings.NewReader(`{"value":"ok"} `)), &value); err != nil || value.Value != "ok" {
		t.Fatal(err)
	}
}
