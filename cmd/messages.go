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
	"time"

	"github.com/astropods/astro-cli/internal/buildinfo"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
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

func msgSelectRegionDescription() string {
	return "Where this agent runs. To move it later, redeploy with --cluster."
}

func errClusterNotAvailable(requested string, available []string) error {
	if len(available) == 0 {
		return fmt.Errorf("cluster %q is not available to this account", requested)
	}
	return fmt.Errorf(
		"cluster %q is not available to this account (available: %s)",
		requested, strings.Join(available, ", "),
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

func errAgentCoreNotServing(hostPort string, wait time.Duration) error {
	return fmt.Errorf(`the agent never bound :%d, so no turn can be delivered

The spec sets agent.annotations.runtime: agentcore, so the agent must serve
POST /invocations and GET /ping on :%d. Nothing answered on localhost:%s
within %s. Containers are still running — check the agent's log first:

  %s project logs agent

Common causes, most likely first:
  1. @astropods/adapter-core in the image is too old to honor ASTRO_RUNTIME.
     It needs 0.9.1 or newer. Check with:
       docker exec <project>-agent-1 grep -m1 version node_modules/@astropods/adapter-core/package.json
  2. A cached build layer installed an older adapter. Rebuild without cache:
       %s project start --rebuild
  3. The agent crashed on boot, which its log will show.`,
		composeBuilder.AgentCorePort, composeBuilder.AgentCorePort, hostPort, wait,
		buildinfo.BinaryName, buildinfo.BinaryName)
}

func msgBillingUnavailable() string {
	return "Billing is not configured for this account"
}

func errBillingUnavailable() error {
	return fmt.Errorf("billing is not configured for this account")
}

func msgNoInvoices() string {
	return "No invoices yet"
}

func errBillingSetConflict(name string) error {
	return fmt.Errorf("--%s and --clear-%s cannot be used together", name, name)
}

func errBillingSetNoChange() error {
	return fmt.Errorf("specify --warning, --limit, --clear-warning, or --clear-limit")
}

func msgSpendControlsSaved() string {
	return "Spend controls saved"
}

func errUnknownUsageMetric(metric string) error {
	return fmt.Errorf("--metric %q is not a metered quantity; use compute or gateway", metric)
}

func msgUsageControlsSaved(metric string) string {
	return fmt.Sprintf("Usage controls saved for %s", metric)
}
