package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pipelineGitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput() //nolint:gosec
	require.NoError(t, err, "%s", output)
	return string(output)
}

func pipelineGitRepository(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	pipelineGitCommand(t, dir, "init")
	pipelineGitCommand(t, dir, "config", "user.email", "test@example.com")
	pipelineGitCommand(t, dir, "config", "user.name", "Astro Test")
	specPath := filepath.Join(dir, "astropods.yml")
	require.NoError(t, os.WriteFile(specPath, []byte("name: example\n"), 0o600))
	pipelineGitCommand(t, dir, "add", "astropods.yml")
	pipelineGitCommand(t, dir, "commit", "-m", "feat: add example blueprint")
	return specPath, pipelineGitCommand(t, dir, "rev-parse", "HEAD")
}

func TestCollectGitMetadataAllowsACleanRepository(t *testing.T) {
	specPath, sha := pipelineGitRepository(t)
	reachedNextStep := false
	pipeline := NewPushPipeline(context.Background(), PushPipelineConfig{SpecPath: specPath}).
		CollectGitMetadata().
		step(func() error {
			reachedNextStep = true
			return nil
		})

	require.NoError(t, pipeline.Err())
	assert.True(t, reachedNextStep)
	assert.Equal(t, sha[:len(sha)-1], pipeline.gitMetadata.CommitSHA)
	assert.Equal(t, "feat: add example blueprint", pipeline.gitMetadata.CommitMessage)
}

func TestCollectGitMetadataRejectsADirtyRepositoryBeforeLaterSteps(t *testing.T) {
	specPath, _ := pipelineGitRepository(t)
	require.NoError(t, os.WriteFile(specPath, []byte("name: changed\n"), 0o600))
	reachedNextStep := false
	pipeline := NewPushPipeline(context.Background(), PushPipelineConfig{
		SpecPath:  specPath,
		Yes:       true,
		SkipBuild: true,
		SkipPush:  true,
	}).
		CollectGitMetadata().
		step(func() error {
			reachedNextStep = true
			return nil
		})

	require.EqualError(t, pipeline.Err(), "uncommitted changes detected in the Git working tree; commit them before running 'ast push'")
	assert.ErrorIs(t, pipeline.Err(), errDirtyWorkingTree)
	assert.False(t, reachedNextStep, "dirty worktrees must stop the remaining pipeline even with --yes or skip flags")
}
