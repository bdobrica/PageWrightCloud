package nginx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const journalName = ".pagewright-transaction"
const healthName = "000-pagewright-health"

var configName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
var safePath = regexp.MustCompile(`^/[A-Za-z0-9/._-]+$`)
var generationPattern = regexp.MustCompile(`return 200 '([a-zA-Z0-9]+)'`)

// AcquireWriter prevents a second supported supervisor from recovering or
// changing the same config volume while the first one is alive.
func AcquireWriter(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".pagewright-writer.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("hosting configuration already has a writer: %w", err)
	}
	return f, nil
}

func validateSite(name, path string, aliases []string) error {
	if !configName.MatchString(name) || len(name) > 245 || strings.HasPrefix(strings.ToLower(name), "preview.") || !strings.Contains(name, ".") || !safePath.MatchString(path) || len(aliases) > 100 {
		return fmt.Errorf("invalid nginx site parameters")
	}
	for _, alias := range aliases {
		if !configName.MatchString(alias) || strings.HasPrefix(strings.ToLower(alias), "preview.") {
			return fmt.Errorf("invalid alias")
		}
	}
	return nil
}

// SetLifecycle is called once, before serving requests. Unit fixtures may use
// command stubs; production always validates nginx and probes its new workers.
func (m *Manager) SetLifecycle(validation, probe string) {
	m.validationCommand, m.probeURL = validation, probe
}

func runCommand(command string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty nginx command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := exec.CommandContext(ctx, parts[0], parts[1:]...).CombinedOutput(); err != nil {
		return fmt.Errorf("nginx command failed: %w", err)
	}
	return nil
}

type backup struct {
	Name   string
	Exists bool
	Data   []byte
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func atomicConfig(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, ".config-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0644); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	return syncDir(dir)
}

func readConfig(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("config is not a regular file")
	}
	data, err := os.ReadFile(path)
	return data, true, err
}

func removeConfig(dir, name string) error {
	if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDir(dir)
}

func restore(dir string, records []backup) error {
	for _, b := range records {
		if b.Exists {
			if err := atomicConfig(dir, b.Name, b.Data); err != nil {
				return err
			}
		} else if err := removeConfig(dir, b.Name); err != nil {
			return err
		}
	}
	return nil
}

// RecoverConfigs runs before nginx starts. An interrupted transaction always
// restores its prior bytes, even if nginx had consumed the unacknowledged change.
func RecoverConfigs(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, exists, err := readConfig(filepath.Join(dir, journalName))
	if err != nil || !exists {
		return err
	}
	var records []backup
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("invalid recovery journal: %w", err)
	}
	if len(records) < 1 || len(records) > 2 {
		return fmt.Errorf("invalid recovery entries")
	}
	seen := map[string]bool{}
	for _, b := range records {
		if !configName.MatchString(b.Name) || seen[b.Name] {
			return fmt.Errorf("invalid recovery name")
		}
		seen[b.Name] = true
	}
	if err := restore(dir, records); err != nil {
		return err
	}
	return removeConfig(dir, journalName)
}

func healthConfig(token string) []byte {
	return []byte(fmt.Sprintf("server { listen 127.0.0.1:8089; server_name localhost; location / { default_type text/plain; return 200 '%s'; } }\n", token))
}

// PrepareRuntime does not overwrite site files. The internal generation endpoint
// is not exposed by either the public proxy or a published container port.
func PrepareRuntime(dir string) error {
	if err := RecoverConfigs(dir); err != nil {
		return err
	}
	return atomicConfig(dir, healthName, healthConfig("startup"))
}

func (m *Manager) probe(expected string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", m.probeURL, nil)
	if err != nil {
		return err
	}
	// No keepalive: each attempt must reach the newly loaded worker generation.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	if err != nil {
		return err
	}
	if response.StatusCode != 200 || (expected != "" && string(body) != expected) {
		return fmt.Errorf("nginx generation not ready")
	}
	return nil
}

func (m *Manager) Ready() error {
	if m.probeURL == "" {
		return nil
	}
	if _, err := os.Lstat(filepath.Join(m.sitesEnabledDir, journalName)); err == nil {
		return fmt.Errorf("nginx transaction pending")
	} else if !os.IsNotExist(err) {
		return err
	}
	return m.probe("")
}

func (m *Manager) awaitGeneration(token string) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := m.probe(token); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("nginx did not acknowledge generation")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (m *Manager) change(name string, data []byte, remove bool) error {
	dir := m.sitesEnabledDir
	if !configName.MatchString(name) || name == healthName {
		return fmt.Errorf("invalid site config name")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(dir, journalName)); err == nil {
		return fmt.Errorf("configuration recovery required")
	} else if !os.IsNotExist(err) {
		return err
	}
	old, exists, err := readConfig(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	records := []backup{{Name: name, Exists: exists, Data: old}}
	token := ""
	oldToken := ""
	if m.probeURL != "" {
		oldHealth, exists, err := readConfig(filepath.Join(dir, healthName))
		if err != nil {
			return err
		}
		matched := generationPattern.FindSubmatch(oldHealth)
		if !exists || len(matched) != 2 {
			return fmt.Errorf("missing or invalid generation config")
		}
		oldToken = string(matched[1])
		records = append(records, backup{Name: healthName, Exists: exists, Data: oldHealth})
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		token = hex.EncodeToString(nonce)
	}
	journal, err := json.Marshal(records)
	if err != nil {
		return err
	}
	if err = atomicConfig(dir, journalName, journal); err != nil {
		return err
	}
	rollback := func(cause error) error {
		if err := restore(dir, records); err != nil {
			return fmt.Errorf("%v; rollback failed: %w", cause, err)
		}
		if err := m.Reload(); err != nil {
			return fmt.Errorf("%v; rollback reload failed: %w", cause, err)
		}
		if oldToken != "" {
			if err := m.awaitGeneration(oldToken); err != nil {
				return fmt.Errorf("%v; rollback unconfirmed: %w", cause, err)
			}
		}
		// Retaining a journal on rollback failure forces recovery before more writes.
		if err := removeConfig(dir, journalName); err != nil {
			return err
		}
		return cause
	}
	if remove {
		err = removeConfig(dir, name)
	} else {
		err = atomicConfig(dir, name, data)
	}
	if err != nil {
		return rollback(err)
	}
	if token != "" {
		if err = atomicConfig(dir, healthName, healthConfig(token)); err != nil {
			return rollback(err)
		}
	}
	if err = m.Reload(); err != nil {
		return rollback(err)
	}
	if token != "" {
		if err = m.awaitGeneration(token); err != nil {
			return rollback(err)
		}
	}
	return removeConfig(dir, journalName)
}
