package util

import (
	"regexp"
	"strings"
)

// Slugify converts a string to a URL-safe slug
func Slugify(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace spaces and underscores with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")

	// Remove non-alphanumeric characters except hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	s = reg.ReplaceAllString(s, "")

	// Remove consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	s = reg.ReplaceAllString(s, "-")

	// Trim hyphens
	s = strings.Trim(s, "-")

	return s
}

// SanitizeID creates an HTML-safe ID from a string
func SanitizeID(s string) string {
	return Slugify(s)
}

// TitleCase converts a slug to title case
// e.g., "about-us" -> "About Us"
func TitleCase(s string) string {
	words := strings.Split(s, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}
