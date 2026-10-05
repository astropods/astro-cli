package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitmetadata "github.com/astropods/astro-cli/internal/git"
	spec "github.com/astropods/astro-spec"
)

type fakeImageInspector struct {
	labels map[string]map[string]string
}

func (f fakeImageInspector) ImageInspect(_ context.Context, name string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	labels, ok := f.labels[name]
	if !ok {
		return client.ImageInspectResult{}, errors.New("not found")
	}
	config := &dockerspec.DockerOCIImageConfig{}
	config.Labels = labels
	return client.ImageInspectResult{InspectResponse: image.InspectResponse{Config: config}}, nil
}

func reusableComponents() []spec.Component {
	return []spec.Component{
		{Kind: spec.ComponentAgent, ImageName: "example", Build: &spec.BuildConfig{Context: "."}},
		{Kind: spec.ComponentIntegration, Name: "search", ImageName: "example-integration-search", Build: &spec.BuildConfig{Context: "."}},
	}
}

func TestInspectReusableBuildReadsConsistentProvenance(t *testing.T) {
	metadata := gitmetadata.Metadata{
		CommitSHA:              "0123456789abcdef0123456789abcdef01234567",
		WorkingTreeDirty:       true,
		WorkingTreeStatusKnown: true,
	}
	labels := provenanceLabels("deadbeef", "example", "linux/amd64", metadata)
	inspector := fakeImageInspector{labels: map[string]map[string]string{
		"example-linux-amd64:deadbeef":                    labels,
		"example-integration-search-linux-amd64:deadbeef": labels,
	}}

	got, err := inspectReusableBuild(context.Background(), inspector, reusableComponents(), "example", "deadbeef", "linux/amd64")

	require.NoError(t, err)
	assert.Equal(t, metadata, got)
	assert.NotContains(t, labels, "io.astropods.git.commit-message")
}

func TestInspectReusableBuildRejectsMissingAndMismatchedImages(t *testing.T) {
	components := reusableComponents()
	labels := provenanceLabels("deadbeef", "example", "linux/amd64", gitmetadata.Metadata{WorkingTreeStatusKnown: true})

	_, err := inspectReusableBuild(context.Background(), fakeImageInspector{labels: map[string]map[string]string{
		"example-linux-amd64:deadbeef": labels,
	}}, components, "example", "deadbeef", "linux/amd64")
	assert.ErrorContains(t, err, "missing image")

	other := provenanceLabels("deadbeef", "example", "linux/amd64", gitmetadata.Metadata{
		CommitSHA:              "ffffffffffffffffffffffffffffffffffffffff",
		WorkingTreeStatusKnown: true,
	})
	_, err = inspectReusableBuild(context.Background(), fakeImageInspector{labels: map[string]map[string]string{
		"example-linux-amd64:deadbeef":                    labels,
		"example-integration-search-linux-amd64:deadbeef": other,
	}}, components, "example", "deadbeef", "linux/amd64")
	assert.ErrorContains(t, err, "inconsistent Git provenance")
}

func TestProvenanceFromLabelsRejectsWrongBuildIdentity(t *testing.T) {
	labels := provenanceLabels("deadbeef", "example", "linux/amd64", gitmetadata.Metadata{})

	_, err := provenanceFromLabels(labels, "other", "deadbeef", "linux/amd64")
	assert.ErrorContains(t, err, "not \"other\"")

	_, err = provenanceFromLabels(labels, "example", "deadbeef", "linux/arm64")
	assert.ErrorContains(t, err, "linux/arm64")
}

func TestProvenanceLabelsPreserveProvenDirtyStateWhenAnotherStatusCheckFailed(t *testing.T) {
	labels := provenanceLabels("deadbeef", "example", "linux/amd64", gitmetadata.Metadata{WorkingTreeDirty: true})
	assert.Equal(t, "true", labels[labelDirty])

	labels = provenanceLabels("deadbeef", "example", "linux/amd64", gitmetadata.Metadata{})
	assert.Equal(t, "unknown", labels[labelDirty])
}

