package git

import (
	"os/exec"
	"strings"
)

// Metadata describes the Git state used for a local blueprint build.
type Metadata struct {
	CommitSHA        string
	CommitMessage    string
	WorkingTreeDirty bool
}

type commandRunner func(dir string, args ...string) (string, error)

// CollectMetadata reads the current commit and worktree state for dir. Each
// lookup is independent and best-effort: ast push must still work without Git,
// outside a repository, or in a repository without a first commit.
func CollectMetadata(dir string) Metadata {
	return collectMetadata(dir, runGit)
}

func collectMetadata(dir string, run commandRunner) Metadata {
	var metadata Metadata

	if output, err := run(dir, "show", "-s", "--format=%H%x00%B", "HEAD"); err == nil {
		parts := strings.SplitN(output, "\x00", 2)
		metadata.CommitSHA = strings.TrimSpace(parts[0])
		if len(parts) == 2 {
			metadata.CommitMessage = strings.TrimRight(parts[1], "\r\n")
		}
	}

	if output, err := run(dir, "status", "--porcelain=v1", "--untracked-files=normal"); err == nil {
		metadata.WorkingTreeDirty = strings.TrimSpace(output) != ""
	}

	return metadata
}

func runGit(dir string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", commandArgs...).Output() //nolint:gosec
	return string(output), err
}
