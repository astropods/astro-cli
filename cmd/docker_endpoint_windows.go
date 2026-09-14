//go:build windows

package cmd

import "os"

// dockerEndpointMissing reports whether Docker Desktop is absent. Its named
// pipe only exists while the engine runs, so a missing pipe cannot distinguish
// "not installed" from "not started"; the install directory can.
func dockerEndpointMissing() bool {
	for _, dir := range []string{
		os.Getenv("ProgramFiles") + `\Docker`,
		os.Getenv("ProgramW6432") + `\Docker`,
		os.Getenv("LOCALAPPDATA") + `\Docker`,
	} {
		if _, err := os.Stat(dir); err == nil {
			return false
		}
	}
	return true
}
