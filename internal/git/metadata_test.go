package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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
	assert.True(t, metadata.WorkingTreeStatusKnown)
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

		assert.Equal(t, Metadata{WorkingTreeDirty: true, WorkingTreeStatusKnown: true}, CollectMetadata(dir))
	})

	t.Run("commit lookup failure does not discard status", func(t *testing.T) {
		metadata := collectMetadata("project", []string{"project"}, func(_ string, args ...string) (string, error) {
			if args[0] == "show" {
				return "", errors.New("no head")
			}
			if args[0] == "rev-parse" {
				return filepath.Abs("project")
			}
			return " M astropods.yml\n", nil
		})

		assert.Equal(t, Metadata{WorkingTreeDirty: true, WorkingTreeStatusKnown: true}, metadata)
	})

	t.Run("status failure does not discard commit", func(t *testing.T) {
		metadata := collectMetadata("project", []string{"project"}, func(_ string, args ...string) (string, error) {
			if args[0] == "rev-parse" {
				return filepath.Abs("project")
			}
			if args[0] == "--no-optional-locks" {
				return "", errors.New("status failed")
			}
			return "abc123\x00Message\n", nil
		})

		assert.Equal(t, Metadata{CommitSHA: "abc123", CommitMessage: "Message"}, metadata)
	})
}

func TestCollectMetadataChecksOnlyRelevantPaths(t *testing.T) {
	dir, _ := committedRepository(t)
	alpha := filepath.Join(dir, "blueprints", "alpha")
	beta := filepath.Join(dir, "blueprints", "beta")
	shared := filepath.Join(dir, "packages", "shared")
	for _, path := range []string{alpha, beta, shared} {
		require.NoError(t, os.MkdirAll(path, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(path, "source.txt"), []byte("initial\n"), 0o600))
	}
	gitCommand(t, dir, "add", "blueprints", "packages")
	gitCommand(t, dir, "commit", "-m", "feat: add blueprints")

	require.NoError(t, os.WriteFile(filepath.Join(beta, "source.txt"), []byte("changed\n"), 0o600))
	assert.False(t, CollectMetadata(alpha, alpha, shared).WorkingTreeDirty, "an unrelated sibling blueprint must not block")

	require.NoError(t, os.WriteFile(filepath.Join(shared, "source.txt"), []byte("changed\n"), 0o600))
	assert.True(t, CollectMetadata(alpha, alpha, shared).WorkingTreeDirty, "a declared shared context must block")
}

func TestCollectMetadataMarksMultipleRepositoriesIncomplete(t *testing.T) {
	specRepo, sha := committedRepository(t)
	contextRepo, _ := committedRepository(t)

	metadata := CollectMetadata(specRepo, specRepo, contextRepo)

	assert.Equal(t, strings.TrimSpace(sha), metadata.CommitSHA)
	assert.False(t, metadata.WorkingTreeDirty)
	assert.False(t, metadata.WorkingTreeStatusKnown, "one SHA cannot fully describe multiple repositories")

	require.NoError(t, os.WriteFile(filepath.Join(contextRepo, "tracked.txt"), []byte("changed\n"), 0o600))
	metadata = CollectMetadata(specRepo, specRepo, contextRepo)
	assert.True(t, metadata.WorkingTreeDirty, "dirtiness remains actionable even when provenance is incomplete")
	assert.False(t, metadata.WorkingTreeStatusKnown)
}

func TestCollectMetadataUsesNoOptionalLocks(t *testing.T) {
	var statusArgs []string
	root, err := filepath.Abs("project")
	require.NoError(t, err)
	collectMetadata("project", []string{"project"}, func(_ string, args ...string) (string, error) {
		switch args[0] {
		case "show":
			return "", errors.New("no head")
		case "rev-parse":
			return root, nil
		default:
			statusArgs = append([]string(nil), args...)
			return "", nil
		}
	})

	assert.Equal(t, []string{"--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=normal", "--", "."}, statusArgs)
}

func TestCollectMetadataTruncatesCommitMessagesAtAUTF8Boundary(t *testing.T) {
	root, err := filepath.Abs("project")
	require.NoError(t, err)
	message := strings.Repeat("a", MaxCommitMessageBytes-1) + "é"
	metadata := collectMetadata("project", []string{"project"}, func(_ string, args ...string) (string, error) {
		switch args[0] {
		case "show":
			return strings.Repeat("a", 40) + "\x00" + message, nil
		case "rev-parse":
			return root, nil
		default:
			return "", nil
		}
	})

	assert.LessOrEqual(t, len(metadata.CommitMessage), MaxCommitMessageBytes)
	assert.True(t, utf8.ValidString(metadata.CommitMessage))
	assert.Equal(t, strings.Repeat("a", MaxCommitMessageBytes-1), metadata.CommitMessage)
}
