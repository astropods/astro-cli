package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/moby/moby/client"

	gitmetadata "github.com/astropods/astro-cli/internal/git"
	spec "github.com/astropods/astro-spec"
)

const (
	labelCommitSHA     = "org.opencontainers.image.revision"
	labelCommitMessage = "io.astropods.git.commit-message"
	labelDirty         = "io.astropods.git.working-tree-dirty"
	labelBuildID       = "io.astropods.build-id"
	labelBlueprintName = "io.astropods.blueprint-name"
	labelBuildPlatform = "io.astropods.build-platform"
)

func validBuildID(value string) bool {
	if len(value) != 8 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func provenanceLabels(buildID, blueprintName, platform string, metadata gitmetadata.Metadata) map[string]string {
	dirty := "unknown"
	if metadata.WorkingTreeDirty {
		dirty = "true"
	} else if metadata.WorkingTreeStatusKnown {
		dirty = strconv.FormatBool(metadata.WorkingTreeDirty)
	}
	return map[string]string{
		labelBuildID:       buildID,
		labelBlueprintName: blueprintName,
		labelBuildPlatform: platform,
		labelCommitSHA:     metadata.CommitSHA,
		labelCommitMessage: metadata.CommitMessage,
		labelDirty:         dirty,
	}
}

type imageInspector interface {
	ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error)
}

func inspectReusableBuild(ctx context.Context, inspector imageInspector, components []spec.Component, blueprintName, buildID, platform string) (gitmetadata.Metadata, error) {
	var expected gitmetadata.Metadata
	for index, component := range components {
		imageName := platformImageTag(component.ImageName, buildID, platform)
		result, err := inspector.ImageInspect(ctx, imageName)
		if err != nil {
			return gitmetadata.Metadata{}, fmt.Errorf("reusable build %s is missing image %s: %w", buildID, imageName, err)
		}
		if result.Config == nil {
			return gitmetadata.Metadata{}, fmt.Errorf("reusable build %s image %s has no image configuration", buildID, imageName)
		}
		metadata, err := provenanceFromLabels(result.Config.Labels, blueprintName, buildID, platform)
		if err != nil {
			return gitmetadata.Metadata{}, fmt.Errorf("reusable build %s image %s: %w", buildID, imageName, err)
		}
		if index == 0 {
			expected = metadata
			continue
		}
		if metadata != expected {
			return gitmetadata.Metadata{}, fmt.Errorf("reusable build %s images have inconsistent Git provenance", buildID)
		}
	}
	return expected, nil
}

func provenanceFromLabels(labels map[string]string, blueprintName, buildID, platform string) (gitmetadata.Metadata, error) {
	if labels[labelBuildID] != buildID {
		return gitmetadata.Metadata{}, fmt.Errorf("expected build ID label %q", buildID)
	}
	if labels[labelBlueprintName] != blueprintName {
		return gitmetadata.Metadata{}, fmt.Errorf("was built for blueprint %q, not %q", labels[labelBlueprintName], blueprintName)
	}
	if labels[labelBuildPlatform] != platform {
		return gitmetadata.Metadata{}, fmt.Errorf("was built for platform %q, not %q", labels[labelBuildPlatform], platform)
	}

	metadata := gitmetadata.Metadata{
		CommitSHA:     labels[labelCommitSHA],
		CommitMessage: labels[labelCommitMessage],
	}
	switch labels[labelDirty] {
	case "true":
		metadata.WorkingTreeDirty = true
		metadata.WorkingTreeStatusKnown = true
	case "false":
		metadata.WorkingTreeStatusKnown = true
	case "unknown":
	default:
		return gitmetadata.Metadata{}, fmt.Errorf("has invalid or missing %s label", labelDirty)
	}
	return metadata, nil
}
