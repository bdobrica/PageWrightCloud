//go:build integration

package codex

// The source-only executor exists only in the disposable service test image.
// Installed CLI acceptance uses the production namespace, not this build tag.
const isolateCLI = false
