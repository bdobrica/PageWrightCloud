package compile

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/assets"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/config"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/content"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/markdown"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/mdx"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/theme"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
)

// Pipeline orchestrates the complete compilation process
type Pipeline struct {
	config   *types.BuildConfig
	theme    *theme.Theme
	mdxReg   *mdx.Registry
	pages    []*types.Page
	homePage *types.Page
	site     types.Site
}

// NewPipeline creates a new compilation pipeline
func NewPipeline(cfg *types.BuildConfig) (*Pipeline, error) {
	return &Pipeline{
		config: cfg,
	}, nil
}

// Run executes the complete compilation pipeline
func (p *Pipeline) Run() error {
	// Phase 1: Load theme
	fmt.Println("Loading theme...")
	var err error
	p.theme, err = theme.Load(p.config.ThemeDir, p.config.SiteConfig)
	if err != nil {
		return fmt.Errorf("failed to load theme: %w", err)
	}

	// Phase 2: Load MDX components
	fmt.Println("Loading MDX components...")
	p.mdxReg = mdx.NewRegistry()
	if err := p.mdxReg.LoadFromTheme(p.config.ThemeDir); err != nil {
		return fmt.Errorf("failed to load MDX components: %w", err)
	}

	// Phase 3: Build site context
	p.site = config.BuildSiteContext(p.config, p.theme.Tokens)

	// Phase 4: Discover content
	fmt.Println("Discovering pages...")
	p.pages, err = content.Discover(p.config.ContentDir)
	if err != nil {
		return fmt.Errorf("failed to discover content: %w", err)
	}
	fmt.Printf("Found %d pages\n", len(p.pages))

	p.homePage = content.FindHomePage(p.pages)
	if p.homePage == nil {
		return &types.CompileError{
			File:    p.config.ContentDir,
			Message: "no home page found (missing home/index.md or index.md)",
		}
	}

	// Phase 5: Process each page
	fmt.Println("Processing pages...")
	for _, page := range p.pages {
		if err := p.compilePage(page); err != nil {
			return fmt.Errorf("failed to compile page %s: %w", page.SourceMD, err)
		}
	}

	// Phase 6: Copy assets
	fmt.Println("Copying assets...")
	if err := p.copyAssets(); err != nil {
		return fmt.Errorf("failed to copy assets: %w", err)
	}

	// Phase 7: Generate tokens.css
	fmt.Println("Generating tokens.css...")
	if err := p.generateTokensCSS(); err != nil {
		return fmt.Errorf("failed to generate tokens.css: %w", err)
	}

	// Phase 8: Process theme CSS with tokens
	fmt.Println("Processing theme CSS...")
	if err := p.processThemeCSS(); err != nil {
		return fmt.Errorf("failed to process theme CSS: %w", err)
	}

	fmt.Println("Build complete!")
	return nil
}

// compilePage processes a single page
func (p *Pipeline) compilePage(page *types.Page) error {
	// Read markdown content
	mdContent, err := os.ReadFile(page.SourceMD)
	if err != nil {
		return &types.CompileError{
			File:    page.SourceMD,
			Message: fmt.Sprintf("failed to read file: %v", err),
		}
	}

	// Extract title from first H1
	title, err := markdown.ExtractTitle(mdContent)
	if err != nil {
		return err
	}

	if title == "" {
		title = p.site.Name // fallback to site name
	}
	page.Title = title

	// Extract all headings for TOC
	headings, err := markdown.ExtractHeadings(mdContent)
	if err != nil {
		return err
	}

	page.TOCHTML = markdown.GenerateTOC(headings)

	// Parse MDX nodes
	nodes, err := mdx.Parse(mdContent)
	if err != nil {
		return err
	}

	// Render content
	contentHTML, err := p.renderNodes(nodes, page)
	if err != nil {
		return err
	}
	page.ContentHTML = contentHTML

	// Set page URL
	page.URL = p.site.BaseURL + page.Slug

	// Generate navigation HTML (with current page highlighted)
	navHTML := content.GenerateNavHTML(p.homePage, page)

	// Build render context
	ctx := types.RenderContext{
		Site:        p.site,
		Page:        *page,
		NavHTML:     navHTML,
		ContentHTML: page.ContentHTML,
	}

	// Render page with theme
	output, err := p.theme.RenderPage(ctx)
	if err != nil {
		return &types.CompileError{
			File:    page.SourceMD,
			Message: fmt.Sprintf("failed to render page: %v", err),
		}
	}

	// Write output file
	outputPath := filepath.Join(p.config.OutputDir, page.OutputPath)
	if err := assets.WriteFile(outputPath, output); err != nil {
		return err
	}

	fmt.Printf("  ✓ %s -> %s\n", page.Slug, page.OutputPath)

	return nil
}

// renderNodes renders a sequence of MDX nodes to HTML
func (p *Pipeline) renderNodes(nodes []types.MDXNode, page *types.Page) (template.HTML, error) {
	var result []byte

	for _, node := range nodes {
		switch n := node.(type) {
		case types.MarkdownNode:
			// Render markdown
			html, err := markdown.Render([]byte(n.Text))
			if err != nil {
				return "", &types.CompileError{
					File:    page.SourceMD,
					Message: fmt.Sprintf("failed to render markdown: %v", err),
				}
			}
			result = append(result, []byte(html)...)

		case types.ComponentNode:
			// Render component
			html, err := p.mdxReg.Render(n.Name, n.Props, n.Line)
			if err != nil {
				if compErr, ok := err.(*types.CompileError); ok {
					compErr.File = page.SourceMD
					return "", compErr
				}
				return "", err
			}
			result = append(result, []byte(html)...)
		}
	}

	return template.HTML(result), nil
}

// copyAssets copies theme and page assets
func (p *Pipeline) copyAssets() error {
	// Copy theme assets
	if err := assets.CopyThemeAssets(p.config.ThemeDir, p.config.OutputDir); err != nil {
		return err
	}

	// Copy per-page assets
	for _, page := range p.pages {
		if err := assets.CopyPageAssets(page, p.config.OutputDir); err != nil {
			return err
		}
	}

	return nil
}

// generateTokensCSS generates the tokens.css file
func (p *Pipeline) generateTokensCSS() error {
	cssContent, err := theme.GenerateTokensCSS(p.theme.Tokens)
	if err != nil {
		return err
	}

	outputPath := filepath.Join(p.config.OutputDir, "assets", "css", "tokens.css")
	return assets.WriteFile(outputPath, cssContent)
}

// processThemeCSS processes theme.css with token substitution
func (p *Pipeline) processThemeCSS() error {
	themeCSSPath := filepath.Join(p.config.ThemeDir, "src", "assets", "css", "theme.css")

	// Check if theme.css exists
	if _, err := os.Stat(themeCSSPath); os.IsNotExist(err) {
		return nil // no theme.css, skip
	}

	// Read theme CSS
	cssContent, err := os.ReadFile(themeCSSPath)
	if err != nil {
		return err
	}

	// Render with tokens
	rendered, err := theme.RenderTokenizedCSS(cssContent, p.theme.Tokens)
	if err != nil {
		return err
	}

	// Write to output
	outputPath := filepath.Join(p.config.OutputDir, "assets", "css", "theme.css")
	return assets.WriteFile(outputPath, rendered)
}
