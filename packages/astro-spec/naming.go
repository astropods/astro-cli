package spec

import (
	"fmt"
	"regexp"
)

var (
	// nameRegex matches agent names: lowercase alphanumeric with hyphens,
	// must start with a letter and end with alphanumeric.
	// Length is enforced separately (4–63 characters).
	nameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

	// ReservedNames is the set of names that cannot be used for agents.
	// Enforced by both the CLI and the server.
	ReservedNames = map[string]bool{
		"astro":       true,
		"agent":       true,
		"model":       true,
		"integration": true,
	}

	// varNameRegex matches environment variable names following POSIX conventions:
	// letters (upper or lower), digits, and underscores; must start with a letter
	// or underscore. No length limit beyond what is practical.
	varNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// ValidateName checks whether name is a valid Astro agent name.
// Rules:
//   - 4–63 characters
//   - lowercase alphanumeric with hyphens only
//   - must start with a lowercase letter
//   - must end with alphanumeric
//   - cannot be a reserved platform name (astro, agent, model, integration)
func ValidateName(name string) error {
	if len(name) < 4 {
		return fmt.Errorf("name must be at least 4 characters")
	}
	if len(name) > 63 {
		return fmt.Errorf("name cannot exceed 63 characters")
	}
	if ReservedNames[name] {
		return fmt.Errorf("'%s' is a reserved name", name)
	}
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("name must be lowercase alphanumeric with hyphens, start with a letter, and end with alphanumeric")
	}
	return nil
}

// IsValidName reports whether name passes ValidateName.
func IsValidName(name string) bool {
	return ValidateName(name) == nil
}

// ValidateVarName checks whether name is a valid variable (secret) name.
// Rules: letters, digits, and underscores only; must start with a letter or underscore.
func ValidateVarName(name string) error {
	if !varNameRegex.MatchString(name) {
		return fmt.Errorf("variable name must start with a letter or underscore and contain only letters, digits, or underscores")
	}
	return nil
}

// IsValidVarName reports whether name passes ValidateVarName.
func IsValidVarName(name string) bool {
	return ValidateVarName(name) == nil
}
