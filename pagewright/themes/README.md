# Pagewright Themes

Themes define the layout, styling, and available components for Pagewright websites.

## Theme Registry

Themes are served as downloadable tar.gz archives from the themes service. Workers can discover and download themes at runtime:

```bash
# Get available themes
curl http://themes:8086/

# Download a theme
curl -O http://themes:8086/starter.tar.gz
```

The registry JSON includes theme metadata:
```json
{
  "themes": [
    {
      "name": "Starter",
      "id": "starter",
      "version": "1.0.0",
      "description": "A minimal responsive theme...",
      "compiler_version": ">=0.1.0",
      "download_url": "/starter.tar.gz",
      "size_bytes": 7891,
      "components": ["Hero", "YouTubeVideo"]
    }
  ]
}
```

## Theme Structure

```
theme/
├── tokens.json              # Design tokens (colors, fonts, spacing)
├── instructions.md          # Theme documentation
└── src/
    ├── layout/
    │   ├── index.html      # Main page template
    │   ├── header.html     # Header component
    │   ├── nav.html        # Navigation component
    │   ├── sidebar.html    # Sidebar component
    │   ├── body.html       # Content wrapper
    │   └── footer.html     # Footer component
    ├── mdx-components/
    │   ├── hero.html       # Hero component
    │   └── youtube-video.html
    └── assets/
        ├── css/
        │   └── theme.css   # Theme styles
        └── js/
            └── theme.js
```

## Template Syntax

Themes use Go's `html/template` syntax:

```html
<!DOCTYPE html>
<html lang="{{ .Site.Lang }}">
<head>
  <title>{{ .Page.Title }} - {{ .Site.Name }}</title>
</head>
<body>
  {{ template "header.html" . }}
  {{ template "nav.html" . }}
  
  <main>
    {{ .ContentHTML }}
  </main>
  
  {{ template "footer.html" . }}
</body>
</html>
```

## Template Context

Templates receive a `RenderContext` with:

**Site data:**
- `.Site.Name` - Site name
- `.Site.BaseURL` - Base URL
- `.Site.Lang` - Language code (e.g., "en")
- `.Site.Year` - Current year
- `.Site.LogoURL` - Logo path
- `.Site.PrimaryCTA.Label` / `.Href` - Primary CTA button
- `.Site.Author` - Author name
- `.Site.Copyright` - Copyright text

**Page data:**
- `.Page.Title` - Page title (from first H1)
- `.Page.Slug` - Page slug (e.g., "/about")
- `.Page.URL` - Full URL
- `.Page.Description` - Page description
- `.Page.TOCHTML` - Table of contents HTML
- `.Page.SidebarHTML` - Sidebar content
- `.Page.BreadcrumbHTML` - Breadcrumb navigation

**Generated content:**
- `.NavHTML` - Full navigation tree
- `.ContentHTML` - Rendered page content (markdown + components)

## Design Tokens

Define design tokens and metadata in `tokens.json`:

```json
{
  "theme_name": "Starter",
  "theme_version": "1.0.0",
  "theme_description": "A minimal responsive theme with sidebar",
  "compiler_version": ">=0.1.0",
  "tokens": {
    "color_bg": "#0b1220",
    "color_text": "#e8eefc",
    "color_primary": "#4a9eff",
    "font_sans": "system-ui, -apple-system, sans-serif"
  }
}
```

**Metadata fields:**
- `theme_name` - Display name for the theme
- `theme_version` - Semantic version (e.g., "1.0.0")
- `theme_description` - Description for AI agents to choose themes
- `compiler_version` - Compatible compiler versions (e.g., ">=0.1.0", ">=0.10,<1.0.0")

The compiler generates `tokens.css`:

```json
{
  "tokens": {
    "color_bg": "#0b1220",
    "color_text": "#e8eefc",
    "color_primary": "#4a9eff",
    "font_sans": "system-ui, -apple-system, sans-serif",
    "font_mono": "Consolas, Monaco, monospace",
    "content_max_width": "1100px",
    "spacing_base": "1rem"
  }
}
```

The compiler generates `tokens.css`:

```css
:root {
  --color-bg: #0b1220;
  --color-text: #e8eefc;
  --color-primary: #4a9eff;
  --font-sans: system-ui, -apple-system, sans-serif;
  /* ... */
}
```

Reference tokens in CSS:

