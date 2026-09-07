package types

import (
	"regexp"
	"strings"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidHost requires canonical ASCII DNS names and leaves room for preview.
// External domain ownership is enforced at the gateway, not inferred here.
func ValidHost(host string) bool {
	if len(host) > 245 || !strings.Contains(host, ".") || strings.HasPrefix(host, "preview.") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !dnsLabel.MatchString(label) {
			return false
		}
	}
	return true
}
