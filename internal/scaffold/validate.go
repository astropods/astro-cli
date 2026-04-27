package scaffold

import (
	"fmt"
	"os"
)

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
