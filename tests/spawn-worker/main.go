// Docker spawner acceptance fixture; never shipped in a production image.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := check(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("spawn fixture passed")
}
func check() error {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal([]byte(os.Getenv("PAGEWRIGHT_JOB")), &wire); err != nil {
		return err
	}
	for _, key := range []string{"job_id", "site_id", "owner_id", "source_version", "target_version", "prompt"} {
		var value string
		if err := json.Unmarshal(wire[key], &value); err != nil || value == "" {
			return fmt.Errorf("missing identity %s", key)
		}
	}
	if os.Getenv("PAGEWRIGHT_LLM_KEY") != "worker-only-secret" || os.Getenv("PAGEWRIGHT_JWT_SECRET") != "" || os.Getenv("PAGEWRIGHT_REDIS_PASSWORD") != "" {
		return fmt.Errorf("incorrect credential scope")
	}
	if _, err := os.Stat("/var/run/docker.sock"); !os.IsNotExist(err) {
		return fmt.Errorf("worker has Docker socket")
	}
	work := os.Getenv("PAGEWRIGHT_WORK_DIR")
	if work != "/work" {
		return fmt.Errorf("wrong work directory")
	}
	if err := os.WriteFile(filepath.Join(work, "fixture.txt"), []byte("isolated workspace"), 0600); err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, key := range []string{"PAGEWRIGHT_MANAGER_URL", "PAGEWRIGHT_STORAGE_URL"} {
		resp, err := client.Get(os.Getenv(key) + "/health")
		if err != nil {
			return fmt.Errorf("cannot reach %s", key)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("unhealthy %s", key)
		}
	}
	return nil
}
