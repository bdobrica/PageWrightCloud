package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestInvalidStartup(t *testing.T) {
	if os.Getenv("PAGEWRIGHT_STARTUP_TEST") == "child" {
		main()
		return
	}
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"missing", nil, "PAGEWRIGHT_JWT_SECRET"},
		{"incomplete SMTP", []string{"PAGEWRIGHT_JWT_SECRET=" + strings.Repeat("s", 32), "PAGEWRIGHT_DATABASE_URL=postgres://pilot:sentinel-private-password@127.0.0.1/db?sslmode=disable", "PAGEWRIGHT_STORAGE_URL=http://storage", "PAGEWRIGHT_MANAGER_URL=http://manager", "PAGEWRIGHT_SERVING_URL=http://serving", "PAGEWRIGHT_SMTP_ADDRESS=smtp.example.test:587", "PAGEWRIGHT_SMTP_PASSWORD=sentinel-private-password"}, "SMTP/reset configuration"},
		{"invalid origins", []string{"PAGEWRIGHT_JWT_SECRET=" + strings.Repeat("s", 32), "PAGEWRIGHT_DATABASE_URL=postgres://pilot:sentinel-private-password@127.0.0.1/db?sslmode=disable", "PAGEWRIGHT_STORAGE_URL=http://storage", "PAGEWRIGHT_MANAGER_URL=http://manager", "PAGEWRIGHT_SERVING_URL=http://serving", "PAGEWRIGHT_APP_ORIGINS=*"}, "PAGEWRIGHT_APP_ORIGINS"},
		{"malformed database", []string{"PAGEWRIGHT_JWT_SECRET=" + strings.Repeat("s", 32), "PAGEWRIGHT_DATABASE_URL=postgres://sentinel-private%invalid@host/db"}, "PostgreSQL configuration"},
		{"missing services", []string{"PAGEWRIGHT_JWT_SECRET=" + strings.Repeat("s", 32), "PAGEWRIGHT_DATABASE_URL=postgres://pilot:sentinel-private-password@127.0.0.1/db?sslmode=disable"}, "PAGEWRIGHT_"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestInvalidStartup$")
			cmd.Env = append([]string{"PAGEWRIGHT_STARTUP_TEST=child", "PAGEWRIGHT_SERVICE_TOKEN=startup-fixture-internal-service-token"}, tc.env...)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("invalid startup succeeded")
			}
			if strings.Contains(string(output), "sentinel-private") {
				t.Fatal("startup leaked a supplied secret")
			}
			if !strings.Contains(string(output), tc.want) || strings.Contains(string(output), "Failed to connect") {
				t.Fatal("startup did not reject configuration before database I/O")
			}
		})
	}
}
