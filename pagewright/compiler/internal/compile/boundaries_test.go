package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/assets"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/config"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/mdx"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/theme"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
)

func TestConfigurationFailures(t *testing.T) {
	for name, body := range map[string]string{
		"syntax": "{", "null": "null", "missing-name": "{}", "blank-name": `{"site_name":"  "}`,
		"wrong-type": `{"site_name":42}`, "unknown-field": `{"site_name":"Test","site_nam":"typo"}`,
		"trailing":        `{"site_name":"Test"} {}`,
		"token-object":    `{"site_name":"Test","tokens":{"color_bg":{"x":"red"}}}`,
		"token-injection": `{"site_name":"Test","tokens":{"color_bg":"red; } body {display:none"}}`,
		"token-key":       `{"site_name":"Test","tokens":{"x; color":"red"}}`,
		"token-url":       `{"site_name":"Test","tokens":{"color_bg":"url(/private)"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			source := seed(t)
			put(t, source, "site.json", body)
			if _, err := config.Load(starter(t), source, filepath.Join(t.TempDir(), "out"), ""); err == nil {
				t.Fatal("invalid site configuration accepted")
			}
		})
	}
	for _, baseURL := range []string{"javascript:alert(1)", "//example.test", "https://user:password@example.test", "https://example.test/path", "https://example.test/?x=1"} {
		if _, err := config.Load(starter(t), seed(t), filepath.Join(t.TempDir(), "out"), baseURL); err == nil {
			t.Fatalf("invalid base URL accepted: %s", baseURL)
		}
	}
	cfg, err := config.Load(starter(t), seed(t), filepath.Join(t.TempDir(), "out"), "https://example.test/")
	if err != nil || cfg.SiteConfig.Lang != "en" || cfg.BaseURL != "https://example.test" {
		t.Fatalf("wrong defaults: %+v %v", cfg, err)
	}
	source := seed(t)
	if err := os.Remove(filepath.Join(source, "site.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(starter(t), source, filepath.Join(t.TempDir(), "out"), ""); err == nil {
		t.Fatal("missing config accepted")
	}
	for _, body := range []string{"{", "null", `{"theme_name":"Test"}`, `{"theme_name":"Test","tokens":{"radius":42}}`} {
		dir := t.TempDir()
		put(t, dir, "tokens.json", body)
		if _, err := config.LoadThemeConfig(dir); err == nil {
			t.Fatal("invalid theme config accepted")
		}
	}
}
func TestTemplateErrorsAndTokenDeterminism(t *testing.T) {
	for _, scenario := range []string{"missing-tokens", "missing-layout", "invalid-layout", "missing-index", "invalid-component", "render-component"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			for name, data := range fileSet(t, starter(t)) {
				put(t, root, name, string(data))
			}
			switch scenario {
			case "missing-tokens":
				if err := os.Remove(filepath.Join(root, "tokens.json")); err != nil {
					t.Fatal(err)
				}
			case "missing-layout":
				if err := os.Rename(filepath.Join(root, "src/layout"), filepath.Join(root, "old-layout")); err != nil {
					t.Fatal(err)
				}
			case "invalid-layout":
				put(t, root, "src/layout/index.html", "{{")
			case "missing-index":
				if err := os.Remove(filepath.Join(root, "src/layout/index.html")); err != nil {
					t.Fatal(err)
				}
			case "invalid-component":
				put(t, root, "src/mdx-components/hero.html", "{{")
			case "render-component":
				put(t, root, "src/mdx-components/hero.html", `{{ index .headline 9999 }}`)
			}
			source := seed(t)
			put(t, source, "home/index.md", "# Test\n\n:::component Hero\nheadline: \"Hello\"\n:::")
			out := filepath.Join(t.TempDir(), "public")
			if err := build(t, root, source, out); err == nil {
				t.Fatal("invalid template accepted")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("failed rendering published output")
			}
		})
	}
	tokens := map[string]interface{}{"color_bg": "#ffffff", "color_text": "#000000", "radius": "10px"}
	first, err := theme.GenerateTokensCSS(tokens)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		next, err := theme.GenerateTokensCSS(tokens)
		if err != nil || string(next) != string(first) {
			t.Fatal("CSS token output changes between runs")
		}
	}
	if _, err := theme.GenerateTokensCSS(map[string]interface{}{"radius": "0; background:red"}); err == nil {
		t.Fatal("unsafe CSS accepted")
	}
	if _, err := theme.RenderTokenizedCSS([]byte("{{"), tokens); err == nil {
		t.Fatal("invalid CSS template accepted")
	}
}
func TestAssetPathAndWriteBoundaries(t *testing.T) {
	source := t.TempDir()
	put(t, source, "asset.bin", string([]byte{0, 1, 127, 128, 255}))
	out := t.TempDir()
	for _, id := range []string{"../escape", "a/../../escape", "/absolute", "a\\b", "a/../b"} {
		if err := assets.CopyPageAssets(&types.Page{ID: id, AssetsDir: source}, out); err == nil {
			t.Fatalf("unsafe asset ID accepted: %s", id)
		}
	}
	if err := assets.CopyPageAssets(&types.Page{ID: "docs/team", AssetsDir: source}, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "assets/pages/docs/team/asset.bin"))
	if err != nil || string(data) != string([]byte{0, 1, 127, 128, 255}) {
		t.Fatal("binary asset changed")
	}
	outside := t.TempDir()
	put(t, outside, "sentinel", "keep")
	for _, scenario := range []string{"target", "parent", "old-temp", "hidden-parent"} {
		t.Run(scenario, func(t *testing.T) {
			dest := t.TempDir()
			path := filepath.Join(dest, "file")
			switch scenario {
			case "target":
				if err := os.Symlink(filepath.Join(outside, "sentinel"), path); err != nil {
					t.Fatal(err)
				}
			case "parent":
				if err := os.Symlink(outside, filepath.Join(dest, "linked")); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(dest, "linked/sentinel")
			case "old-temp":
				if err := os.Symlink(filepath.Join(outside, "sentinel"), path+".tmp"); err != nil {
					t.Fatal(err)
				}
			case "hidden-parent":
				if err := os.Symlink(outside, filepath.Join(dest, "linked")); err != nil {
					t.Fatal(err)
				}
				path = dest + "/missing/../linked/../file"
			}
			err := assets.WriteFile(path, []byte("new"))
			if scenario == "old-temp" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("symlink write accepted")
			}
			data, err := os.ReadFile(filepath.Join(outside, "sentinel"))
			if err != nil || string(data) != "keep" {
				t.Fatal("outside target modified")
			}
		})
	}
}
func TestLiteralComponentsAndStringProps(t *testing.T) {
	for _, fence := range []string{"```", "~~~"} {
		nodes, err := mdx.Parse([]byte(fence + "markdown\n:::component Missing\n" + fence + "\n"))
		if err != nil || len(nodes) != 1 {
			t.Fatalf("literal component parsed: %v", err)
		}
		if _, ok := nodes[0].(types.MarkdownNode); !ok {
			t.Fatal("code fence became a component")
		}
	}
	nodes, err := mdx.Parse([]byte(":::component Hero\nheadline: Plain text\nitems: [\"one\", {\"label\": \"two\"}]\n:::"))
	if err != nil || len(nodes) != 1 {
		t.Fatalf("valid props rejected: %v", err)
	}
	if nodes[0].(types.ComponentNode).Props["headline"] != "Plain text" {
		t.Fatal("bare-string compatibility lost")
	}
	if _, err := mdx.Parse([]byte(strings.Repeat("x", 70000))); err == nil {
		t.Fatal("oversized scanner line accepted")
	}
}
