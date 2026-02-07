package types

import "html/template"

// Site represents the site-level configuration and context
type Site struct {
	Name               string
	BaseURL            string
	Lang               string
	Year               int
	LogoURL            string
	PrimaryCTA         *CTA
	SidebarDefaultHTML template.HTML
	FooterLinksHTML    template.HTML
	Author             string
	Copyright          string
}

// CTA represents a call-to-action button
type CTA struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

// Page represents a single page in the site
type Page struct {
	ID         string // stable ID from path, e.g., "about-us/team"
	Dir        string // absolute directory path
	RelDir     string // relative path under content root
	Slug       string // URL slug, e.g., "/about-us/team"
	URL        string // full URL including base
	OutputPath string // output file path in dist
	SourceMD   string // absolute path to index.md
	AssetsDir  string // optional page assets directory

	Title          string
	Description    string
	ContentMD      string        // raw markdown content
	ContentHTML    template.HTML // rendered HTML
	TOCHTML        template.HTML // table of contents HTML
	SidebarHTML    template.HTML // optional sidebar content
	BreadcrumbHTML template.HTML // breadcrumb navigation

	Children []*Page
	Parent   *Page
}

// RenderContext is the complete context passed to page templates
type RenderContext struct {
	Site        Site
	Page        Page
	NavHTML     template.HTML
	ContentHTML template.HTML
}

// NavNode represents a navigation tree node
type NavNode struct {
	Label    string
	Slug     string
	Active   bool
	Children []*NavNode
}

// Token represents theme tokens for CSS variable generation
type Token struct {
	Name  string
	Value string
}

// SiteConfig represents the site.json configuration
type SiteConfig struct {
	SiteName   string                 `json:"site_name"`
	Author     string                 `json:"author,omitempty"`
	Copyright  string                 `json:"copyright,omitempty"`
	Lang       string                 `json:"lang,omitempty"`
	LogoURL    string                 `json:"logo_url,omitempty"`
	PrimaryCTA *CTA                   `json:"primary_cta,omitempty"`
	Tokens     map[string]interface{} `json:"tokens,omitempty"` // overrides for theme tokens
}

// ThemeConfig represents the theme's tokens.json
type ThemeConfig struct {
	ThemeName string                 `json:"theme_name"`
	Tokens    map[string]interface{} `json:"tokens"`
}

// BuildConfig represents the compiler build configuration
type BuildConfig struct {
	ThemeDir   string
	ContentDir string
	OutputDir  string
	BaseURL    string
	SiteConfig *SiteConfig
}

// CompileError represents a compilation error with file context
type CompileError struct {
	File    string
	Line    int
	Column  int
	Message string
}

func (e *CompileError) Error() string {
	if e.Line > 0 {
		return e.File + ":" + string(rune(e.Line)) + ":" + string(rune(e.Column)) + ": " + e.Message
	}
	return e.File + ": " + e.Message
}

// MDXNode represents a node in the parsed MDX content
type MDXNode interface {
	node()
}

// MarkdownNode represents a chunk of markdown text
type MarkdownNode struct {
	Text string
}

func (n MarkdownNode) node() {}

// ComponentNode represents an MDX component block
type ComponentNode struct {
	Name  string
	Props map[string]interface{}
	Line  int // for error reporting
}

func (n ComponentNode) node() {}

// Heading represents a heading extracted from markdown
type Heading struct {
	Level int
	Text  string
	ID    string // anchor ID
}
