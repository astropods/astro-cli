package compose

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	spec "github.com/astropods/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
)

// WatchDirPlan is what hot reload mounts, and what it was asked to mount and
// could not find.
type WatchDirPlan struct {
	// Mount holds the project-relative directories that exist on disk,
	// slash-separated whatever the host (path.Clean, not filepath.Clean),
	// because one of these becomes a container path and a container path is
	// always POSIX.
	Mount []string
	// Missing holds directories a spec named explicitly that are not on disk.
	// A spec that names none is never reported, since the agent/ default is
	// assumed rather than asked for.
	Missing []string
	// Rejected holds entries refused before they were looked for, with why.
	Rejected []WatchDirRejection
	// Duplicate holds entries naming a directory an earlier entry already
	// named, such as agent and ./agent. Two mounts on one container path make
	// Docker refuse to create the container.
	Duplicate []string
}

// WatchDirRejection is a dev.watch entry that cannot be mounted, and why.
type WatchDirRejection struct {
	Dir    string
	Reason string
}

// subdir reports whether target is a path strictly inside root. Rel encodes
// "outside" as a leading .., which IsLocal tests properly, including a ..
// buried mid-path. A rel of "." means target is root itself, which must not be
// mounted. A Rel error means the two share no base, as across Windows drive
// letters, so it reads as not contained.
func subdir(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

// invalidWatchEntry reports why entry cannot name a watch directory, or "" if
// it can. Forward slash is the only separator, and every check is on the string
// alone.
//
// Deliberately not filepath's predicates: a spec is shared across hosts, and
// filepath.IsLocal answers differently per host, cutting on a backslash under
// Windows while treating one as an ordinary filename character under Unix. A
// spec that validates on a laptop has to validate on CI.
func invalidWatchEntry(entry string) string {
	switch {
	case entry == "":
		return "an empty entry names no directory"
	case strings.Contains(entry, `\`):
		return `a backslash is not a path separator here, use /`
	case strings.Contains(entry, ":"):
		return "a drive letter cannot be mounted"
	case strings.HasPrefix(entry, "/"):
		return "an absolute path would mount something outside the project"
	case entry == "." || entry == "./":
		return "the project root would hide everything the image built"
	}

	// A trailing slash names the same directory, so it is the only empty
	// component allowed.
	for _, part := range strings.Split(strings.TrimSuffix(entry, "/"), "/") {
		switch part {
		case "":
			return "an empty path component names no directory"
		case ".":
			return "a . path component is not allowed, name the directory directly"
		case "..":
			return "the path escapes the project"
		}
	}
	return ""
}

// statDir is the stat PlanWatchDirs uses. A test replaces it to exercise a
// stat failure, which chmod cannot force for a process running as root.
var statDir = os.Stat

// PlanWatchDirs resolves dev.watch against the project on disk in one pass.
//
// Each entry is checked as a string by invalidWatchEntry, then cleaned, which
// fixes both its container path and its identity for deduplication, then
// resolved through any symlinks and checked once against the filesystem: it has
// to exist, be a directory, and sit inside the project. Anything else is
// reported rather than mounted.
//
// The project root is resolved once, before the loop. A temp directory is
// commonly reached through a symlinked prefix, /var to /private/var on macOS
// among them, so comparing a resolved entry against an unresolved root would
// reject every directory under one.
func PlanWatchDirs(dev *spec.Dev, workingDir string) (WatchDirPlan, error) {
	var plan WatchDirPlan
	named := dev != nil && len(dev.Watch) > 0
	seen := map[string]bool{}

	root := workingDir
	if resolved, err := filepath.EvalSymlinks(workingDir); err == nil {
		root = resolved
	}

	for _, entry := range dev.WatchDirs() {
		reject := func(reason string) {
			if named {
				plan.Rejected = append(plan.Rejected, WatchDirRejection{Dir: entry, Reason: reason})
			}
		}
		miss := func() {
			if named {
				plan.Missing = append(plan.Missing, entry)
			}
		}

		// Passing invalidWatchEntry leaves only slash-separated relative
		// paths, which is why path.Clean does the cleaning here: it means the
		// same thing on every host, where filepath.Clean would emit
		// backslashes under Windows.
		reason := invalidWatchEntry(entry)
		dir := entry
		if reason == "" {
			dir = path.Clean(entry)
		}

		// Marked on first sight rather than on a successful mount, so a repeat
		// of a missing or rejected entry is reported once as a duplicate
		// instead of warning twice. An entry with no cleaned form dedups on the
		// spelling the spec used.
		if seen[dir] {
			if named {
				plan.Duplicate = append(plan.Duplicate, entry)
			}
			continue
		}
		seen[dir] = true

		if reason != "" {
			reject(reason)
			continue
		}

		// Resolution answers existence too: EvalSymlinks fails for a path that
		// is not there. Any other failure is the entry's problem rather than
		// the project's, so it is reported like every other unmountable entry
		// instead of abandoning the remaining ones.
		real, err := filepath.EvalSymlinks(filepath.Join(workingDir, dir))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				miss()
			} else {
				reject(fmt.Sprintf("the path cannot be resolved: %v", err))
			}
			continue
		}

		fi, err := statDir(real)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			miss()
			continue
		case err != nil:
			return WatchDirPlan{}, fmt.Errorf("reading watch directory %s: %w", dir, err)
		case !fi.IsDir():
			reject("only a directory can be mounted")
			continue
		case real == root:
			reject("the project root would hide everything the image built")
			continue
		case !subdir(root, real):
			reject("the path resolves outside the project")
			continue
		}

		plan.Mount = append(plan.Mount, dir)
	}
	return plan, nil
}

// PrintWarnings reports every entry the plan could not mount. Nothing is
// dropped in silence: a missing, rejected or duplicated entry each says so.
func (p WatchDirPlan) PrintWarnings(w io.Writer) {
	for _, dir := range p.Duplicate {
		warnf(w, "%s", msgWatchDirDuplicate(dir))
	}
	for _, r := range p.Rejected {
		warnf(w, "%s", msgWatchDirRejected(r.Dir, r.Reason))
	}
	for _, dir := range p.Missing {
		warnf(w, "%s", msgWatchDirMissing(dir))
	}
}

// BindMounts is the mount for each planned directory, in plan order.
//
// The source is joined with filepath, which is native to the host, while the
// target is joined with path, because a container path is POSIX wherever the
// build runs.
func (p WatchDirPlan) BindMounts(workingDir string) []types.ServiceVolumeConfig {
	mounts := make([]types.ServiceVolumeConfig, 0, len(p.Mount))
	for _, dir := range p.Mount {
		mounts = append(mounts, types.ServiceVolumeConfig{
			Type:   types.VolumeTypeBind,
			Source: filepath.Join(workingDir, dir),
			Target: path.Join(agentWorkdir, dir),
		})
	}
	return mounts
}
