package api

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidPathIdentityBeforeQueue(t *testing.T) {
	h := &Handler{}
	for _, field := range []string{"site_id", "source_version", "target_version"} {
		for _, id := range []string{"../escape", "a/b", "a%2fb", strings.Repeat("a", 201)} {
			body := fmt.Sprintf(`{"site_id":"site","source_version":"initial","target_version":"v1","owner_id":"owner","prompt":"test",%q:%q}`, field, id)
			w := httptest.NewRecorder()
			h.CreateJob(w, httptest.NewRequest("POST", "/jobs", strings.NewReader(body)))
			if w.Code != 400 {
				t.Fatalf("%s: got %d", field, w.Code)
			}
		}
	}
}
