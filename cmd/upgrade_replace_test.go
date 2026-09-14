package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestArtifactSuffix_MatchesTheReleasedName(t *testing.T) {
	want := ""
	if runtime.GOOS == "windows" {
		want = ".exe"
	}
	// The release names Windows artifacts with .exe; a mismatch here makes
	// `ast upgrade` request a URL that does not exist.
	if artifactSuffix != want {
		t.Fatalf("artifactSuffix = %q, want %q for %s", artifactSuffix, want, runtime.GOOS)
	}
}

func TestReplaceRunningBinary_PutsTheNewContentInPlace(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ast"+artifactSuffix)
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(dir, ".ast-upgrade-tmp")
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceRunningBinary(staged, target); err != nil {
		t.Fatalf("replaceRunningBinary: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("target unreadable after replace: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("target content = %q, want %q", got, "new")
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Error("staged file survived the replace, so the install dir accumulates temp files")
	}
}

func TestReplaceRunningBinary_WorksWhenNoTargetExists(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, ".ast-upgrade-tmp")
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "ast"+artifactSuffix)

	if err := replaceRunningBinary(staged, target); err != nil {
		t.Fatalf("replaceRunningBinary with no existing target: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target missing after replace: %v", err)
	}
}

func TestReplaceRunningBinary_KeepsTheOriginalWhenTheMoveFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ast"+artifactSuffix)
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory cannot be renamed over a file, so the move fails after the
	// original has been moved aside — the rollback path.
	staged := filepath.Join(dir, "staged-dir")
	if err := os.Mkdir(staged, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceRunningBinary(staged, target); err == nil {
		t.Skip("this platform allows the move; the rollback path is unreachable here")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("original lost after a failed replace: %v", err)
	}
	if string(got) != "old" {
		t.Errorf("target content = %q, want the original %q", got, "old")
	}
}

func TestSweepReplacedBinaries_LeavesTheLiveBinary(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "ast"+artifactSuffix)
	if err := os.WriteFile(live, []byte("live"), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := live + ".old"
	if err := os.WriteFile(stale, []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}

	sweepReplacedBinaries(dir)

	if _, err := os.Stat(live); err != nil {
		t.Errorf("sweep removed the live binary: %v", err)
	}
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Error("sweep left a .old leftover behind")
		}
		return
	}
	// Elsewhere the sweep is a no-op, since the old file is unlinked at once.
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("sweep should not touch anything off Windows: %v", err)
	}
}

func TestVersionedInstallSupported_MatchesSymlinkAvailability(t *testing.T) {
	// Windows symlinks need elevation or developer mode, so the versioned
	// scheme has to fall through to a direct replace there.
	if runtime.GOOS == "windows" && versionedInstallSupported() {
		t.Fatal("versionedInstallSupported() = true on Windows, which would try to symlink")
	}
	if runtime.GOOS != "windows" && !versionedInstallSupported() {
		t.Fatalf("versionedInstallSupported() = false on %s", runtime.GOOS)
	}
}
