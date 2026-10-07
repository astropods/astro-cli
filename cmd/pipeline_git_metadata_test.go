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

	spec "github.com/astropods/astro-spec"
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

func TestCollectGitMetadataWarnsWhenProvenanceIsIncomplete(t *testing.T) {
	root := t.TempDir()
	specRepo := filepath.Join(root, "blueprint")
	contextRepo := filepath.Join(root, "shared")
	require.NoError(t, os.MkdirAll(specRepo, 0o755))
	require.NoError(t, os.MkdirAll(contextRepo, 0o755))

	for _, dir := range []string{specRepo, contextRepo} {
		pipelineGitCommand(t, dir, "init")
		pipelineGitCommand(t, dir, "config", "user.email", "test@example.com")
		pipelineGitCommand(t, dir, "config", "user.name", "Astro Test")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("initial\n"), 0o600))
		pipelineGitCommand(t, dir, "add", ".")
		pipelineGitCommand(t, dir, "commit", "-m", "feat: initial")
	}
	specPath := filepath.Join(specRepo, "astropods.yml")
	require.NoError(t, os.WriteFile(specPath, []byte("name: example\n"), 0o600))
	pipelineGitCommand(t, specRepo, "add", "astropods.yml")
	pipelineGitCommand(t, specRepo, "commit", "-m", "feat: add blueprint")

	var progress strings.Builder
	t.Cleanup(redirectProgress(&progress))
	pipeline := NewPushPipeline(context.Background(), PushPipelineConfig{SpecPath: specPath})
	pipeline.components = []spec.Component{{Build: &spec.BuildConfig{Context: "../shared"}}}
	pipeline.CollectGitMetadata()

	require.NoError(t, pipeline.Err())
	assert.NotEmpty(t, pipeline.gitMetadata.CommitSHA)
	assert.False(t, pipeline.gitMetadata.WorkingTreeStatusKnown)
	assert.Contains(t, stripANSI(progress.String()), "some files used by this blueprint may not be represented by the recorded commit")
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

func TestCollectGitMetadataRejectsDirtyInteractivePush(t *testing.T) {
	specPath, _ := pipelineGitRepository(t)
	require.NoError(t, os.WriteFile(specPath, []byte("name: changed\n"), 0o600))
	originalTerminal := interactiveTerminal
	interactiveTerminal = func() bool { return true }
	t.Cleanup(func() { interactiveTerminal = originalTerminal })

	pipeline := NewPushPipeline(context.Background(), PushPipelineConfig{SpecPath: specPath}).CollectGitMetadata()

	assert.ErrorIs(t, pipeline.Err(), errDirtyWorkingTree)
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

func TestCollectGitMetadataSkipsNoBuildPushes(t *testing.T) {
	specPath, _ := pipelineGitRepository(t)
	require.NoError(t, os.WriteFile(specPath, []byte("name: changed\n"), 0o600))
	reachedNextStep := false
	pipeline := NewPushPipeline(context.Background(), PushPipelineConfig{
		SpecPath:   specPath,
		SkipBuild:  true,
		AllowDirty: false,
	}).
		CollectGitMetadata().
		step(func() error {
			reachedNextStep = true
			return nil
		})

	require.NoError(t, pipeline.Err())
	assert.True(t, reachedNextStep)
	assert.Empty(t, pipeline.gitMetadata.CommitSHA)
	assert.Empty(t, pipeline.gitMetadata.CommitMessage)
	assert.False(t, pipeline.gitMetadata.WorkingTreeDirty)
	assert.False(t, pipeline.gitMetadata.WorkingTreeStatusKnown)
}
