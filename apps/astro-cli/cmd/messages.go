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
	"fmt"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
)

func errNoSpecFile() error {
	return fmt.Errorf(
		"astropods.yml not found in current directory, run '%s project create' to create a new agent harness or pass -f to specify a path to a valid spec",
		buildinfo.BinaryName,
	)
}

func errAgentTargetRequired() error {
	return fmt.Errorf("expected a deployment name or ID — multi-word names can be unquoted, e.g. ast agent get Pirate Parrot EU")
}

func errDeployNameConflict(displayName string) error {
	return fmt.Errorf(
		"deployment name %q is already in use — choose a different name:\n  %s deploy <blueprint> --name <new-name>",
		displayName, buildinfo.BinaryName,
	)
}

func msgDeployURLNotReady(url, reason string) string {
	if reason != "" {
		return fmt.Sprintf("deployed — Launch URL not ready yet (%s): %s", url, reason)
	}
	return fmt.Sprintf("deployed — Launch URL not ready yet: %s", url)
}

func msgLaunchURLLine(url string) string {
	return fmt.Sprintf("Launch URL:  %s", url)
}

func msgLaunchURLPending(message string) string {
	if message != "" {
		return fmt.Sprintf("URL status:  not ready — %s", message)
	}
	return "URL status:  not ready"
}

func msgLaunchURLReady() string {
	return "URL status:  ready"
}

func errAccountMismatch(specAccount, currentAccount string) error {
	return fmt.Errorf(
		"spec account %q does not match current account %q\n\n"+
			"To push as %s, switch first:\n  %s account switch %s\n\n"+
			"To push under the current account (%s), use --allow-account-override",
		specAccount, currentAccount, specAccount, buildinfo.BinaryName, specAccount, currentAccount,
	)
}
