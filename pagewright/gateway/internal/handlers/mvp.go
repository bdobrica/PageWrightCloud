package handlers

import (
	"fmt"
	"net/http"
	"strings"
)

// This release is MVP-only: there is no switch to enable unaccepted workflows.
func mvpUnavailable(w http.ResponseWriter, feature string) {
	w.Header().Set("Cache-Control", "no-store")
	respondError(w, http.StatusNotImplemented, feature+" is unavailable in the MVP")
}

// SetSiteDomain configures creation only; existing sites are not renamed or deleted.
func (h *SitesHandler) SetSiteDomain(domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !validSiteFQDN(domain) || len(domain) > 181 {
		return fmt.Errorf("invalid PAGEWRIGHT_SITE_DOMAIN")
	}
	h.siteDomain = domain
	return nil
}

func (h *SitesHandler) supportedSiteName(fqdn string) bool {
	suffix := "." + h.siteDomain
	if h.siteDomain == "" || !validSiteFQDN(fqdn) || !strings.HasSuffix(fqdn, suffix) {
		return false
	}
	label := strings.TrimSuffix(fqdn, suffix)
	if !domainLabel.MatchString(label) || strings.HasPrefix(label, "xn--") {
		return false
	}
	switch label {
	case "preview", "www", "api", "app", "admin", "auth", "assets", "cdn", "mail", "status", "support", "ns1", "ns2":
		return false
	}
	return true
}

// Capabilities is public, non-secret and authoritative for the creation UI.
func (h *SitesHandler) Capabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, map[string]interface{}{
		"mode": "mvp", "site_domain": h.siteDomain,
		"attachments": false, "custom_domains": false, "aliases": false,
		"oauth": false, "site_deletion": false,
		"registration_open": h.RegistrationOpen,
	})
}
