package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/artifact"
	"github.com/stretchr/testify/require"
)

func TestInvalidHostRejectedBeforeDependencies(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	for _, host := range []string{"a..test", "a-.test", "a_test.test", "preview.a.test", "a.test%3B", "a.test%0Aroot"} {
		w := httptest.NewRecorder()
		h.SetupRoutes().ServeHTTP(w, httptest.NewRequest("POST", "/sites/"+host+"/disable", nil))
		require.Equal(t, 400, w.Code, host)
	}
}

func TestDeploymentRejectsSymlinkBeforeReceiptWrite(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "example.test"), 0700))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "example.test", "site.example.test")))
	h := NewHandler(artifact.NewManager(root, 3), nil, nil)
	w := httptest.NewRecorder()
	h.SetupRoutes().ServeHTTP(w, httptest.NewRequest("POST", "/sites/site.example.test/deployment", strings.NewReader(`{"site_id":"site","fqdn":"site.example.test","version":"v1","sequence":1,"target":"live","status":"pending"}`)))
	require.Equal(t, 400, w.Code)
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestReceiptSymlinkRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt")
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "outside"), path))
	_, err := readReceipt(path)
	require.ErrorContains(t, err, "unsafe")
}

func TestInvalidVersionBeforeNginxOrDownload(t *testing.T) {
	h := NewHandler(artifact.NewManager(t.TempDir(), 3), nil, nil)
	for _, action := range []string{"activate", "preview", "artifacts"} {
		for _, id := range []string{"../outside", "a/b", strings.Repeat("a", 201)} {
			w := httptest.NewRecorder()
			body := `{"site_id":"site","version":"` + id + `"}`
			h.SetupRoutes().ServeHTTP(w, httptest.NewRequest("POST", "/sites/site.example.test/"+action, strings.NewReader(body)))
			require.Equal(t, 400, w.Code, action)
		}
	}
}
