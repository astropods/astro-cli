//go:build !windows

package cmd

import "os"

// artifactSuffix is appended to the released binary name.
const artifactSuffix = ""

// versionedInstallSupported reports whether the versioned-binary-plus-symlink
// install can be used.
func versionedInstallSupported() bool { return true }

// replaceRunningBinary moves newPath over target, which may be the running
// executable.
func replaceRunningBinary(newPath, target string) error {
	return os.Rename(newPath, target)
}

// sweepReplacedBinaries is a no-op where the old file is unlinked immediately.
func sweepReplacedBinaries(string) {}
