//go:build integration

// This executable is built only into the integration test image, never a worker image.
package main

import (
	"fmt"
	"io"
	"os"
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
	if err != nil || len(os.Args) < 3 || os.Args[1] != "exec" {
		return fmt.Errorf("unsupported fixture request")
	}
	// Edit source only: all public bytes must come from the real compiler.
	var source []byte
	switch string(prompt) {
	case "Set the homepage heading to Deterministic M1 round trip.":
		source = []byte("# Deterministic M1 round trip\n\nCompiled from bootstrapped source.\n")
	case "Append a second unpublished edit.":
		source, err = os.ReadFile("content/home/index.md")
		if err != nil {
			return err
		}
		source = append(source, []byte("\nSecond unpublished edit preserved.\n")...)
	default:
		return fmt.Errorf("unsupported fixture request")
	}
	if err := os.WriteFile("content/home/index.md", source, 0644); err != nil {
		return err
	}
	fmt.Println("SUMMARY: Deterministic source edit completed")
	return nil
}
