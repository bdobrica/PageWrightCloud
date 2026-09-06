package markdown

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

var md goldmark.Markdown

func init() {
	md = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
}

// Render converts markdown to HTML
func Render(source []byte) (template.HTML, error) {
	return RenderWithIDs(source, parser.NewContext().IDs())
}

// RenderWithIDs shares heading IDs across Markdown chunks separated by components.
func RenderWithIDs(source []byte, ids parser.IDs) (template.HTML, error) {
	var buf bytes.Buffer
	if err := md.Convert(source, &buf, parser.WithContext(parser.NewContext(parser.WithIDs(ids)))); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// ExtractHeadings parses markdown and extracts all headings
func ExtractHeadings(source []byte) ([]types.Heading, error) {
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	var headings []types.Heading

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if heading, ok := n.(*ast.Heading); ok {
			// Extract heading text
			text := string(heading.Text(source))
			rawID, _ := heading.AttributeString("id")
			idBytes, _ := rawID.([]byte)
			id := string(idBytes)

			headings = append(headings, types.Heading{
				Level: heading.Level,
				Text:  text,
				ID:    id,
			})
		}

		return ast.WalkContinue, nil
	})

	return headings, nil
}

// ExtractTitle returns the first H1 heading text, or empty string
func ExtractTitle(source []byte) (string, error) {
	headings, err := ExtractHeadings(source)
	if err != nil {
		return "", err
	}

	for _, h := range headings {
		if h.Level == 1 {
			return h.Text, nil
		}
	}

	return "", nil
}

// GenerateTOC creates a table of contents HTML from headings
func GenerateTOC(headings []types.Heading) template.HTML {
	if len(headings) == 0 {
		return ""
	}

	// Filter out H1 (page title) and only include H2-H4
	var tocHeadings []types.Heading
	for _, h := range headings {
		if h.Level >= 2 && h.Level <= 4 {
			tocHeadings = append(tocHeadings, h)
		}
	}

	if len(tocHeadings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<ul class=\"toc-list\">\n")

	for _, h := range tocHeadings {
		indent := strings.Repeat("  ", h.Level-2)
		sb.WriteString(indent)
		sb.WriteString("<li>")
		sb.WriteString("<a href=\"#")
		sb.WriteString(template.HTMLEscapeString(h.ID))
		sb.WriteString("\">")
		sb.WriteString(template.HTMLEscapeString(h.Text))
		sb.WriteString("</a></li>\n")
	}

	sb.WriteString("</ul>")
	return template.HTML(sb.String())
}
