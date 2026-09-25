package compose

// User-facing messages live here so that callers never inline copy, tests can
// assert exact strings against the same function instead of substrings, and a
// wording change happens in one place. Same convention as cmd/messages.go:
// functions returning a plain string are named msgXxx.
//
// The dev.watch three are exported because 'spec validate' reports the same
// three conditions from package cmd. One condition keeps one wording; only the
// marker differs, and that belongs to whoever prints it. warnf owns the ⚠ and
// the newline here; validate supplies its own ✗ and source snippet.

import (
	"fmt"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

func MsgWatchDirDuplicate(dir string) string {
	return fmt.Sprintf("dev.watch names %s more than once, mounting it once", dir)
}

func MsgWatchDirRejected(dir, reason string) string {
	return fmt.Sprintf("dev.watch entry %q cannot be mounted: %s", dir, reason)
}

func MsgWatchDirMissing(dir string) string {
	return fmt.Sprintf(
		"dev.watch names %s, which is not a directory in this project. Edits under it will not reload",
		dir,
	)
}

func msgSlackTokenMissing() string {
	return fmt.Sprintf(
		"Slack adapter listed but SLACK_BOT_TOKEN not set, skipping (run '%s project configure' to add it)",
		buildinfo.BinaryName,
	)
}
