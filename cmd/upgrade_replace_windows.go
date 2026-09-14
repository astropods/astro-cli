//go:build windows

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const artifactSuffix = ".exe"

// versionedInstallSupported is false: the symlink it relies on needs elevation
// or developer mode on Windows.
func versionedInstallSupported() bool { return false }

const replacedSuffix = ".old"

// replaceRunningBinary swaps newPath in for target. Windows refuses to delete
// or overwrite a running executable but allows renaming one, so the old file is
// moved aside first and deleted on a later run.
func replaceRunningBinary(newPath, target string) error {
	aside := target + replacedSuffix
	_ = os.Remove(aside)

	if err := os.Rename(target, aside); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("move the running binary aside: %w", err)
	}
	if err := os.Rename(newPath, target); err != nil {
		// Put the original back rather than leaving no binary at all.
		_ = os.Rename(aside, target)
		return err
	}
	// Fails while this process still holds the handle; sweepReplacedBinaries
	// clears it next run.
	_ = os.Remove(aside)
	return nil
}

// sweepReplacedBinaries removes leftovers a previous upgrade could not delete
// because it was running from them.
func sweepReplacedBinaries(installDir string) {
	entries, err := os.ReadDir(installDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), replacedSuffix) {
			_ = os.Remove(filepath.Join(installDir, e.Name()))
		}
	}
}
