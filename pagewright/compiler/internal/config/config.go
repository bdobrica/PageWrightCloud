package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/util"
)

// Load reads and merges configuration from theme and site
func Load(themeDir, contentDir, outputDir, baseURL string) (*types.BuildConfig, error) {
	if err := ValidatePaths(themeDir, contentDir, outputDir); err != nil {
		return nil, err
	}
	if baseURL != "" {
		u, err := url.Parse(baseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, fmt.Errorf("base URL must be an HTTP(S) origin")
		}
		baseURL = strings.TrimSuffix(baseURL, "/")
	}
	// Load site.json
	siteConfigPath := filepath.Join(contentDir, "site.json")
	siteConfig, err := loadSiteConfig(siteConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load site config: %w", err)
	}

	// Validate required fields
	if strings.TrimSpace(siteConfig.SiteName) == "" {
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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, &types.CompileError{
			File:    path,
			Message: fmt.Sprintf("invalid JSON: %v", err),
		}
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return nil, fmt.Errorf("%s: trailing JSON data", path)
	}
	if err := ValidateTokens(config.Tokens); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
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

	if strings.TrimSpace(config.ThemeName) == "" || len(config.Tokens) == 0 {
		return nil, fmt.Errorf("%s: theme_name and tokens are required", tokensPath)
	}
	if err := ValidateTokens(config.Tokens); err != nil {
		return nil, fmt.Errorf("%s: %w", tokensPath, err)
	}
	return &config, nil
}

func ValidatePaths(themeDir, contentDir, outputDir string) error {
	if themeDir == "" || contentDir == "" || outputDir == "" {
		return fmt.Errorf("theme, content and output directories are required")
	}
	for _, root := range []string{themeDir, contentDir} {
		if err := util.CheckPath(root, false); err != nil {
			return err
		}
	}
	if err := util.CheckPath(outputDir, true); err != nil {
		return err
	}
	themeAbs, err := util.Absolute(themeDir)
	if err != nil {
		return err
	}
	contentAbs, err := util.Absolute(contentDir)
	if err != nil {
		return err
	}
	outAbs, err := util.Absolute(outputDir)
	if err != nil {
		return err
	}
	for _, pair := range [][2]string{{themeAbs, contentAbs}, {themeAbs, outAbs}, {contentAbs, outAbs}} {
		if util.Overlap(pair[0], pair[1]) || util.Overlap(pair[1], pair[0]) {
			return fmt.Errorf("theme, content and output roots must not overlap")
		}
	}
	if err := util.CheckTree(themeAbs); err != nil {
		return err
	}
	if err := util.CheckTree(contentAbs); err != nil {
		return err
	}
	return util.CheckPath(outAbs, true)
}

var tokenName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var tokenValue = regexp.MustCompile(`^[A-Za-z0-9#.,%() +/_-]+$`)

// MVP tokens are simple CSS values, not arbitrary declarations or URL loads.
func ValidateTokens(tokens map[string]interface{}) error {
	for name, value := range tokens {
		s, ok := value.(string)
		if !tokenName.MatchString(name) || !ok || strings.TrimSpace(s) == "" {
			return fmt.Errorf("invalid token %q: expected named nonempty string", name)
		}
		if strings.HasPrefix(name, "site_") || name == "theme_name" {
			continue
		}
		if !tokenValue.MatchString(s) || strings.Contains(strings.ToLower(s), "url(") {
			return fmt.Errorf("unsafe CSS token %q", name)
		}
	}
	return nil
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
