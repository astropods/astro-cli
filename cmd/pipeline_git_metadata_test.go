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

	"github.com/astropods/astro-cli/internal/tui"
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
		SpecPath: specPath,
		Yes:      true,
	}).
		CollectGitMetadata().
		step(func() error {
			reachedNextStep = true
			return nil
		})

	require.EqualError(t, pipeline.Err(), "push blocked: files used by this blueprint have uncommitted changes; commit them, or rerun this command with --allow-dirty to push anyway")
	assert.ErrorIs(t, pipeline.Err(), errDirtyWorkingTree)
	assert.False(t, reachedNextStep, "--yes must not bypass the dirty-input check")
}

func TestDirtyPushPromptCopyAndDefault(t *testing.T) {
	choice := dirtyPushCancelChoice
	selectField := newDirtyPushSelect(&choice)
	selectField.WithTheme(cliHuhTheme())
	selectField.WithWidth(120)
	_ = selectField.Init()
	_ = selectField.Focus()

	rendered := stripANSI(selectField.View())
	compact := strings.Join(strings.Fields(strings.ReplaceAll(rendered, "┃", "")), " ")
	assert.Contains(t, compact, "Push with uncommitted changes?")
	assert.Contains(t, compact, dirtyPushPromptDescription)
	assert.Contains(t, compact, "Cancel the push")
	assert.Contains(t, compact, "Push with uncommitted changes")
	assert.Equal(t, dirtyPushCancelChoice, selectField.GetValue(), "cancel must remain the default choice")
}

func TestCollectGitMetadataAllowsDirtyWithExplicitFlag(t *testing.T) {
	specPath, _ := pipelineGitRepository(t)
	require.NoError(t, os.WriteFile(specPath, []byte("name: changed\n"), 0o600))
	var progress strings.Builder
	t.Cleanup(redirectProgress(&progress))

	pipeline := NewPushPipeline(context.Background(), PushPipelineConfig{
		SpecPath:   specPath,
		AllowDirty: true,
	}).CollectGitMetadata()

	require.NoError(t, pipeline.Err())
	assert.True(t, pipeline.gitMetadata.WorkingTreeDirty)
	assert.Contains(t, progress.String(), "may not be reproducible")
}

func TestCollectGitMetadataInteractiveChoiceControlsDirtyPush(t *testing.T) {
	specPath, _ := pipelineGitRepository(t)
	require.NoError(t, os.WriteFile(specPath, []byte("name: changed\n"), 0o600))
	originalTerminal := interactiveTerminal
	originalPrompt := confirmDirtyPushPrompt
	interactiveTerminal = func() bool { return true }
	t.Cleanup(func() {
		interactiveTerminal = originalTerminal
		confirmDirtyPushPrompt = originalPrompt
	})

	confirmDirtyPushPrompt = func() (bool, error) { return false, nil }
	stopped := NewPushPipeline(context.Background(), PushPipelineConfig{SpecPath: specPath}).CollectGitMetadata()
	assert.ErrorIs(t, stopped.Err(), tui.ErrCanceled)

	confirmDirtyPushPrompt = func() (bool, error) { return true, nil }
	proceeded := NewPushPipeline(context.Background(), PushPipelineConfig{SpecPath: specPath}).CollectGitMetadata()
	require.NoError(t, proceeded.Err())
	assert.True(t, proceeded.gitMetadata.WorkingTreeDirty)
}

func TestCollectGitMetadataRejectsDirtyNoninteractivePush(t *testing.T) {
	specPath, _ := pipelineGitRepository(t)
	require.NoError(t, os.WriteFile(specPath, []byte("name: changed\n"), 0o600))
	originalTerminal := interactiveTerminal
	interactiveTerminal = func() bool { return false }
	t.Cleanup(func() { interactiveTerminal = originalTerminal })

	pipeline := NewPushPipeline(context.Background(), PushPipelineConfig{SpecPath: specPath}).CollectGitMetadata()

	assert.ErrorIs(t, pipeline.Err(), errDirtyWorkingTree)
}
