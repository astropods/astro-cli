package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolatedGit pins a commit identity and an empty config, so a test never reads
// or writes the developer's own git config to make its commit.
func isolatedGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "Astro Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Astro Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(context.Background(), dir, args...)
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return out
}

func TestInitGitRepo_CommitsTheScaffold(t *testing.T) {
	isolatedGit(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "astropods.yml"), []byte("name: test-agent\n"), 0o600))

	var out strings.Builder
	initGitRepo(context.Background(), &out, dir)

	assert.Contains(t, out.String(), msgGitRepoInitialized())
	assert.Equal(t, msgInitialCommitSubject(), gitOutput(t, dir, "log", "-1", "--format=%s"))
	assert.Empty(t, gitOutput(t, dir, "status", "--porcelain"),
		"the scaffold should be committed, not left staged or untracked")
}

func TestInitGitRepo_SkipsInsideExistingRepo(t *testing.T) {
	isolatedGit(t)
	outer := t.TempDir()
	gitOutput(t, outer, "init")
	nested := filepath.Join(outer, "test-agent")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	var out strings.Builder
	initGitRepo(context.Background(), &out, nested)

	assert.Contains(t, out.String(), msgGitInitSkipped(msgGitAlreadyInRepo()))
	assert.NoDirExists(t, filepath.Join(nested, ".git"),
		"a repository nested inside an existing checkout is never tracked by the outer one")
}

func TestRunCreate_InitializesGitRepo(t *testing.T) {
	isolatedGit(t)
	stubInteractiveTerminal(t, false)
	dir, out := runCreateInTempDir(t, "versioned-agent", "--yes")

	assert.Contains(t, out, msgGitRepoInitialized())
	assert.DirExists(t, filepath.Join(dir, ".git"))
	assert.Equal(t, msgInitialCommitSubject(), gitOutput(t, dir, "log", "-1", "--format=%s"))
	assert.Empty(t, gitOutput(t, dir, "status", "--porcelain"))
}

func TestRunCreate_NoGitSkipsInitialization(t *testing.T) {
	isolatedGit(t)
	stubInteractiveTerminal(t, false)
	dir, out := runCreateInTempDir(t, "unversioned-agent", "--yes", "--no-git")

	assert.NoDirExists(t, filepath.Join(dir, ".git"))
	assert.NotContains(t, out, msgGitRepoInitialized())
	assert.NotContains(t, out, "Skipped git init",
		"--no-git is the user's choice, not a failure worth reporting")
}
