package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

// pushLink is the project-local record of the account and name `ast push`
// last resolved to from this directory, stored at .ast/push.json so a
// repeat push recognizes its own prior push and skips the existence check
// and confirmation prompt instead of asking again every time.
type pushLink struct {
	Account string `json:"account"`
	Name    string `json:"name"`
}

func pushLinkPath() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(workingDir, buildinfo.AppDirName, "push.json"), nil
}

// readPushLink returns the stored link, or nil if there isn't one or it
// can't be read. A missing or malformed file isn't an error worth
// surfacing: the caller just falls back to the normal existence check.
func readPushLink() *pushLink {
	path, err := pushLinkPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from os.Getwd() + hardcoded suffix
	if err != nil {
		return nil
	}
	var link pushLink
	if err := json.Unmarshal(data, &link); err != nil {
		return nil
	}
	return &link
}

// writePushLink records account/name as this directory's push target.
// Best-effort: a write failure only means the next push re-checks instead
// of trusting the link, not a reason to fail the push that just succeeded.
func writePushLink(account, name string) {
	path, err := pushLinkPath()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(pushLink{Account: account, Name: name}, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
