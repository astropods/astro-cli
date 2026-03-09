package account

import (
	"fmt"
	"strings"
	"unicode"
)

// reserved names that cannot be used as account names
var reservedNames = map[string]bool{
	"admin":       true,
	"api":         true,
	"auth":        true,
	"deploy":      true,
	"health":      true,
	"login":       true,
	"logout":      true,
	"new":         true,
	"onboarding":  true,
	"operator":    true,
	"register":    true,
	"settings":    true,
	"status":      true,
	"support":     true,
	"system":      true,
	"www":         true,
	"hire":        true,
	"dev":         true,
	"agents":      true,
	"deployments": true,
	"accounts":    true,
	"me":          true,
	"ready":       true,
	"undeploy":    true,
	"schema":      true,
}

// ValidateAccountName validates an account name format:
// - 4-39 characters
// - lowercase alphanumeric + hyphens only
// - must start with a letter
// - must not end with a hyphen
// - no consecutive hyphens
func ValidateAccountName(name string) error {
	if len(name) < 4 {
		return fmt.Errorf("account name must be at least 4 characters")
	}
	if len(name) > 39 {
		return fmt.Errorf("account name must be at most 39 characters")
	}

	// Must be lowercase
	if name != strings.ToLower(name) {
		return fmt.Errorf("account name must be lowercase")
	}

	// Must start with a letter
	if !unicode.IsLetter(rune(name[0])) {
		return fmt.Errorf("account name must start with a letter")
	}

	// Must not end with a hyphen
	if name[len(name)-1] == '-' {
		return fmt.Errorf("account name must not end with a hyphen")
	}

	// Check each character and no consecutive hyphens
	prevHyphen := false
	for _, ch := range name {
		if ch == '-' {
			if prevHyphen {
				return fmt.Errorf("account name must not contain consecutive hyphens")
			}
			prevHyphen = true
			continue
		}
		prevHyphen = false
		if !unicode.IsLower(ch) && !unicode.IsDigit(ch) {
			return fmt.Errorf("account name must contain only lowercase letters, digits, and hyphens")
		}
	}

	return nil
}

// CheckAccountNameRestricted checks if a name is reserved or denied.
// Call this in addition to ValidateAccountName for user-facing registration
// where brand/system names should be blocked.
func CheckAccountNameRestricted(name string) error {
	if reservedNames[name] {
		return fmt.Errorf("account name %q is reserved", name)
	}
	if deniedNames[name] {
		return fmt.Errorf("account name %q is reserved for brand use — contact support to request it", name)
	}
	return nil
}
