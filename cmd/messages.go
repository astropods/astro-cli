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
	"strings"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

func errNoSpecFile() error {
	return fmt.Errorf(
		"astropods.yml not found in current directory, run '%s project create' to create a new agent harness or pass -f to specify a path to a valid spec",
		buildinfo.BinaryName,
	)
}

func errAgentTargetRequired() error {
	return fmt.Errorf(
		"required: --name <display-or-blueprint-name> or --id <deployment-id> (from %s agent list; IDs only with --id)",
		buildinfo.BinaryName,
	)
}

func errAgentUnexpectedArgument(arg string) error {
	return fmt.Errorf("unexpected argument %q — use --name or --id", arg)
}

func errAgentDeploymentNotFoundForID(id string) error {
	return fmt.Errorf("no deployment found for ID %q", id)
}

func errAgentDeploymentNotFound(target string) error {
	return fmt.Errorf("no deployment found for %q", target)
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

func errNoAgentWorkload(available []string) error {
	return fmt.Errorf("no agent workload found — pass --workload to pick another one (available: %s)", strings.Join(available, ", "))
}

func errWorkloadNotFound(requested string, available []string) error {
	return fmt.Errorf("no workload matches %q (available: %s)", requested, strings.Join(available, ", "))
}

func errWorkloadAmbiguous(requested string, matches []string) error {
	return fmt.Errorf("%q is ambiguous; pass the full workload name (matches: %s)", requested, strings.Join(matches, ", "))
}

func errContainerNotInWorkload(container, workload string, available []string) error {
	return fmt.Errorf("container %q not found in workload %q (available: %s)", container, workload, strings.Join(available, ", "))
}

func errNonNegativeIntFlag(name string) error {
	return fmt.Errorf("--%s must be zero or greater", name)
}

func errPositiveIntFlag(name string) error {
	return fmt.Errorf("--%s must be greater than zero", name)
}

func errRFC3339TimeFlag(name, value string) error {
	return fmt.Errorf("--%s %q is not a valid RFC3339 timestamp", name, value)
}

func errTraceStartAfterEnd() error {
	return fmt.Errorf("--start must be before --end")
}

func errAgentTraceNotFound(traceID, target string) error {
	return fmt.Errorf("no trace %q found for %q", traceID, target)
}

func msgNoTracesForAgent(target string) string {
	return fmt.Sprintf("No traces found for %s", target)
}

func errAgentTraceSummaryWithTraceID() error {
	return fmt.Errorf("--summary cannot be used with --trace-id")
}

func msgNoObsSummaryForAgent(target string) string {
	return fmt.Sprintf("No activity summary for %s yet (updates about every 10m)", target)
}

func msgLoginPriorAccountUnavailable(account string) string {
	return fmt.Sprintf("  Note: previous account %q is no longer available; using personal account.\n", account)
}

func errLoginAccountsLoadEmpty() error {
	return fmt.Errorf(
		"could not load your accounts from the server (empty response). Try again in a moment",
	)
}

func errAgentCoreRejected(name, reason string) error {
	return fmt.Errorf("cannot deploy %q to AgentCore Runtime: %s", name, reason)
}

func errAgentCoreMissingSecrets(names []string) error {
	return fmt.Errorf(
		"missing secret value(s) %s: supply them with --secret NAME=VALUE or --secrets-file",
		strings.Join(names, ", "),
	)
}

func errAgentCoreMissingImage() error {
	return fmt.Errorf("an agentcore deploy needs --image <ecr-uri> (use --dry-run to preview without it)")
}

func errAgentCoreMissingExecRole() error {
	return fmt.Errorf(
		"an agentcore deploy needs %s set to the execution role ARN (use --dry-run to preview without it)",
		execRoleEnv,
	)
}

func errAgentCoreInvalidSecret(pair string) error {
	return fmt.Errorf("invalid --secret %q: expected NAME=VALUE", pair)
}

func errAgentCoreSecretsFileLine(line int) error {
	return fmt.Errorf("secrets-file line %d: expected KEY=VALUE", line)
}
