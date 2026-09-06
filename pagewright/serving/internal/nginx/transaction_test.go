package nginx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigTransactionRollbackAndRecovery(t *testing.T) {
	for _, existing := range []bool{false, true} {
		dir := t.TempDir()
		m := NewManager(dir, "true", "/tmp/503.html")
		name := "site.example.test"
		path := filepath.Join(dir, name)
		if existing {
			require.NoError(t, atomicConfig(dir, name, []byte("prior bytes")))
		}
		// A persistent reload failure leaves a recovery journal and blocks writes.
		m.reloadCommand = "false"
		require.Error(t, m.CreateSiteConfig(name, "/var/www/site", nil, true))
		require.FileExists(t, filepath.Join(dir, journalName))
		data, exists, err := readConfig(path)
		require.NoError(t, err)
		require.Equal(t, existing, exists)
		if existing {
			require.Equal(t, "prior bytes", string(data))
		}
		m.reloadCommand = "true"
		require.Error(t, m.CreateSiteConfig("other.example.test", "/var/www/other", nil, true))
		require.NoError(t, RecoverConfigs(dir))
		require.NoFileExists(t, filepath.Join(dir, journalName))
		require.NoError(t, m.CreateSiteConfig(name, "/var/www/site", nil, true))
	}
}

func TestWriterLockExcludesSecondSupervisor(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireWriter(dir)
	require.NoError(t, err)
	defer first.Close()
	_, err = AcquireWriter(dir)
	require.Error(t, err)
	require.NoError(t, first.Close())
	second, err := AcquireWriter(dir)
	require.NoError(t, err)
	require.NoError(t, second.Close())
}

func TestValidationFailureRestoresBeforeReload(t *testing.T) {
	dir := t.TempDir()
	name := "site.example.test"
	require.NoError(t, atomicConfig(dir, name, []byte("old valid bytes")))
	// Test-only validator accepts only the restored config, not the proposed one.
	script := filepath.Join(t.TempDir(), "validate")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ngrep -q 'old valid bytes' '"+filepath.Join(dir, name)+"'\n"), 0700))
	m := NewManager(dir, "true", "/tmp/503.html")
	m.SetLifecycle(script, "")
	require.Error(t, m.CreateSiteConfig(name, "/var/www/site", nil, true))
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	require.Equal(t, "old valid bytes", string(data))
	require.NoFileExists(t, filepath.Join(dir, journalName))
}

func TestRestartRestoresInterruptedConfigAndGeneration(t *testing.T) {
	dir := t.TempDir()
	records := []backup{{Name: "site.example.test", Exists: true, Data: []byte("old")}, {Name: healthName, Exists: true, Data: healthConfig("previous")}}
	data, err := json.Marshal(records)
	require.NoError(t, err)
	require.NoError(t, atomicConfig(dir, journalName, data))
	require.NoError(t, atomicConfig(dir, "site.example.test", []byte("unacknowledged")))
	require.NoError(t, atomicConfig(dir, healthName, healthConfig("new")))
	require.NoError(t, RecoverConfigs(dir))
	got, err := os.ReadFile(filepath.Join(dir, "site.example.test"))
	require.NoError(t, err)
	require.Equal(t, "old", string(got))
	got, err = os.ReadFile(filepath.Join(dir, healthName))
	require.NoError(t, err)
	require.Equal(t, healthConfig("previous"), got)
	require.NoError(t, RecoverConfigs(dir))
}

func TestBadRecoveryJournalFailsClosed(t *testing.T) {
	for _, content := range []string{"invalid", `[{"Name":"../outside","Exists":false}]`, `[]`} {
		dir := t.TempDir()
		require.NoError(t, atomicConfig(dir, journalName, []byte(content)))
		require.Error(t, RecoverConfigs(dir))
		require.FileExists(t, filepath.Join(dir, journalName))
	}
}

func TestConfigChangesSerializeAndRejectInjection(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, "true", "/tmp/503.html")
	var wg sync.WaitGroup
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- m.CreateSiteConfig("site.example.test", "/var/www/site", nil, true) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.NoFileExists(t, filepath.Join(dir, journalName))
	require.Error(t, m.CreateSiteConfig("../site", "/var/www/site", nil, true))
	require.Error(t, m.CreateSiteConfig("site.example.test", "/var/www/site;", nil, true))
	require.Error(t, m.CreateSiteConfig("site.example.test", "/var/www/site", []string{"bad;directive"}, true))
}
