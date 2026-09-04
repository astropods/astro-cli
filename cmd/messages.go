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

func errNoEvaluationFile() error {
	return fmt.Errorf("%s not found beside astropods.yml", strings.Join(evaluationFilenameAliases, " or "))
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

func errAccountNotLoggedIn() error {
	return fmt.Errorf("not logged in. Run '%s login' to authenticate", buildinfo.BinaryName)
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

func errAgentCoreOnlyFlags(flags []string) error {
	return fmt.Errorf(
		"cannot honor %s: this deploy goes to Astro, and those flags apply only when the spec sets agent.annotations.runtime: agentcore",
		strings.Join(flags, ", "),
	)
}

func errAgentCoreDeployTakesNoName(name, specName string) error {
	return fmt.Errorf(
		"cannot deploy %q: this spec sets agent.annotations.runtime: agentcore, which deploys %q from the local spec. Drop the name, or pass -f to select another spec",
		name, specName,
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

func errGrantNeedsAdapterOnRedeploy() error {
	return fmt.Errorf("--grant needs --adapter on redeploy: grants alone would reset the deployment's adapters")
}

func msgWorkloadIssueLine(workload, component, phase, message string) string {
	name := workload
	if component != "" && component != workload {
		name = fmt.Sprintf("%s (%s)", workload, component)
	}
	detail := message
	if detail == "" {
		detail = phase
	}
	if detail == "" {
		return name
	}
	return fmt.Sprintf("%s: %s", name, detail)
}

func msgRestartCount(restarts int32) string {
	if restarts == 1 {
		return "1 restart"
	}
	return fmt.Sprintf("%d restarts", restarts)
}

func msgContainerStateLine(container, state string, restarts int32, message string) string {
	parts := []string{container}
	if state != "" {
		parts = append(parts, state)
	}
	if restarts > 0 {
		parts = append(parts, msgRestartCount(restarts))
	}
	line := strings.Join(parts, ", ")
	if message != "" {
		return fmt.Sprintf("%s: %s", line, message)
	}
	return line
}

func msgContainerRestartWarning(container, state string, restarts int32, message string) string {
	line := fmt.Sprintf("! %s is %s", container, strings.ToLower(state))
	if restarts > 0 {
		line = fmt.Sprintf("! %s is %s after %s", container, strings.ToLower(state), msgRestartCount(restarts))
	}
	if message != "" {
		line = fmt.Sprintf("%s: %s", line, message)
	}
	return line + ". The lines below come from the current container."
}

func msgAlertLine(severity, title, state, since string) string {
	line := fmt.Sprintf("%s  %s  %s", severity, title, state)
	if since != "" {
		line = fmt.Sprintf("%s since %s", line, since)
	}
	return line
}

func errInvalidSchedule(raw string) error {
	return fmt.Errorf(`invalid --schedule %q: expected <ingestion>=<cron expression>, e.g. --schedule weekly-sync="0 3 * * *"`, raw)
}

func errDuplicateSchedule(name string) error {
	return fmt.Errorf("--schedule %s was given more than once", name)
}

func errUnknownIngestionSchedule(unknown, available []string) error {
	if len(available) == 0 {
		return fmt.Errorf("this blueprint runs no ingestion on a schedule, so --schedule %s has nothing to set", strings.Join(unknown, ", "))
	}
	return fmt.Errorf("no scheduled ingestion named %s (available: %s)", strings.Join(unknown, ", "), strings.Join(available, ", "))
}
