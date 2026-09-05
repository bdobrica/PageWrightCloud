package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestVersionDeletionDisabledWithoutSideEffects(t *testing.T) {
	// Nil clients/database deliberately make accidental side effects panic.
	h := NewVersionsHandler(nil, nil, nil, 25)
	w := httptest.NewRecorder()
	h.DeleteVersion(w, httptest.NewRequest("DELETE", "/sites/site/versions/version", nil))
	if w.Code != 501 {
		t.Fatalf("deletion status %d", w.Code)
	}
}
