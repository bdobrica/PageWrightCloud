package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelper(t *testing.T) {
	if os.Getenv("PAGEWRIGHT_COMPILER_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"pagewrightc"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(99)
}
func TestCLIContract(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "home"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "site.json"), []byte(`{"site_name":"CLI Test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "home/index.md"), []byte("# CLI Home"), 0644); err != nil {
		t.Fatal(err)
	}
	theme := "../../../themes/starter"
	out := filepath.Join(t.TempDir(), "public")
	cases := []struct {
		name    string
		args    []string
		code    int
		message string
	}{
		{"help", []string{"help"}, 0, "Usage:"},
		{"version", []string{"version"}, 0, "pagewrightc version"},
		{"unknown", []string{"unknown"}, 1, "unknown command"},
		{"missing-theme", []string{"build"}, 1, "--theme is required"},
		{"missing-content", []string{"build", "--theme", theme}, 1, "--content is required"},
		{"unknown-flag", []string{"build", "--no-such-flag"}, 2, "flag provided but not defined"},
		{"build", []string{"build", "--theme", theme, "--content", source, "--out", out}, 0, "Build complete!"},
		{"existing-output", []string{"build", "--theme", theme, "--content", source, "--out", out}, 1, "output directory must be absent or empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-test.run=^TestCLIHelper$", "--"}, tc.args...)
			cmd := exec.Command(os.Args[0], args...)
			cmd.Env = append(os.Environ(), "PAGEWRIGHT_COMPILER_TEST_PROCESS=1")
			output, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				exit, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != tc.code || !strings.Contains(string(output), tc.message) {
				t.Fatalf("exit=%d want=%d: %s", code, tc.code, output)
			}
		})
	}
	data, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !strings.Contains(string(data), "<title>CLI Home</title>") {
		t.Fatal("CLI did not preserve complete output")
	}
}
