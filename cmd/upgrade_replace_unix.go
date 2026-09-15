//go:build !windows

package cmd

import "os"

const artifactSuffix = ""

func versionedInstallSupported() bool { return true }

// replaceRunningBinary moves newPath over target. POSIX unlinks the old inode
// rather than refusing, so target may be the running executable.
func replaceRunningBinary(newPath, target string) error {
	return os.Rename(newPath, target)
}

// sweepReplacedBinaries is a no-op where the old file is unlinked immediately.
func sweepReplacedBinaries(string) {}
