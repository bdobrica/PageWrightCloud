package content

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
)

// Discover walks the content directory and builds a page tree
func Discover(contentRoot string) ([]*types.Page, error) {
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

		// Check if this directory has an index.md
		indexPath := filepath.Join(path, "index.md")
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			return nil // no index.md, skip this directory
		}

		// Create page
		relPath, err := filepath.Rel(contentRoot, path)
		if err != nil {
			return err
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
			cleanPath := strings.TrimPrefix(relPath, "home/")
			cleanPath = strings.TrimPrefix(cleanPath, "home")
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
			Message: "no pages found (no directories with index.md)",
		}
	}

	// Build parent-child relationships
	for _, page := range pages {
		if page.ID == "home" {
			continue // home has no parent
		}

		// Find parent by looking at the directory structure
		parentPath := filepath.Dir(page.RelDir)
		if parentPath == "." {
			// Parent is home
			if homePage, ok := pageMap["home"]; ok {
				page.Parent = homePage
				homePage.Children = append(homePage.Children, page)
			}
		} else {
			parentID := strings.ReplaceAll(parentPath, string(os.PathSeparator), "/")
			parentID = strings.TrimPrefix(parentID, "home/")
			parentID = strings.TrimPrefix(parentID, "home")

			if parentID == "" || parentID == "." {
				parentID = "home"
			}

			if parent, ok := pageMap[parentID]; ok {
				page.Parent = parent
				parent.Children = append(parent.Children, page)
			}
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
