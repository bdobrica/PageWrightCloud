//go:build integration

package nginx

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRealNginxValidationAcknowledgementAndRollback(t *testing.T) {
	dir := t.TempDir()
	sites := filepath.Join(dir, "sites")
	require.NoError(t, PrepareRuntime(sites))
	config := filepath.Join(dir, "nginx.conf")
	require.NoError(t, os.WriteFile(config, []byte(fmt.Sprintf("pid %s; error_log stderr; events {} http { include %s/*; }", filepath.Join(dir, "nginx.pid"), sites)), 0600))
	child := exec.Command("nginx", "-c", config, "-g", "daemon off;")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child.Stderr = os.Stderr
	require.NoError(t, child.Start())
	t.Cleanup(func() { _ = syscall.Kill(-child.Process.Pid, syscall.SIGTERM); _ = child.Wait() })
	m := NewManager(sites, "nginx -s reload -c "+config, "/tmp/503.html")
	m.SetLifecycle("nginx -t -c "+config, "http://127.0.0.1:8089/")
	require.NoError(t, m.awaitGeneration("startup"))
	require.NoError(t, m.CreateSiteConfig("site.example.test", "/var/www/site", nil, true))
	original, err := os.ReadFile(filepath.Join(sites, "site.example.test"))
	require.NoError(t, err)
	require.Error(t, m.change("site.example.test", []byte("not valid nginx syntax;"), false))
	restored, err := os.ReadFile(filepath.Join(sites, "site.example.test"))
	require.NoError(t, err)
	require.Equal(t, original, restored)
	require.NoFileExists(t, filepath.Join(sites, journalName))
	// A successful no-op command is not sufficient: nginx must serve the nonce.
	m.reloadCommand = "true"
	require.Error(t, m.CreateSiteConfig("site.example.test", "/var/www/other", nil, true))
	restored, err = os.ReadFile(filepath.Join(sites, "site.example.test"))
	require.NoError(t, err)
	require.Equal(t, original, restored)
	require.NoFileExists(t, filepath.Join(sites, journalName))
	require.NoError(t, m.Ready())
	// The internal generation endpoint cannot be obtained on the public site port.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1/", nil)
	require.NoError(t, err)
	request.Host = "site.example.test"
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 404, response.StatusCode)
}
