// Package-level user-facing messages live here so that:
//   - callers never inline error strings (no invisible drift between command output and docs),
//   - tests can do exact-string assertions against the same function instead of
//     fragile substring checks, and
//   - copy changes happen in one place.
//
// Convention: functions that return an error are named errXxx; functions that
// return a plain string are named msgXxx.
package cmd

import (
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
)

var errNoSpecFile = errors.New("astropods.yml not found in current directory, run 'ast project create' to create a new agent harness or pass -f to specify a path to a valid spec")

func errDeployNameConflict(displayName string) error {
	return fmt.Errorf(
		"deployment name %q is already in use — choose a different name:\n  %s deploy <blueprint> --name <new-name>",
		displayName, buildinfo.BinaryName,
	)
}

func errAccountMismatch(specAccount, currentAccount string) error {
	return fmt.Errorf(
		"spec account %q does not match current account %q\n\n"+
			"To push as %s, switch first:\n  %s account switch %s\n\n"+
			"To push under the current account (%s), use --allow-account-override",
		specAccount, currentAccount, specAccount, buildinfo.BinaryName, specAccount, currentAccount,
	)
}
