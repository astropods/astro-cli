package git

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const MaxCommitMessageBytes = 8 << 10

// Metadata describes the Git state used for a local blueprint build.
type Metadata struct {
	CommitSHA              string
	CommitMessage          string
	WorkingTreeDirty       bool
	WorkingTreeStatusKnown bool
}

type commandRunner func(dir string, args ...string) (string, error)

// CollectMetadata reads the current commit for dir and checks only the supplied
// paths for changes. Paths may span repositories; the commit still describes
// the repository containing dir, while dirtiness is the union of every
// successfully discovered relevant repository.
func CollectMetadata(dir string, paths ...string) Metadata {
	return collectMetadata(dir, paths, runGit)
}

func collectMetadata(dir string, paths []string, run commandRunner) Metadata {
	var metadata Metadata

	if output, err := run(dir, "show", "-s", "--format=%H%x00%B", "HEAD"); err == nil {
		parts := strings.SplitN(output, "\x00", 2)
		metadata.CommitSHA = strings.TrimSpace(parts[0])
		if len(parts) == 2 {
			metadata.CommitMessage = truncateUTF8(strings.TrimRight(parts[1], "\r\n"), MaxCommitMessageBytes)
		}
	}

	if len(paths) == 0 {
		paths = []string{dir}
	}
	groups, complete := statusGroups(paths, run)
	metadata.WorkingTreeStatusKnown = complete && len(groups) > 0
	for _, root := range sortedKeys(groups) {
		args := []string{"--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=normal", "--"}
		args = append(args, groups[root]...)
		output, err := run(root, args...)
		if err != nil {
			metadata.WorkingTreeStatusKnown = false
			continue
		}
		if strings.TrimSpace(output) != "" {
			metadata.WorkingTreeDirty = true
		}
	}

	return metadata
}

func statusGroups(paths []string, run commandRunner) (map[string][]string, bool) {
	groups := make(map[string][]string)
	complete := true
	seen := make(map[string]map[string]struct{})

	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			complete = false
			continue
		}
		if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = resolved
		}
		rootOutput, err := run(absolute, "rev-parse", "--show-toplevel")
		if err != nil {
			complete = false
			continue
		}
		root := filepath.Clean(strings.TrimSpace(rootOutput))
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			complete = false
			continue
		}
		if relative == "" {
			relative = "."
		}
		if seen[root] == nil {
			seen[root] = make(map[string]struct{})
		}
		if _, ok := seen[root][relative]; ok {
			continue
		}
		seen[root][relative] = struct{}{}
		groups[root] = append(groups[root], relative)
	}

	for root := range groups {
		sort.Strings(groups[root])
	}
	return groups, complete
}

func sortedKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func runGit(dir string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", commandArgs...).Output() //nolint:gosec
	return string(output), err
}
