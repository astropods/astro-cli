package compose

// User-facing messages live here so that callers never inline copy, tests can
// assert exact strings against the same function instead of substrings, and a
// wording change happens in one place. Same convention as cmd/messages.go:
// functions returning a plain string are named msgXxx.
//
// These carry the copy alone. warnf owns the ⚠ prefix and the newline.

import (
	"fmt"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

func msgWatchDirDuplicate(dir string) string {
	return fmt.Sprintf("dev.watch names %s more than once, mounting it once", dir)
}

func msgWatchDirRejected(dir, reason string) string {
	return fmt.Sprintf("dev.watch entry %q cannot be mounted: %s", dir, reason)
}

func msgWatchDirMissing(dir string) string {
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
