package compile

import (
	"errors"
	"html/template"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/content"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/markdown"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/mdx"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/util"
	"os"
	"path/filepath"
)

func TestComponentFailures(t *testing.T) {
	for name, body := range map[string]string{
		"missing-name":  ":::component\n:::",
		"unclosed":      ":::component Hero\nheadline: \"Hi\"",
		"missing-colon": ":::component Hero\nheadline \"Hi\"\n:::",
		"broken-json":   ":::component Hero\nheadline: [broken\n:::",
		"number":        ":::component Hero\nheadline: 42\n:::",
		"boolean":       ":::component Hero\nheadline: true\n:::",
		"null":          ":::component Hero\nheadline: null\n:::",
		"duplicate":     ":::component Hero\nheadline: \"one\"\nheadline: \"two\"\n:::",
		"empty-key":     ":::component Hero\n: \"value\"\n:::",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mdx.Parse([]byte(body)); err == nil {
				t.Fatal("invalid component accepted")
			}
		})
	}
}

func TestWorkingDirectoryAlias(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PWD", alias)
	got, err := util.Absolute("testdata")
	if err != nil || got != filepath.Join(physical, "testdata") {
		t.Fatalf("alias not normalized: %q %v", got, err)
	}
	if err := util.CheckPath(filepath.Join(alias, "testdata"), false); err == nil {
		t.Fatal("explicit symlink path accepted")
	}
	if err := util.CheckPath(alias+"/../file", true); err == nil {
		t.Fatal("symlink hidden by dot-dot accepted")
	}
}
func TestComponentLineAndEscaping(t *testing.T) {
	source := []byte("# Test\n\n:::component Hero\nheadline: \"Safe\"\n:::\n\n:::component Missing\n:::\n")
	nodes, err := mdx.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	reg := mdx.NewRegistry()
	if err := reg.LoadFromTheme(starter(t)); err != nil {
		t.Fatal(err)
	}
	var last types.ComponentNode
	for _, node := range nodes {
		if n, ok := node.(types.ComponentNode); ok {
			last = n
		}
	}
	_, err = reg.Render(last.Name, last.Props, last.Line)
	var diagnostic *types.CompileError
	if !errors.As(err, &diagnostic) || diagnostic.Line != 7 || !strings.Contains(diagnostic.Error(), ":7:") {
		t.Fatalf("wrong diagnostic: %v", err)
	}
	html, err := reg.Render("Hero", map[string]interface{}{"headline": "<img src=x onerror=alert(1)>", "cta_href": "javascript:alert(1)", "cta_text": "Click"}, 1)
	if err != nil || !strings.Contains(string(html), "&lt;img") || !strings.Contains(string(html), "#ZgotmplZ") {
		t.Fatalf("unsafe component: %s (%v)", html, err)
	}
	if !reg.HasComponent("YouTubeVideo") {
		t.Fatal("documented YouTubeVideo component unavailable")
	}
}
func TestMarkdownTOCAndNavigationEscaping(t *testing.T) {
	source := []byte("# Hello **world**\n\n## Repeat\n\n## Repeat\n\n<script>alert(1)</script>\n")
	title, err := markdown.ExtractTitle(source)
	if err != nil || title != "Hello world" {
		t.Fatalf("title=%q, err=%v", title, err)
	}
	headings, err := markdown.ExtractHeadings(source)
	if err != nil {
		t.Fatal(err)
	}
	html, err := markdown.Render(source)
	if err != nil {
		t.Fatal(err)
	}
	toc := markdown.GenerateTOC(headings)
	if !strings.Contains(string(toc), `href="#repeat-1"`) || !strings.Contains(string(html), `id="repeat-1"`) {
		t.Fatal("TOC anchors do not match HTML")
	}
	home := &types.Page{Slug: "/", Title: "Home"}
	child := &types.Page{Slug: `/bad" onclick="alert(1)`, Title: "<script>", Parent: home}
	home.Children = []*types.Page{child}
	for _, output := range []template.HTML{content.GenerateNavHTML(home, child), content.GenerateBreadcrumbHTML(&types.Page{Slug: "/leaf", Parent: child}), markdown.GenerateTOC([]types.Heading{{Level: 2, Text: "<script>", ID: `x" onclick="alert(1)`}})} {
		if strings.Contains(string(output), `" onclick="`) || strings.Contains(string(output), "<script>") {
			t.Fatalf("unescaped markup: %s", output)
		}
	}
}
