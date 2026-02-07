# Pagewright Compiler

A standalone Go binary that transforms markdown content and a theme into a static website. Designed to limit what AI agents can modify while giving them full control over content.

## Quick Start

```bash
# Build the compiler
cd pagewright/compiler
go build -o pagewrightc ./cmd/pagewrightc

# Compile a site
./pagewrightc build \
  --theme ../themes/starter \
  --content ./my-site/content \
  --out ./my-site/dist
```

## How It Works

**Input:**
- **Theme directory** - Layout templates, CSS, tokens, MDX components
- **Content directory** - Folder tree with `index.md` files
- **site.json** - Site configuration (name, author, token overrides)

**Output:**
- **dist/** - Complete static website

**Pipeline:**
1. Walk content tree → build page graph + navigation
2. Parse markdown + MDX component blocks (:::component Name)
3. Extract titles, headings, TOC from markdown
4. Render components with validated JSON props
5. Apply theme templates with Go html/template
6. Generate CSS from tokens.json
7. Copy assets (theme + per-page)
8. Write HTML files atomically

## Content Structure

```
content/
├── site.json           # Site config
├── home/
│   └── index.md       # Maps to /
├── about/
│   └── index.md       # Maps to /about
└── blog/
    ├── index.md       # Maps to /blog
    └── post-1/
        └── index.md   # Maps to /blog/post-1
```

**Navigation is auto-generated from folder hierarchy.**

## MDX Components

Use components in markdown:

```markdown
# My Page

Regular markdown content here.

:::component Hero
headline: "Welcome"
cta_text: "Get Started"
cta_href: "/about"
:::

More markdown content.
```

**Rules:**
- Components must exist in theme's `src/mdx-components/`
- Props are JSON with string leaf values only
- Unknown components cause build failure

## Worker Integration

The compiler is designed to be invoked by the PageWright worker as a tool:

```go
// In worker tool handler
// 1. Download theme from registry
resp, _ := http.Get("http://themes:8086/starter.tar.gz")
themeTgz, _ := io.ReadAll(resp.Body)

// 2. Extract theme
exec.Command("tar", "-xzf", themeTgz, "-C", "/workspace/theme").Run()

// 3. Run compiler
cmd := exec.Command("pagewrightc", "build",
    "--theme", "/workspace/theme/starter",
    "--content", contentPath,
    "--out", outputPath,
    "--base-url", baseURL,
)
output, err := cmd.CombinedOutput()
```

**What AI agents can do:**
- ✅ Edit markdown content files
- ✅ Modify site.json (name, author, token overrides)
- ✅ Add page assets
- ✅ Choose between available themes

**What AI agents cannot do:**
- ❌ Modify base URL (controlled by tool invocation)
- ❌ Edit theme templates (read-only)
- ❌ Execute arbitrary code
- ❌ Access files outside workspace

This provides a safe, constrained environment for AI-assisted website building.

## Commands

```bash
# Build a site
pagewrightc build --theme <dir> --content <dir> --out <dir> [--base-url <url>]

# Show version
pagewrightc version

# Show help
pagewrightc help
```

## site.json Example

```json
{
  "site_name": "My Site",
  "author": "Jane Doe",
  "lang": "en",
  "logo_url": "/assets/logo.svg",
  "primary_cta": {
    "label": "Get Started",
    "href": "/contact"
  },
  "tokens": {
    "color_primary": "#ff6b6b",
    "font_sans": "Inter, sans-serif"
  }
}
```

## Architecture

```
cmd/pagewrightc/main.go     - CLI entry point
internal/
  types/types.go            - Core data structures
  config/config.go          - Load site.json, tokens.json
  content/discover.go       - Walk content tree, build page graph
  content/nav.go            - Generate navigation HTML
  markdown/render.go        - Goldmark markdown rendering
  mdx/parse.go              - Parse :::component blocks
  mdx/registry.go           - Component template loading
  theme/theme.go            - Theme loading & rendering
  assets/assets.go          - Asset copying, atomic writes
  compile/pipeline.go       - Main orchestration
  util/slug.go              - String utilities
```

## Dependencies

- `github.com/yuin/goldmark` - Markdown parser with GFM support
- Go 1.21+ standard library

## Development

```bash
make build      # Build binary
make test       # Run tests (TODO)
make fmt        # Format code
make clean      # Remove binaries
```

See [../themes/README.md](../themes/README.md) for theme documentation.
