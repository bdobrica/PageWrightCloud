//go:build compiler_acceptance

package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstalledTrustedInputs(t *testing.T) {
	require.Equal(t, 1000, os.Getuid())
	require.Equal(t, DefaultCompiler, testCompiler)
	require.Equal(t, DefaultTheme, testTheme)
	for _, name := range []string{DefaultCompiler, filepath.Join(DefaultTheme, "tokens.json")} {
		f, err := os.OpenFile(name, os.O_WRONLY, 0)
		if f != nil {
			f.Close()
		}
		require.Error(t, err, "trusted input must not be writable: %s", name)
	}
	f, err := os.CreateTemp(DefaultTheme, "unauthorized-")
	if f != nil {
		f.Close()
		os.Remove(f.Name())
	}
	require.Error(t, err, "theme directory must not be writable")
}
