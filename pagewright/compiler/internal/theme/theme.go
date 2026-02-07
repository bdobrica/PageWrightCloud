package theme

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/config"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
)

// Theme represents a loaded theme with templates and tokens
type Theme struct {
	Templates *template.Template
	Tokens    map[string]interface{}
	Dir       string
}

// Load loads a theme from the specified directory
func Load(themeDir string, siteConfig *types.SiteConfig) (*Theme, error) {
	// Load theme config (tokens.json)
	themeConfig, err := config.LoadThemeConfig(themeDir)
	if err != nil {
		return nil, err
	}

	// Merge tokens with site overrides
	tokens := config.MergeTokens(themeConfig.Tokens, siteConfig.Tokens)

	// Load templates
	templates, err := loadTemplates(themeDir)
	if err != nil {
		return nil, err
	}

	return &Theme{
		Templates: templates,
		Tokens:    tokens,
		Dir:       themeDir,
	}, nil
}

// loadTemplates loads all layout templates from the theme
func loadTemplates(themeDir string) (*template.Template, error) {
	layoutDir := filepath.Join(themeDir, "src", "layout")

	// Create root template with custom functions
	root := template.New("root").Funcs(template.FuncMap{
		"safeHTML": func(s interface{}) template.HTML {
			switch v := s.(type) {
			case template.HTML:
				return v
			case string:
				return template.HTML(v)
			default:
				return template.HTML(fmt.Sprint(v))
			}
		},
	})

	// Walk the layout directory and parse all .html files
	err := filepath.Walk(layoutDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(info.Name(), ".html") {
			return nil
		}

		// Read template file
		content, err := os.ReadFile(path)
		if err != nil {
			return &types.CompileError{
				File:    path,
				Message: fmt.Sprintf("failed to read template: %v", err),
			}
		}

		// Get relative path for template name
		relPath, err := filepath.Rel(layoutDir, path)
		if err != nil {
			return err
		}

		// Use just the filename as the template name (simpler for {{ template "header.html" }})
		templateName := filepath.Base(relPath)

		// Parse and add to template set
		_, err = root.New(templateName).Parse(string(content))
		if err != nil {
			return &types.CompileError{
				File:    path,
				Message: fmt.Sprintf("failed to parse template: %v", err),
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return root, nil
}

// RenderPage renders a page using the theme templates
func (t *Theme) RenderPage(ctx types.RenderContext) ([]byte, error) {
	var buf bytes.Buffer

	// Execute the index.html template
	if err := t.Templates.ExecuteTemplate(&buf, "index.html", ctx); err != nil {
		return nil, fmt.Errorf("failed to render page: %w", err)
	}

	return buf.Bytes(), nil
}

// GenerateTokensCSS generates a tokens.css file from theme tokens
func GenerateTokensCSS(tokens map[string]interface{}) ([]byte, error) {
	var sb strings.Builder

	sb.WriteString(":root {\n")

	// Convert tokens to CSS variables
	for key, value := range tokens {
		// Skip non-CSS tokens (like site_name, logo_url, etc.)
		if strings.HasPrefix(key, "site_") || key == "theme_name" {
			continue
		}

		// Convert snake_case to kebab-case for CSS
		cssVar := strings.ReplaceAll(key, "_", "-")

		sb.WriteString("  --")
		sb.WriteString(cssVar)
		sb.WriteString(": ")
		sb.WriteString(fmt.Sprint(value))
		sb.WriteString(";\n")
	}

	sb.WriteString("}\n")

	return []byte(sb.String()), nil
}

// RenderTokenizedCSS processes a CSS file with token placeholders
func RenderTokenizedCSS(cssContent []byte, tokens map[string]interface{}) ([]byte, error) {
	tmpl, err := template.New("css").Parse(string(cssContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSS template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tokens); err != nil {
		return nil, fmt.Errorf("failed to render CSS template: %w", err)
	}

	return buf.Bytes(), nil
}
