package content

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/util"
)

var pageSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Discover walks the content directory and builds a page tree
func Discover(contentRoot string) ([]*types.Page, error) {
	if err := util.CheckTree(contentRoot); err != nil {
		return nil, err
	}
	var pages []*types.Page
	pageMap := make(map[string]*types.Page)

	err := filepath.Walk(contentRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if not a directory
		if !info.IsDir() {
			return nil
		}
		if path != contentRoot && info.Name() == "assets" {
			return filepath.SkipDir
		}

		// Check if this directory has an index.md
		indexPath := filepath.Join(path, "index.md")
		mdInfo, mdErr := os.Stat(indexPath)
		mdxPath := filepath.Join(path, "index.mdx")
		mdxInfo, mdxErr := os.Stat(mdxPath)
		if mdErr != nil && !os.IsNotExist(mdErr) {
			return mdErr
		}
		if mdxErr != nil && !os.IsNotExist(mdxErr) {
			return mdxErr
		}
		if mdErr == nil && mdxErr == nil {
			return fmt.Errorf("ambiguous index.md/index.mdx in %s", path)
		}
		if mdErr != nil && mdxErr != nil {
			return nil
		}
		if mdErr == nil && !mdInfo.Mode().IsRegular() || mdxErr == nil && !mdxInfo.Mode().IsRegular() {
			return fmt.Errorf("page source is not a regular file: %s", path)
		}
		if mdErr != nil {
			indexPath = mdxPath
		}

		// Create page
		relPath, err := filepath.Rel(contentRoot, path)
		if err != nil {
			return err
		}
		if relPath != "." {
			for _, part := range strings.Split(filepath.ToSlash(relPath), "/") {
				if !pageSegment.MatchString(part) {
					return fmt.Errorf("invalid page directory %q", relPath)
				}
			}
		}

		page := &types.Page{
			Dir:      path,
			RelDir:   relPath,
			SourceMD: indexPath,
		}

		// Generate ID and slug
		if relPath == "." || relPath == "home" {
			page.ID = "home"
			page.Slug = "/"
		} else {
			// Remove "home" prefix if present
			cleanPath := strings.TrimPrefix(filepath.ToSlash(relPath), "home/")
			if cleanPath == "" {
				cleanPath = "home"
			}

			page.ID = strings.ReplaceAll(cleanPath, string(os.PathSeparator), "/")
			page.Slug = "/" + strings.ReplaceAll(cleanPath, string(os.PathSeparator), "/")
		}

		// Set output path
		if page.Slug == "/" {
			page.OutputPath = filepath.Join("index.html")
		} else {
			page.OutputPath = filepath.Join(strings.Trim(page.Slug, "/"), "index.html")
		}

		// Check for assets directory
		assetsDir := filepath.Join(path, "assets")
		if stat, err := os.Stat(assetsDir); err == nil && stat.IsDir() {
			page.AssetsDir = assetsDir
		}

		if _, exists := pageMap[page.ID]; exists {
			return fmt.Errorf("duplicate page route %s", page.Slug)
		}
		pages = append(pages, page)
		pageMap[page.ID] = page

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk content directory: %w", err)
	}

	if len(pages) == 0 {
		return nil, &types.CompileError{
			File:    contentRoot,
			Message: "no pages found (no directories with index.md or index.mdx)",
		}
	}

	// Build parent-child relationships
	for _, page := range pages {
		if page.ID == "home" {
			continue // home has no parent
		}

		// Attach to the closest ancestor page, falling back to home when a
		// grouping directory has no index of its own.
		parentID := filepath.ToSlash(filepath.Dir(page.ID))
		for {
			if parentID == "." {
				parentID = "home"
			}
			if parent, ok := pageMap[parentID]; ok {
				page.Parent = parent
				parent.Children = append(parent.Children, page)
				break
			}
			if parentID == "home" {
				break
			}
			parentID = filepath.ToSlash(filepath.Dir(parentID))
		}
	}

	// Sort children alphabetically by slug
	for _, page := range pages {
		sort.Slice(page.Children, func(i, j int) bool {
			return page.Children[i].Slug < page.Children[j].Slug
		})
	}

	return pages, nil
}

// FindHomePage returns the home page from the list
func FindHomePage(pages []*types.Page) *types.Page {
	for _, page := range pages {
		if page.ID == "home" {
			return page
		}
	}
	return nil
}
