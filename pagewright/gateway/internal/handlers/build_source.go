package handlers

import (
	"fmt"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

// Select once per new reservation. Idempotent retries replay the persisted base.
// Manifest-committed builds remain usable even if their manager callback is lost.
func (h *BuildHandler) selectBuildSource(site *types.Site) (string, error) {
	if h.storageClient == nil {
		return "", fmt.Errorf("storage client required")
	}
	stored, err := h.storageClient.ListVersions(site.ID)
	if err != nil {
		return "", err
	}
	versions, err := normalizeVersions(site.ID, stored)
	if err != nil {
		return "", err
	}
	for _, version := range versions {
		if version.BuildID != "initial" {
			return version.BuildID, nil
		}
	}
	if site.LiveVersionID != nil && *site.LiveVersionID != "" {
		if !versionIDPattern.MatchString(*site.LiveVersionID) {
			return "", fmt.Errorf("invalid live version")
		}
		return *site.LiveVersionID, nil
	}
	return "initial", nil
}
