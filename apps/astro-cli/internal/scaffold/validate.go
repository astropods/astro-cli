package scaffold

import (
	"fmt"
	"os"
	"regexp"
)

var (
	// Name must be lowercase alphanumeric with hyphens, start with letter, end with alphanumeric
	nameRegex     = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)
	reservedNames = map[string]bool{
		"astro": true,
		"agent": true,
		"model": true,
		"tool":  true,
	}
)

// ValidateName checks if the agent name is valid.
// Rules:
// - Lowercase alphanumeric with hyphens
// - Must start with a letter
// - Must end with alphanumeric
// - Max 63 characters
// - Cannot be a reserved name
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if len(name) > 63 {
		return fmt.Errorf("name cannot exceed 63 characters")
	}

	// Single character names are valid if they're a letter
	if len(name) == 1 {
		if name[0] >= 'a' && name[0] <= 'z' {
			if reservedNames[name] {
				return fmt.Errorf("'%s' is a reserved name", name)
			}
			return nil
		}
		return fmt.Errorf("name must start with a lowercase letter")
	}

	if !nameRegex.MatchString(name) {
		return fmt.Errorf("name must be lowercase alphanumeric with hyphens, start with a letter, and end with alphanumeric")
	}

	if reservedNames[name] {
		return fmt.Errorf("'%s' is a reserved name", name)
	}

	return nil
}

// ValidateDirectory checks if the target directory can be created.
// Returns an error if the directory already exists.
func ValidateDirectory(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("directory '%s' already exists", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check directory: %w", err)
	}
	return nil
}
