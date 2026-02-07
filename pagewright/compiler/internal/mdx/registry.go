package mdx

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/bogdan/pagewright/compiler/internal/types"
)

// Registry manages MDX component templates
type Registry struct {
	templates map[string]*template.Template
	propsInfo map[string]string // component name -> props comment
}

// NewRegistry creates a new component registry
func NewRegistry() *Registry {
	return &Registry{
		templates: make(map[string]*template.Template),
		propsInfo: make(map[string]string),
	}
}

// LoadFromTheme loads all MDX component templates from the theme directory
func (r *Registry) LoadFromTheme(themeDir string) error {
	componentsDir := filepath.Join(themeDir, "src", "mdx-components")

	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return &types.CompileError{
			File:    componentsDir,
			Message: fmt.Sprintf("failed to read mdx-components directory: %v", err),
		}
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}

		componentName := componentNameFromFile(entry.Name())
		filePath := filepath.Join(componentsDir, entry.Name())

		// Read template file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return &types.CompileError{
				File:    filePath,
				Message: fmt.Sprintf("failed to read component template: %v", err),
			}
		}

		// Extract props comment if present
		propsComment := extractPropsComment(content)
		if propsComment != "" {
			r.propsInfo[componentName] = propsComment
		}

		// Parse template
		tmpl, err := template.New(componentName).Parse(string(content))
		if err != nil {
			return &types.CompileError{
				File:    filePath,
				Message: fmt.Sprintf("failed to parse component template: %v", err),
			}
		}

		r.templates[componentName] = tmpl
	}

	return nil
}

// Render renders a component with the given props
func (r *Registry) Render(componentName string, props map[string]interface{}, line int) (template.HTML, error) {
	tmpl, ok := r.templates[componentName]
	if !ok {
		return "", &types.CompileError{
			Line:    line,
			Message: fmt.Sprintf("unknown component: %s", componentName),
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, props); err != nil {
		return "", &types.CompileError{
			Line:    line,
			Message: fmt.Sprintf("failed to render component %s: %v", componentName, err),
		}
	}

	return template.HTML(buf.String()), nil
}

// HasComponent checks if a component is registered
func (r *Registry) HasComponent(name string) bool {
	_, ok := r.templates[name]
	return ok
}

// componentNameFromFile converts a filename to a component name
// e.g., "hero.html" -> "Hero", "youtube-video.html" -> "YouTubeVideo"
func componentNameFromFile(filename string) string {
	// Remove .html extension
	name := strings.TrimSuffix(filename, ".html")

	// Split by hyphen
	parts := strings.Split(name, "-")

	// Title case each part
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}

	return strings.Join(parts, "")
}

// extractPropsComment extracts the <!-- Props: ... --> comment from a template
func extractPropsComment(content []byte) string {
	lines := bytes.Split(content, []byte("\n"))
	if len(lines) == 0 {
		return ""
	}

	firstLine := string(bytes.TrimSpace(lines[0]))
	if strings.HasPrefix(firstLine, "<!--") && strings.HasSuffix(firstLine, "-->") {
		comment := strings.TrimPrefix(firstLine, "<!--")
		comment = strings.TrimSuffix(comment, "-->")
		comment = strings.TrimSpace(comment)

		if strings.HasPrefix(comment, "Props:") {
			return strings.TrimSpace(strings.TrimPrefix(comment, "Props:"))
		}
	}

	return ""
}
