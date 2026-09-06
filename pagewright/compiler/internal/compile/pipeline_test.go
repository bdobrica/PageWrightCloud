package compile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/config"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/util"
)

func put(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
func seed(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, "site.json", `{"site_name":"Test Site"}`)
	put(t, root, "home/index.md", "# Home")
	return root
}
func starter(t *testing.T) string {
	t.Helper()
	path, err := util.Absolute("../../../themes/starter")
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func build(t *testing.T, theme, source, out string) error {
	t.Helper()
	cfg, err := config.Load(theme, source, out, "https://example.test")
	if err != nil {
		return err
	}
	p, err := NewPipeline(cfg)
	if err != nil {
		return err
	}
	return p.Run()
}
func fileSet(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
func TestStarterFixture(t *testing.T) {
	source := "testdata/site/content"
	out := filepath.Join(t.TempDir(), "public")
	if err := build(t, starter(t), source, out); err != nil {
		t.Fatal(err)
	}
	files := fileSet(t, out)
	expected := []string{"index.html", "about/index.html", "about/team/index.html", "homestead/index.html", "assets/css/theme.css", "assets/css/tokens.css", "assets/js/theme.js", "assets/pages/home/example.txt", "assets/pages/about/example.txt"}
	if len(files) != len(expected) {
		t.Fatalf("unexpected output set: %v", files)
	}
	for _, name := range expected {
		if len(files[name]) == 0 {
			t.Errorf("missing %s", name)
		}
	}
	home := string(files["index.html"])
	for _, fragment := range []string{"<title>Welcome Friends</title>", "Fixture &amp; Friends", `href="/about">About Our Team</a>`, `href="/about/team">Meet the Team</a>`, `href="/homestead">The Homestead</a>`, `href="#repeated-heading-1"`, `id="repeated-heading-1"`, "&lt;script&gt;alert(1)&lt;/script&gt;", "#ZgotmplZ"} {
		if !strings.Contains(home, fragment) {
			t.Errorf("missing rendered fragment %q", fragment)
		}
	}
	if strings.Contains(home, "<script>alert") || strings.Contains(home, `href="javascript:`) {
		t.Fatal("unsafe HTML/URL rendered")
	}
	if !strings.Contains(string(files["about/index.html"]), `<li class="active"><a href="/about">`) {
		t.Fatal("active navigation missing")
	}
	if !strings.Contains(string(files["assets/css/tokens.css"]), "--color-primary: #123456;") {
		t.Fatal("token override missing")
	}
	for _, pair := range [][2]string{{"home/assets/example.txt", "assets/pages/home/example.txt"}, {"about/assets/example.txt", "assets/pages/about/example.txt"}} {
		original, err := os.ReadFile(filepath.Join(source, pair[0]))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, files[pair[1]]) {
			t.Fatal("asset changed")
		}
	}
	second := filepath.Join(t.TempDir(), "public")
	if err := build(t, starter(t), source, second); err != nil {
		t.Fatal(err)
	}
	for name, data := range fileSet(t, second) {
		if !bytes.Equal(data, files[name]) {
			t.Errorf("nondeterministic output %s", name)
		}
	}
}
func TestHomeAndRouteDiscovery(t *testing.T) {
	cases := []struct {
		name      string
		pages     []string
		wantError bool
	}{
		{"root", []string{"index.md"}, false},
		{"home-md", []string{"home/index.md"}, false},
		{"home-mdx", []string{"home/index.mdx"}, false},
		{"missing", []string{"about/index.md"}, true},
		{"empty", nil, true},
		{"duplicate-home", []string{"index.md", "home/index.md"}, true},
		{"ambiguous-extension", []string{"home/index.md", "home/index.mdx"}, true},
		{"duplicate-route", []string{"home/index.md", "home/about/index.md", "about/index.md"}, true},
		{"nested-home", []string{"home/index.md", "home/docs/index.md", "home/docs/guide/index.md"}, false},
		{"grouping-directory", []string{"home/index.md", "docs/guide/index.md"}, false},
		{"traversal-name", []string{"home/index.md", "..\\escape/index.md"}, true},
		{"attribute-injection", []string{"home/index.md", `bad" onclick="alert(1)/index.md`}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := t.TempDir()
			put(t, source, "site.json", `{"site_name":"Test"}`)
			for _, page := range tc.pages {
				put(t, source, page, "# Page")
			}
			out := filepath.Join(t.TempDir(), "public")
			err := build(t, starter(t), source, out)
			if (err != nil) != tc.wantError {
				t.Fatalf("build error=%v, want error=%v", err, tc.wantError)
			}
			if tc.wantError {
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("failed build published output")
				}
			}
			if tc.name == "grouping-directory" {
				data, err := os.ReadFile(filepath.Join(out, "index.html"))
				if err != nil || !strings.Contains(string(data), `href="/docs/guide">Page</a>`) {
					t.Fatal("grouped page absent from navigation")
				}
			}
		})
	}
}
func TestBuildFailureAndOutputBoundaries(t *testing.T) {
	for _, scenario := range []string{"content-link", "index-link", "config-link", "asset-link", "asset-dir-link", "theme-link", "output-link", "output-parent-link", "overlap", "existing-output", "invalid-component"} {
		t.Run(scenario, func(t *testing.T) {
			source := seed(t)
			theme := starter(t)
			base := t.TempDir()
			out := filepath.Join(base, "public")
			outside := t.TempDir()
			put(t, outside, "sentinel", "unchanged")
			link := func(target, path string) {
				t.Helper()
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "content-link":
				link(source, filepath.Join(base, "linked"))
				source = filepath.Join(base, "linked")
			case "index-link":
				if err := os.Remove(filepath.Join(source, "home/index.md")); err != nil {
					t.Fatal(err)
				}
				link(filepath.Join(outside, "sentinel"), filepath.Join(source, "home/index.md"))
			case "config-link":
				if err := os.Remove(filepath.Join(source, "site.json")); err != nil {
					t.Fatal(err)
				}
				link(filepath.Join(outside, "sentinel"), filepath.Join(source, "site.json"))
			case "asset-link":
				put(t, source, "home/assets/ok.txt", "ok")
				link(filepath.Join(outside, "sentinel"), filepath.Join(source, "home/assets/secret"))
			case "asset-dir-link":
				link(outside, filepath.Join(source, "home/assets"))
			case "theme-link":
				link(theme, filepath.Join(base, "theme"))
				theme = filepath.Join(base, "theme")
			case "output-link":
				link(outside, out)
			case "output-parent-link":
				link(outside, filepath.Join(base, "parent"))
				out = filepath.Join(base, "parent/public")
			case "overlap":
				out = filepath.Join(source, "public")
			case "existing-output":
				put(t, out, "sentinel", "unchanged")
			case "invalid-component":
				put(t, source, "zzz/index.md", "# Last\n\n:::component Missing\n:::")
			}
			if err := build(t, theme, source, out); err == nil {
				t.Fatal("unsafe build accepted")
			}
			if got := fileSet(t, outside); len(got) != 1 || string(got["sentinel"]) != "unchanged" {
				t.Fatal("outside files modified")
			}
			if scenario == "invalid-component" {
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("partial output published")
				}
			}
			if scenario == "existing-output" {
				if got := fileSet(t, out); len(got) != 1 || string(got["sentinel"]) != "unchanged" {
					t.Fatal("existing output modified")
				}
			}
		})
	}
}
