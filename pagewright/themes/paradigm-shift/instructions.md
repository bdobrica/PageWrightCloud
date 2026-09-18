# Paradigm Shift for PageWrightCloud

An adaptation of [Paradigm Shift by HTML5 UP / AJ](https://html5up.net/paradigm-shift).
The theme and its modifications are **CC BY 3.0**, independently of the
application's license. Keep the visible design credit and license link in
the footer. The full upstream license is in LICENSE.txt and is also copied
into generated sites under assets/licenses/, with attribution and change notes.
UPSTREAM-README.txt preserves the original release notes and credits; its list
of dependencies describes the original distribution, not this adaptation.

## Using the theme

From pagewright/compiler, build the included example into a new output directory:

```sh
example_dir=$(mktemp -d)
cp -R ../themes/paradigm-shift/example/content "$example_dir/content"
go run ./cmd/pagewrightc build \
  --theme ../themes/paradigm-shift \
  --content "$example_dir/content" \
  --out /tmp/pagewright-paradigm-preview
```

Serve the output directory with any static HTTP server. Output must be absent
or empty; choose a fresh output directory for subsequent builds.
The example is copied outside the theme because the compiler requires separate
theme, content, and output trees.
The themes service automatically discovers this directory on its next rebuild:
`docker compose build themes && docker compose up -d themes`.
This packages the theme; it does not add compiler integration to the worker.

Use ordinary Markdown and a single first-level heading for each page's title.
The title is displayed in the colored column; the first content H1 is visually
hidden to avoid displaying it twice. Navigation follows the content directory
tree. The site name, logo, primary CTA, copyright, and language use site.json.
The layout supports plain Markdown pages as well as component-based pages.

Tokens available for site.json overrides: color_primary, color_bg, color_text,
font_sans, and font_heading. The original geometric decoration retains its mint
tint. This is a light theme; choose colors with sufficient text contrast.

## Components

Properties use the compiler's `key: JSON-value` syntax. Text is escaped, not
interpreted as HTML or Markdown. Optional properties can be omitted.

| Component | Properties |
| --- | --- |
| Hero | headline, subheadline, image_src, image_alt, cta_text, cta_href |
| Feature | title, text, image_src, image_alt |
| Gallery | images: array of objects with src, alt, optional caption and href |
| CallToAction | title, text, label, href |
| YouTubeVideo | video_id, title, caption |

All leaf values must be strings. Gallery's JSON array must be on one line.
Use informative image alt text, or an empty string for decorative images.
Image paths can reference per-page assets, e.g. /assets/pages/home/work.svg.
Gallery links open the supplied URL directly; no modal or script is required.
YouTubeVideo loads an external YouTube privacy-enhanced embed when used.

```markdown
:::component Feature
title: "What we do"
text: "Thoughtful design for ambitious projects."
image_src: "/assets/pages/home/work.svg"
image_alt: "An example of our work"
:::
```

The example covers a landing page, a plain Markdown page, a nested page, and
all five components. Replace the included geometric placeholder with your own
media. No upstream demo photos, external fonts, icon fonts, jQuery, or contact
form are included. The original stylesheet is adapted in src/assets/css/main.css;
PageWrightCloud-specific styling is in src/assets/css/theme.css.
