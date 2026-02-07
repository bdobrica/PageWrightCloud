# Starter Theme

This theme is a minimal responsive layout with:

- Sticky header
- Collapsible nav on mobile
- Optional sidebar (TOC + page/sidebar content)
- Body region for compiled MDX content
- Footer

## Required compile-time variables

- site.lang (e.g. "en")
- site.name
- site.base_url
- site.year
- site.logo_url (optional)
- site.primary_cta (optional): {label, href}
- site.sidebar_default_html (optional)
- site.footer_links_html (optional)

- page.title
- page.description (optional)
- page.updated_at (optional)
- page.toc_html (optional)
- page.sidebar_html (optional)
- page.breadcrumb_html (optional)

- nav_html (generated from content folder structure)
- content_html (result of MDX-to-HTML rendering)

## Theme tokens

tokens.json defines tokens that should be converted to CSS variables.
The compiler should replace:
- {{ token.* }} placeholders in CSS
- {{ site.* }} and {{ page.* }} placeholders in HTML

## Notes

- /storage should never be emitted into output.
- Prefer generating nav_html as nested <ul> lists with class "nav-list".
