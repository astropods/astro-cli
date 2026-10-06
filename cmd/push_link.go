package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
)

// pushLink is the user-local record of the account and name `ast push` last
// resolved to for one spec file, so a repeat push recognizes its own prior
// push and skips the confirmation prompt instead of asking again every
// time. It never skips the existence check itself — see
// resolveOrRenameBlueprint.
//
// Stored under the user's own config directory, keyed by the spec file's
// canonical path, never inside the project checkout: a file the repository
// tracks must not stand in for a user's own confirmation, or a cloned repo
// could ship a link that suppresses the prompt for someone who never
// confirmed anything.
type pushLink struct {
	Account string `json:"account"`
	Name    string `json:"name"`
}

// pushLinkPath returns the user-local path recording specPath's last push
// resolution, keyed by specPath's canonical absolute form so invocations
// from different working directories (e.g. --file subdir/astropods.yml run
// from a parent directory) still resolve to the same link, and two spec
// files never collide onto one.
func pushLinkPath(specPath string) (string, error) {
	configDir, err := auth.ConfigDir(buildinfo.BinaryName)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.Abs(specPath)
	if err != nil {
		canonical = specPath
	}
	sum := sha256.Sum256([]byte(canonical))
	return filepath.Join(configDir, "push-links", hex.EncodeToString(sum[:])+".json"), nil
}

// readPushLink returns the stored link for specPath, or nil if there isn't
// one or it can't be read. A missing or malformed file isn't an error worth
// surfacing: the caller just falls back to the normal existence check.
func readPushLink(specPath string) *pushLink {
	path, err := pushLinkPath(specPath)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's own config dir plus a content hash, not external input
	if err != nil {
		return nil
	}
	var link pushLink
	if err := json.Unmarshal(data, &link); err != nil {
		return nil
	}
	return &link
}

// writePushLink records account/name as specPath's push target. Call only
// once the push is actually authorized (after checkBlueprintPushPermission
// succeeds) — writing any earlier would let a later retry, after a
// permission failure, skip straight back to this same rejected target
// without offering the rename path.
//
// Best-effort: a write failure only means the next push re-checks instead
// of trusting the link, not a reason to fail a push that already succeeded.
func writePushLink(specPath, account, name string) {
	path, err := pushLinkPath(specPath)
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(pushLink{Account: account, Name: name}, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
