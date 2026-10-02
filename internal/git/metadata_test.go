package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput() //nolint:gosec
	require.NoError(t, err, "%s", output)
	return string(output)
}

func committedRepository(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	gitCommand(t, dir, "init")
	gitCommand(t, dir, "config", "user.email", "test@example.com")
	gitCommand(t, dir, "config", "user.name", "Astro Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("initial\n"), 0o600))
	gitCommand(t, dir, "add", "tracked.txt")
	gitCommand(t, dir, "commit", "-m", "feat: initial blueprint", "-m", "Commit body")
	return dir, gitCommand(t, dir, "rev-parse", "HEAD")
}

func TestCollectMetadataReadsCommitFromAContainingRepository(t *testing.T) {
	dir, sha := committedRepository(t)
	nested := filepath.Join(dir, "blueprints", "example")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	metadata := CollectMetadata(nested)

	assert.Equal(t, sha[:len(sha)-1], metadata.CommitSHA)
	assert.Equal(t, "feat: initial blueprint\n\nCommit body", metadata.CommitMessage)
	assert.False(t, metadata.WorkingTreeDirty, "empty nested directories must not make the repository dirty")
}

func TestCollectMetadataDetectsWorkingTreeChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, dir string)
		dirty  bool
	}{
		{name: "clean", change: func(_ *testing.T, _ string) {}, dirty: false},
		{name: "unstaged", change: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("changed\n"), 0o600))
		}, dirty: true},
		{name: "staged", change: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged\n"), 0o600))
			gitCommand(t, dir, "add", "staged.txt")
		}, dirty: true},
		{name: "untracked", change: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o600))
		}, dirty: true},
		{name: "ignored", change: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0o600))
			gitCommand(t, dir, "add", ".gitignore")
			gitCommand(t, dir, "commit", "-m", "chore: ignore generated file")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("ignored\n"), 0o600))
		}, dirty: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := committedRepository(t)
			tt.change(t, dir)

			assert.Equal(t, tt.dirty, CollectMetadata(dir).WorkingTreeDirty)
		})
	}
}

func TestCollectMetadataDetectsModifiedSubmodules(t *testing.T) {
	submoduleSource, _ := committedRepository(t)
	parent, _ := committedRepository(t)
	gitCommand(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", submoduleSource, "dependency")
	gitCommand(t, parent, "commit", "-m", "chore: add dependency")
	require.NoError(t, os.WriteFile(filepath.Join(parent, "dependency", "tracked.txt"), []byte("changed\n"), 0o600))

	assert.True(t, CollectMetadata(parent).WorkingTreeDirty)
}

func TestCollectMetadataIsBestEffort(t *testing.T) {
	t.Run("git executable unavailable", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		assert.Equal(t, Metadata{}, CollectMetadata(t.TempDir()))
	})

	t.Run("outside a repository", func(t *testing.T) {
		assert.Equal(t, Metadata{}, CollectMetadata(t.TempDir()))
	})

	t.Run("repository without commits", func(t *testing.T) {
		dir := t.TempDir()
		gitCommand(t, dir, "init")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "astropods.yml"), []byte("kind: blueprint\n"), 0o600))

		assert.Equal(t, Metadata{WorkingTreeDirty: true}, CollectMetadata(dir))
	})

	t.Run("commit lookup failure does not discard status", func(t *testing.T) {
		metadata := collectMetadata("project", func(_ string, args ...string) (string, error) {
			if args[0] == "show" {
				return "", errors.New("no head")
			}
			return " M astropods.yml\n", nil
		})

		assert.Equal(t, Metadata{WorkingTreeDirty: true}, metadata)
	})

	t.Run("status failure does not discard commit", func(t *testing.T) {
		metadata := collectMetadata("project", func(_ string, args ...string) (string, error) {
			if args[0] == "status" {
				return "", errors.New("status failed")
			}
			return "abc123\x00Message\n", nil
		})

		assert.Equal(t, Metadata{CommitSHA: "abc123", CommitMessage: "Message"}, metadata)
	})
}
