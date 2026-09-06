package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/artifact"
	"github.com/stretchr/testify/require"
)

var testCompiler, testTheme string

func TestMain(m *testing.M) {
	testTheme = DefaultTheme
	testCompiler, _ = exec.LookPath("pagewrightc")
	var temp string
	if testCompiler == "" {
		var err error
		temp, err = os.MkdirTemp("", "pagewright-compiler-test-")
		if err != nil {
			panic(err)
		}
		testCompiler = filepath.Join(temp, "pagewrightc")
		cmd := exec.Command("go", "build", "-o", testCompiler, "./cmd/pagewrightc")
		cmd.Dir = "../../../compiler"
		if output, err := cmd.CombinedOutput(); err != nil {
			os.RemoveAll(temp)
			panic(string(output))
		}
	}
	if _, err := os.Stat(testTheme); err != nil {
		testTheme, _ = filepath.Abs("../../../themes/starter")
		// Normalize the inherited repository alias, not user-selected inputs.
		testTheme, err = filepath.EvalSymlinks(testTheme)
		if err != nil {
			panic(err)
		}
	}
	code := m.Run()
	if temp != "" {
		os.RemoveAll(temp)
	}
	os.Exit(code)
}

func fixture(t *testing.T) (string, artifact.Snapshot) {
	t.Helper()
	site := filepath.Join(t.TempDir(), "site")
	for name, data := range map[string]string{
		"content/site.json":      `{"site_name":"M2.4 fixture"}`,
		"content/home/index.md":  "# Original\n",
		"public/index.html":      "<html><body>Previous output</body></html>",
		"public/stale.html":      "<html><body>Stale output</body></html>",
		"manifest.json":          `{"schema_version":1,"theme_id":"starter","kind":"compiled"}`,
		".codex/instructions.md": "trusted instructions",
	} {
		write(t, filepath.Join(site, name), data)
	}
	before, err := artifact.SnapshotWorkspace(site)
	require.NoError(t, err)
	return site, before
}

func write(t *testing.T, name, data string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
	require.NoError(t, os.WriteFile(name, []byte(data), 0644))
}

func TestCompileFrozenSourceAndFreshOutput(t *testing.T) {
	site, before := fixture(t)
	write(t, filepath.Join(site, "content/home/index.md"), "# Changed by executor\n")
	archive := filepath.Join(filepath.Dir(site), "output.tar.gz")
	result, err := Compile(context.Background(), site, testCompiler, testTheme, archive, before)
	require.NoError(t, err)
	require.Equal(t, []string{"content/home/index.md"}, result.FilesChanged)
	require.Equal(t, "compiled", result.Layout.Kind)
	require.Len(t, result.Checks, 4)
	data, err := os.ReadFile(filepath.Join(site, "public/index.html"))
	require.NoError(t, err)
	require.Contains(t, string(data), "Changed by executor")
	_, err = os.Stat(filepath.Join(site, "public/stale.html"))
	require.True(t, os.IsNotExist(err))
	unpacked := filepath.Join(filepath.Dir(site), "unpacked")
	require.NoError(t, artifact.Unpack(archive, unpacked))
	source, err := os.ReadFile(filepath.Join(unpacked, "content/home/index.md"))
	require.NoError(t, err)
	require.Equal(t, "# Changed by executor\n", string(source))
	again, err := artifact.SnapshotWorkspace(site)
	require.NoError(t, err)
	write(t, filepath.Join(site, "content/home/index.md"), ":::component MissingComponent\n:::\n")
	_, err = Compile(context.Background(), site, testCompiler, testTheme, archive, again)
	require.Error(t, err)
	preserved, err := os.ReadFile(filepath.Join(site, "public/index.html"))
	require.NoError(t, err)
	require.Equal(t, data, preserved)
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(site), ".compile-*"))
	require.NoError(t, err)
	require.Empty(t, leftovers)
}

