//go:build integration

// This executable is built only into the integration test image, never a worker image.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute() error {
	// This build-tag-only fixture stands in for the CLI, including its preflight.
	if len(os.Args) > 1 && os.Args[1] == "sandbox" {
		return nil
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil || len(os.Args) < 3 || os.Args[1] != "exec" || string(prompt) != "Set the homepage heading to Deterministic M1 round trip." {
		return fmt.Errorf("unsupported fixture request")
	}
	// Edit source only: all public bytes must come from the real compiler.
	if err := os.WriteFile("content/home/index.md", []byte("# Deterministic M1 round trip\n\nCompiled from bootstrapped source.\n"), 0644); err != nil {
		return err
	}
	cmd := exec.Command("/usr/local/bin/pagewrightc", "build", "--theme", "/workspace/pagewright/themes/starter", "--content", "content", "--out", "public")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("SUMMARY: Deterministic source edit compiled successfully")
	return nil
}