```css
body {
  background: var(--color-bg);
  color: var(--color-text);
  font-family: var(--font-sans);
}
```

**Token overrides:** Users can override tokens in `site.json`:

```json
{
  "site_name": "My Site",
  "tokens": {
    "color_primary": "#ff6b6b",
    "font_sans": "Inter, sans-serif"
  }
}
```

## MDX Components

Components are HTML templates in `src/mdx-components/`:

**hero.html:**
```html
<!-- Props: headline, subheadline, cta_text, cta_href -->
<section class="hero">
  <h2>{{ .Props.headline }}</h2>
  <p>{{ .Props.subheadline }}</p>
  {{ if .Props.cta_text }}
    <a href="{{ .Props.cta_href }}" class="btn-primary">
      {{ .Props.cta_text }}
    </a>
  {{ end }}
</section>
```

**Used in markdown:**
```markdown
:::component Hero
headline: "Welcome to My Site"
subheadline: "Build amazing things"
cta_text: "Get Started"
cta_href: "/start"
:::
```

**Component naming:** File `hero.html` → component `Hero`, file `youtube-video.html` → component `YouTubeVideo`

**Component context:**
- `.Site` - Full site data
- `.Page` - Current page data
- `.Props` - Component properties (must be strings)

## Navigation

Navigation is auto-generated from content structure:

```html
<!-- nav.html -->
<nav>
  {{ .NavHTML }}
</nav>
```

The compiler generates:

```html
<ul class="nav-list">
  <li><a href="/" class="active">Home</a></li>
  <li><a href="/about">About</a>
    <ul class="nav-list">
      <li><a href="/about/team">Team</a></li>
    </ul>
  </li>
  <li><a href="/blog">Blog</a></li>
</ul>
```

Style it with CSS:

```css
.nav-list {
  list-style: none;
}

.nav-list a {
  color: var(--color-text);
}

.nav-list a.active {
  color: var(--color-primary);
  font-weight: bold;
}
```

## Assets

**Theme assets** in `src/assets/` are copied to `dist/assets/`:
- `src/assets/css/theme.css` → `dist/assets/css/theme.css`
- `src/assets/js/theme.js` → `dist/assets/js/theme.js`
- `src/assets/images/logo.svg` → `dist/assets/images/logo.svg`

**Reference in templates:**
```html
<link rel="stylesheet" href="/assets/css/tokens.css">
<link rel="stylesheet" href="/assets/css/theme.css">
<script src="/assets/js/theme.js"></script>
```

**Per-page assets** in `content/page/assets/` are copied to `dist/assets/pages/page/`:
- `content/about/assets/team.jpg` → `dist/assets/pages/about/team.jpg`

**Reference in markdown:**
```markdown
![Team Photo](/assets/pages/about/team.jpg)
```

## Creating a Theme

1. **Start with the starter theme:**
   ```bash
   cp -r pagewright/themes/starter pagewright/themes/my-theme
   ```

2. **Update tokens.json** with metadata and design tokens:
   ```json
   {
     "theme_name": "My Theme",
     "theme_version": "1.0.0",
     "theme_description": "A beautiful theme for portfolios and creative professionals",
     "compiler_version": ">=0.1.0",
     "tokens": { ... }
   }
   ```

3. **Customize layout templates** in `src/layout/`

4. **Style with CSS** in `src/assets/css/theme.css`

5. **Add custom components** in `src/mdx-components/`

6. **Test with the compiler:**
   ```bash
   cd pagewright/compiler
   ./pagewrightc build \
     --theme ../themes/my-theme \
     --content ./test-site/content \
     --out ./test-site/dist
   ```

7. **Rebuild themes service** to include your new theme:
   ```bash
   docker-compose build themes
   docker-compose up -d themes
   ```

## Best Practices

- ✅ Use semantic HTML5 elements
- ✅ Make navigation accessible (ARIA labels)
- ✅ Include mobile-responsive CSS
- ✅ Keep tokens minimal and reusable
- ✅ Document component props in HTML comments
- ✅ Test with various content structures
- ❌ Don't hardcode colors/fonts - use tokens
- ❌ Don't use external CDN dependencies (bundle locally)
- ❌ Don't assume specific content structure beyond folder tree

## Example Themes

- **starter** - Minimal responsive theme with sidebar
- *(more themes coming soon)*