func TestHydrateReusableBuildCommitMessageUsesRecordedSHA(t *testing.T) {
	specPath, sha := pipelineGitRepository(t)
	metadata := gitmetadata.Metadata{
		CommitSHA:              strings.TrimSpace(sha),
		WorkingTreeStatusKnown: true,
	}

	got := hydrateReusableBuildCommitMessage(filepath.Dir(specPath), metadata)
	assert.Equal(t, "feat: add example blueprint", got.CommitMessage)

	metadata.CommitSHA = strings.Repeat("f", 40)
	got = hydrateReusableBuildCommitMessage(filepath.Dir(specPath), metadata)
	assert.Empty(t, got.CommitMessage)
}

func TestValidBuildID(t *testing.T) {
	assert.True(t, validBuildID("deadbeef"))
	assert.True(t, validBuildID("DEADBEEF"))
	assert.False(t, validBuildID("not-an-id"))
	assert.False(t, validBuildID("abc"))
}

func TestValidateNoBuildOptions(t *testing.T) {
	tests := []struct {
		name                string
		noBuild             bool
		buildID             string
		buildableComponents int
		wantErr             string
	}{
		{name: "normal build", buildableComponents: 1},
		{name: "reusable build", noBuild: true, buildID: "deadbeef", buildableComponents: 1},
		{name: "image only", noBuild: true},
		{name: "id requires no build", buildID: "deadbeef", buildableComponents: 1, wantErr: "--build-id requires --no-build"},
		{name: "invalid id", noBuild: true, buildID: "invalid!", buildableComponents: 1, wantErr: "--build-id must be an eight-character hexadecimal ID produced by 'ast build'"},
		{name: "build blocks require id", noBuild: true, buildableComponents: 1, wantErr: "--build-id is required with --no-build when the blueprint contains build blocks; run 'ast build' first"},
		{name: "image only rejects id", noBuild: true, buildID: "deadbeef", wantErr: "--build-id cannot be used because this blueprint has no build blocks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoBuildOptions(tt.noBuild, tt.buildID, tt.buildableComponents)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestBlueprintBuildJSONReportsReusableBuildAndCapturesDirtyProvenance(t *testing.T) {
	dir := t.TempDir()
	pipelineGitCommand(t, dir, "init")
	pipelineGitCommand(t, dir, "config", "user.email", "test@example.com")
	pipelineGitCommand(t, dir, "config", "user.name", "Astro Test")
	specPath := filepath.Join(dir, "astropods.yml")
	require.NoError(t, os.WriteFile(specPath, []byte(`spec: blueprint/v1
name: reusable-agent
meta: {}
agent:
  build:
    context: .
    dockerfile: Dockerfile
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o600))
	pipelineGitCommand(t, dir, "add", ".")
	pipelineGitCommand(t, dir, "commit", "-m", "feat: reusable image")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n# dirty\n"), 0o600))

	originalRunner := runBlueprintBuildImages
	var captured gitmetadata.Metadata
	runBlueprintBuildImages = func(_ context.Context, _ string, _ string, buildID string, _ []string, _, _, quiet bool, metadata gitmetadata.Metadata) error {
		assert.True(t, validBuildID(buildID))
		assert.True(t, quiet)
		captured = metadata
		return nil
	}
	t.Cleanup(func() { runBlueprintBuildImages = originalRunner })

	cmd := &cobra.Command{Use: "build"}
	cmd.Flags().StringP("file", "f", "", "")
	cmd.Flags().Bool("json", false, "")
	require.NoError(t, cmd.Flags().Set("file", specPath))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetContext(context.Background())

	require.NoError(t, runBlueprintBuild(cmd, nil))
	var result blueprintBuildResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, "reusable-agent", result.Name)
	assert.True(t, validBuildID(result.BuildID))
	assert.True(t, captured.WorkingTreeDirty)
	assert.True(t, captured.WorkingTreeStatusKnown)
	assert.Len(t, captured.CommitSHA, 40)
	assert.Equal(t, "feat: reusable image", captured.CommitMessage)
	assert.Contains(t, stripANSI(stderr.String()), "Building with uncommitted blueprint changes")
}
