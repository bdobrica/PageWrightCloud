# Pagewright Compiler

A standalone Go binary that transforms Markdown/component content and a trusted
theme into a static website. It is not an AI execution sandbox. See the
[compiler contract and tested boundaries](../../docs/COMPILER_CONTRACT.md).

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

**Navigation is auto-generated from folder hierarchy.** `index.mdx` also works;
duplicate routes or both extensions in one directory fail. Output must be absent
or empty and separate from source/theme. Use a fresh output directory for each
build; failure does not publish partially rendered pages.

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
- Props are JSON with string leaf values only; plain unquoted strings also work
- Unknown components cause build failure

## Worker Integration

Real worker invocation and trusted-theme packaging remain M2. The current worker
does not call this compiler. The supported local CLI validates a quiescent input
tree and stages output, but process isolation, credentials, resource limits and
publication policy must be enforced by the eventual worker integration.

Only bundled trusted themes are in MVP scope. Do not accept uploaded templates
or use these filesystem checks as protection against concurrent hostile mutation.

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
- Go 1.24.10 (matching go.mod and the pinned development baseline)

## Development

```bash
make build      # Build binary
make test       # Run compiler fixtures and CLI tests
make fmt        # Format code
make clean      # Remove binaries
```

See [../themes/README.md](../themes/README.md) for theme documentation.
