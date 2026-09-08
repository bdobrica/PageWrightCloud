package auth

import (
	"strings"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	for _, value := range []string{"", "1234567", strings.Repeat("a", 73), strings.Repeat("é", 37), "abcdefg\xff"} {
		if ValidatePassword(value) == nil {
			t.Fatal("accepted invalid password")
		}
	}
	for _, value := range []string{"12345678", strings.Repeat("a", 72), strings.Repeat("é", 8), " eight spaces "} {
		if err := ValidatePassword(value); err != nil {
			t.Fatal(err)
		}
	}
}
