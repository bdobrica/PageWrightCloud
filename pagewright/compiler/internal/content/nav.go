package content

import (
	"html/template"
	"strings"

	"github.com/bogdan/pagewright/compiler/internal/types"
	"github.com/bogdan/pagewright/compiler/internal/util"
)

// GenerateNavHTML builds navigation HTML from the page tree
func GenerateNavHTML(homePage *types.Page, currentPage *types.Page) template.HTML {
	if homePage == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<ul class=\"nav-list\">\n")

	// Add home page
	renderNavNode(&sb, homePage, currentPage, 0)

	sb.WriteString("</ul>\n")
	return template.HTML(sb.String())
}

// renderNavNode recursively renders a navigation node and its children
func renderNavNode(sb *strings.Builder, page *types.Page, currentPage *types.Page, depth int) {
	indent := strings.Repeat("  ", depth+1)

	activeClass := ""
	if currentPage != nil && page.Slug == currentPage.Slug {
		activeClass = " class=\"active\""
	}

	// Use page title if available, otherwise derive from slug
	label := page.Title
	if label == "" {
		if page.Slug == "/" {
			label = "Home"
		} else {
			// Extract last segment of slug
			parts := strings.Split(strings.Trim(page.Slug, "/"), "/")
			label = util.TitleCase(parts[len(parts)-1])
		}
	}

	sb.WriteString(indent)
	sb.WriteString("<li")
	sb.WriteString(activeClass)
	sb.WriteString("><a href=\"")
	sb.WriteString(page.Slug)
	sb.WriteString("\">")
	sb.WriteString(template.HTMLEscapeString(label))
	sb.WriteString("</a>")

	// Render children
	if len(page.Children) > 0 {
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString("  <ul>\n")
		for _, child := range page.Children {
			renderNavNode(sb, child, currentPage, depth+2)
		}
		sb.WriteString(indent)
		sb.WriteString("  </ul>\n")
		sb.WriteString(indent)
	}

	sb.WriteString("</li>\n")
}

// GenerateBreadcrumbHTML builds breadcrumb HTML for a page
func GenerateBreadcrumbHTML(page *types.Page) template.HTML {
	if page == nil || page.Slug == "/" {
		return ""
	}

	var crumbs []*types.Page
	current := page
	for current != nil {
		crumbs = append([]*types.Page{current}, crumbs...)
		current = current.Parent
	}

	var sb strings.Builder
	sb.WriteString("<nav class=\"breadcrumb\" aria-label=\"Breadcrumb\">\n")
	sb.WriteString("  <ol>\n")

	for i, crumb := range crumbs {
		label := crumb.Title
		if label == "" {
			if crumb.Slug == "/" {
				label = "Home"
			} else {
				parts := strings.Split(strings.Trim(crumb.Slug, "/"), "/")
				label = util.TitleCase(parts[len(parts)-1])
			}
		}

		sb.WriteString("    <li>")
		if i < len(crumbs)-1 {
			sb.WriteString("<a href=\"")
			sb.WriteString(crumb.Slug)
			sb.WriteString("\">")
			sb.WriteString(template.HTMLEscapeString(label))
			sb.WriteString("</a>")
		} else {
			sb.WriteString("<span aria-current=\"page\">")
			sb.WriteString(template.HTMLEscapeString(label))
			sb.WriteString("</span>")
		}
		sb.WriteString("</li>\n")
	}

	sb.WriteString("  </ol>\n")
	sb.WriteString("</nav>")

	return template.HTML(sb.String())
}