func TestRejectInvalidEditsBeforePublication(t *testing.T) {
	for _, name := range []string{"public/index.html", "manifest.json", ".codex/instructions.md", "theme/tokens.json", "content/.secret", "content/site.json", "content/home/index.md"} {
		t.Run(name, func(t *testing.T) {
			site, before := fixture(t)
			// The source case is an invalid component: an allowed diff still must compile.
			write(t, filepath.Join(site, name), ":::component MissingComponent\n:::\n")
			previous, err := os.ReadFile(filepath.Join(site, "public/index.html"))
			require.NoError(t, err)
			archive := filepath.Join(filepath.Dir(site), "output.tar.gz")
			_, err = Compile(context.Background(), site, testCompiler, testTheme, archive, before)
			require.Error(t, err)
			preserved, err := os.ReadFile(filepath.Join(site, "public/index.html"))
			require.NoError(t, err)
			require.Equal(t, previous, preserved)
			_, err = os.Stat(archive)
			require.True(t, os.IsNotExist(err))
		})
	}
	for _, kind := range []string{"symlink", "executable", "missing-home"} {
		t.Run(kind, func(t *testing.T) {
			site, before := fixture(t)
			file := filepath.Join(site, "content/home/index.md")
			switch kind {
			case "symlink":
				require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(site, "content/leak")))
			case "executable":
				require.NoError(t, os.Chmod(file, 0755))
			case "missing-home":
				require.NoError(t, os.Remove(file))
			}
			_, err := Compile(context.Background(), site, testCompiler, testTheme, filepath.Join(filepath.Dir(site), "output.tar.gz"), before)
			require.Error(t, err)
		})
	}
}

func TestMissingAssetPreservesPublic(t *testing.T) {
	site, before := fixture(t)
	write(t, filepath.Join(site, "content/home/index.md"), "# Home\n\n![Missing](/assets/missing.png)\n")
	archive := filepath.Join(filepath.Dir(site), "output.tar.gz")
	_, err := Compile(context.Background(), site, testCompiler, testTheme, archive, before)
	require.ErrorContains(t, err, "missing local output reference")
	data, err := os.ReadFile(filepath.Join(site, "public/index.html"))
	require.NoError(t, err)
	require.Contains(t, string(data), "Previous output")
	_, err = os.Stat(archive)
	require.True(t, os.IsNotExist(err))
}

func TestSourceAssetAndDeletion(t *testing.T) {
	site, _ := fixture(t)
	write(t, filepath.Join(site, "content/obsolete/index.md"), "# Obsolete\n")
	before, err := artifact.SnapshotWorkspace(site)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(site, "content/obsolete/index.md")))
	write(t, filepath.Join(site, "content/home/assets/logo.svg"), "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")
	write(t, filepath.Join(site, "content/home/index.md"), "# Home\n\n![Logo](/assets/pages/home/logo.svg)\n")
	result, err := Compile(context.Background(), site, testCompiler, testTheme, filepath.Join(filepath.Dir(site), "output.tar.gz"), before)
	require.NoError(t, err)
	require.Equal(t, []string{"content/home/assets/logo.svg", "content/home/index.md", "content/obsolete/index.md"}, result.FilesChanged)
	data, err := os.ReadFile(filepath.Join(site, "public/assets/pages/home/logo.svg"))
	require.NoError(t, err)
	require.Equal(t, "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", string(data))
}

func TestOutputChecks(t *testing.T) {
	for _, tc := range []struct {
		name, html string
		valid      bool
	}{
		{"valid", "<html><body><a href='/'>Home</a><img src='/assets/logo.svg'></body></html>", true},
		{"missing", "<html><body><img src='absent.png'></body></html>", false},
		{"private", "<html><body><a href='/content/site.json'>Source</a></body></html>", false},
		{"scheme", "<html><body><a href='javascript:alert(1)'>X</a></body></html>", false},
		{"fragment", "not a full document", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "index.html"), tc.html)
			write(t, filepath.Join(dir, "assets/logo.svg"), "<svg/>")
			err := ValidateOutput(dir)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
