package types

import (
	"strings"
	"testing"
)

func TestCanonicalHost(t *testing.T) {
	for _, host := range []string{"demo.pagewright.io", "demo.sites.pagewright.io", "legacy.example.test"} {
		if !ValidHost(host) {
			t.Errorf("rejected %q", host)
		}
	}
	for _, host := range []string{"../outside", "a..test", "-a.test", "a-.test", "a.test.", "A.test", "a.test;", "a.test\nroot /tmp;", "preview.a.test", "a_test.example", strings.Repeat("a", 64) + ".test", "a%2ftest", "000-maintenance"} {
		if ValidHost(host) {
			t.Errorf("accepted %q", host)
		}
	}
}
