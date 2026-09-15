//go:build windows

package cmd

import "os"

// dockerEndpointMissing reports whether Docker Desktop is absent. Its named
// pipe only exists while the engine runs, so a missing pipe cannot distinguish
// "not installed" from "not started"; the install directory can.
func dockerEndpointMissing() bool {
	for _, envVar := range []string{"ProgramFiles", "ProgramW6432", "LOCALAPPDATA"} {
		// An unset variable would leave a drive-relative `\Docker`, which a
		// stray C:\Docker would then answer for.
		root := os.Getenv(envVar)
		if root == "" {
			continue
		}
		if _, err := os.Stat(root + `\Docker`); err == nil {
			return false
		}
	}
	return true
}
