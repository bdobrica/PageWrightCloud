package compile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/mdx"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/util"
)

func TestParadigmShiftExample(t *testing.T) {
	theme, err := util.Absolute("../../../themes/paradigm-shift")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "dist")
	example := t.TempDir()
	for name, data := range fileSet(t, filepath.Join(theme, "example/content")) {
		put(t, example, name, string(data))
	}
	if err := build(t, theme, example, out); err != nil {
		t.Fatal(err)
	}
	files := fileSet(t, out)
	for _, name := range []string{"index.html", "about/index.html", "about/process/index.html"} {
		page := string(files[name])
		for _, want := range []string{"HTML5 UP", "https://creativecommons.org/licenses/by/3.0/", "/assets/licenses/ATTRIBUTION.txt", "Main navigation", "/about/process"} {
			if !strings.Contains(page, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
		for _, bad := range []string{"<no value>", "ZgotmplZ", "is-preload"} {
			if strings.Contains(page, bad) {
				t.Errorf("%s contains %q", name, bad)
			}
		}
	}
	for _, want := range []string{"pw-hero", "pw-feature", "pw-gallery", "pw-cta", "youtube-nocookie.com/embed/aqz-KE-bpKQ"} {
		if !strings.Contains(string(files["index.html"]), want) {
			t.Errorf("home missing component %q", want)
		}
	}
	for _, name := range []string{"assets/pages/home/work.svg", "assets/css/main.css", "assets/css/theme.css", "assets/css/tokens.css", "assets/licenses/ATTRIBUTION.txt"} {
		if len(files[name]) == 0 {
			t.Errorf("missing asset %s", name)
		}
	}
	license, err := os.ReadFile(filepath.Join(theme, "LICENSE.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(license, files["assets/licenses/CC-BY-3.0.txt"]) {
		t.Error("published license differs from theme license")
	}
	if strings.Contains(string(files["assets/css/main.css"]), "@import") {
		t.Error("unexpected external CSS dependency")
	}

	// The theme must also build plain Markdown with all optional site fields absent.
	source := seed(t)
	put(t, source, "site.json", `{"site_name":"Plain site","tokens":{"color_primary":"#abcdef"}}`)
	plainOut := filepath.Join(t.TempDir(), "dist")
	if err := build(t, theme, source, plainOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fileSet(t, plainOut)["assets/css/tokens.css"]), "--color-primary: #abcdef") {
		t.Error("site token override not published")
	}

	reg := mdx.NewRegistry()
	if err := reg.LoadFromTheme(theme); err != nil {
		t.Fatal(err)
	}
	rendered, err := reg.Render("Hero", map[string]interface{}{
		"headline": "<script>alert(1)</script>", "cta_text": "Click", "cta_href": "javascript:alert(1)",
	}, 1)
	if err != nil || !strings.Contains(string(rendered), "&lt;script&gt;") || !strings.Contains(string(rendered), "#ZgotmplZ") {
		t.Fatalf("unsafe component output: %s (%v)", rendered, err)
	}
}
