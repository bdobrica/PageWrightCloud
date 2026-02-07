package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
)

// Load reads and merges configuration from theme and site
func Load(themeDir, contentDir, outputDir, baseURL string) (*types.BuildConfig, error) {
	// Load site.json
	siteConfigPath := filepath.Join(contentDir, "site.json")
	siteConfig, err := loadSiteConfig(siteConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load site config: %w", err)
	}

	// Validate required fields
	if siteConfig.SiteName == "" {
		return nil, &types.CompileError{
			File:    siteConfigPath,
			Message: "site_name is required in site.json",
		}
	}

	// Set defaults
	if siteConfig.Lang == "" {
		siteConfig.Lang = "en"
	}

	return &types.BuildConfig{
		ThemeDir:   themeDir,
		ContentDir: contentDir,
		OutputDir:  outputDir,
		BaseURL:    baseURL,
		SiteConfig: siteConfig,
	}, nil
}

// loadSiteConfig reads and parses site.json
func loadSiteConfig(path string) (*types.SiteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("site.json not found at %s (required)", path)
		}
		return nil, err
	}

	var config types.SiteConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, &types.CompileError{
			File:    path,
			Message: fmt.Sprintf("invalid JSON: %v", err),
		}
	}

	return &config, nil
}

// LoadThemeConfig reads and parses tokens.json from the theme
func LoadThemeConfig(themeDir string) (*types.ThemeConfig, error) {
	tokensPath := filepath.Join(themeDir, "tokens.json")
	data, err := os.ReadFile(tokensPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &types.CompileError{
				File:    tokensPath,
				Message: "tokens.json not found in theme directory",
			}
		}
		return nil, err
	}

	var config types.ThemeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, &types.CompileError{
			File:    tokensPath,
			Message: fmt.Sprintf("invalid JSON: %v", err),
		}
	}

	return &config, nil
}

// MergeTokens merges theme tokens with site.json overrides
func MergeTokens(themeTokens map[string]interface{}, siteTokens map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy theme tokens
	for k, v := range themeTokens {
		result[k] = v
	}

	// Apply site overrides
	for k, v := range siteTokens {
		result[k] = v
	}

	return result
}

// BuildSiteContext creates a Site context from config
func BuildSiteContext(config *types.BuildConfig, tokens map[string]interface{}) types.Site {
	year := time.Now().Year()

	site := types.Site{
		Name:      config.SiteConfig.SiteName,
		BaseURL:   config.BaseURL,
		Lang:      config.SiteConfig.Lang,
		Year:      year,
		LogoURL:   config.SiteConfig.LogoURL,
		Author:    config.SiteConfig.Author,
		Copyright: config.SiteConfig.Copyright,
	}

	if config.SiteConfig.PrimaryCTA != nil {
		site.PrimaryCTA = config.SiteConfig.PrimaryCTA
	}

	return site
}
