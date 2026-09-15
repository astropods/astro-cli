package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeUpgradeFixture lays down a live binary and returns the install dir,
// the target path and a staged replacement.
func writeUpgradeFixture(t *testing.T, live string) (dir, target, staged string) {
	t.Helper()
	dir = t.TempDir()
	target = filepath.Join(dir, "ast"+artifactSuffix)
	if live != "" {
		require.NoError(t, os.WriteFile(target, []byte(live), 0o755))
	}
	staged = filepath.Join(dir, ".ast-upgrade-tmp")
	return dir, target, staged
}

func TestArtifactSuffix_MatchesTheReleasedName(t *testing.T) {
	want := ""
	if runtime.GOOS == "windows" {
		want = ".exe"
	}
	// The release names Windows artifacts with .exe; a mismatch here makes
	// `ast upgrade` request a URL that does not exist.
	assert.Equal(t, want, artifactSuffix)
}

func TestReplaceRunningBinary_PutsTheNewContentInPlace(t *testing.T) {
	_, target, staged := writeUpgradeFixture(t, "old")
	require.NoError(t, os.WriteFile(staged, []byte("new"), 0o755))

	require.NoError(t, replaceRunningBinary(staged, target))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	assert.NoFileExists(t, staged,
		"the staged file survived, so the install dir accumulates temp files")
}

func TestReplaceRunningBinary_WorksWhenNoTargetExists(t *testing.T) {
	_, target, staged := writeUpgradeFixture(t, "")
	require.NoError(t, os.WriteFile(staged, []byte("new"), 0o755))

	require.NoError(t, replaceRunningBinary(staged, target))
	assert.FileExists(t, target)
}

func TestReplaceRunningBinary_KeepsTheOriginalWhenTheMoveFails(t *testing.T) {
	dir, target, _ := writeUpgradeFixture(t, "old")
	// A directory cannot be renamed over a file, so the move fails after the
	// original has been moved aside — the rollback path.
	staged := filepath.Join(dir, "staged-dir")
	require.NoError(t, os.Mkdir(staged, 0o755))

	if err := replaceRunningBinary(staged, target); err == nil {
		t.Skip("this platform allows the move; the rollback path is unreachable here")
	}

	got, err := os.ReadFile(target)
	require.NoError(t, err, "the original was lost after a failed replace")
	assert.Equal(t, "old", string(got))
}

func TestSweepReplacedBinaries_LeavesTheLiveBinary(t *testing.T) {
	dir, live, _ := writeUpgradeFixture(t, "live")
	stale := live + ".old"
	require.NoError(t, os.WriteFile(stale, []byte("stale"), 0o755))

	sweepReplacedBinaries(dir)

	assert.FileExists(t, live, "the sweep removed the live binary")
	if runtime.GOOS == "windows" {
		assert.NoFileExists(t, stale, "the sweep left a .old leftover behind")
		return
	}
	// Elsewhere the sweep is a no-op, since the old file is unlinked at once.
	assert.FileExists(t, stale, "the sweep should not touch anything off Windows")
}

func TestVersionedInstallSupported_MatchesSymlinkAvailability(t *testing.T) {
	// Windows symlinks need elevation or developer mode, so the versioned
	// scheme has to fall through to a direct replace there.
	assert.Equal(t, runtime.GOOS != "windows", versionedInstallSupported())
}
