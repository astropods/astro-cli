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
	"net/http"
	"strings"
	"time"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/claudesettings"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
)

func errAIGatewayRequiresLogin(err error) error {
	return fmt.Errorf("AI Gateway requires login: %w", err)
}

func errAIGatewayNotEnabled() error {
	return fmt.Errorf(
		"AI Gateway is not enabled in this environment; agents with agent.astro_ai_gateway: true can't run locally here",
	)
}

func msgAIGatewayKeyMinted(expiresAt string) string {
	return fmt.Sprintf("AI Gateway development key minted (expires %s)", expiresAt)
}

func errSandboxRequiresLogin(err error) error {
	return fmt.Errorf("a sandbox requires login: %w", err)
}

func errSandboxNeedsAgentName() error {
	return fmt.Errorf("a sandbox needs the agent's name, and the spec did not supply one")
}

func errSandboxNotEnabled(account string) error {
	return fmt.Errorf(
		"sandboxes are not enabled for %s, or no cluster there runs them",
		account,
	)
}

func errSandboxSessionFailed(err error) error {
	return fmt.Errorf("could not open a sandbox session: %w", err)
}

func msgSandboxSessionOpened(expiresAt string) string {
	return fmt.Sprintf("Sandbox session opened (expires %s)", expiresAt)
}

func msgConnectionsDevSessionOpened(expiresAt string) string {
	return fmt.Sprintf("Dev session opened for connections (expires %s)", expiresAt)
}

func msgConnectionsDevSessionFailed(err error) string {
	return fmt.Sprintf("Connections won't work in this session, because a dev session could not be opened: %v", err)
}

func msgSandboxSessionCloseFailed(err error) string {
	return fmt.Sprintf("Could not close the sandbox session: %v", err)
}

func errEnvFileNotFound(path string) error {
	return fmt.Errorf("env file %s not found; pass --env-file with the path to an existing file", path)
}

func errEnvFileUnreadable(path string, err error) error {
	return fmt.Errorf("could not read env file %s: %w", path, err)
}

func msgDevEnvFileLoaded(count int, path string) string {
	return fmt.Sprintf("%s→%s Environment: %d variable(s) from %s\n", colorCyan, colorReset, count, path)
}

func msgDevEnvStoreLoaded(count int) string {
	return fmt.Sprintf("%s→%s Config: %d variable(s) from project store\n", colorCyan, colorReset, count)
}

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

type authError struct {
	msg   string
	cause error
}

func (e *authError) Error() string { return e.msg }

func (e *authError) Unwrap() error { return e.cause }

func msgReauthenticate() string {
	return fmt.Sprintf("Run '%s login' to re-authenticate.", buildinfo.BinaryName)
}

func errAuthSessionEnded(cause error) error {
	return &authError{
		msg:   "authentication failed: your session has ended. " + msgReauthenticate(),
		cause: cause,
	}
}

func errAuthNoRefreshToken(cause error) error {
	return &authError{
		msg:   "authentication failed: your session can't be renewed. " + msgReauthenticate(),
		cause: cause,
	}
}

func errAuthFailed(cause error) error {
	return &authError{
		msg:   fmt.Sprintf("authentication failed: %s. %s", strings.TrimRight(cause.Error(), ". "), msgReauthenticate()),
		cause: cause,
	}
}

func errAccountMismatch(specAccount, currentAccount string) error {
	return fmt.Errorf(
		"spec account %q does not match current account %q\n\n"+
			"To push as %s, switch first:\n  %s account switch %s\n\n"+
			"To push under the current account (%s), use --allow-account-override",
		specAccount, currentAccount, specAccount, buildinfo.BinaryName, specAccount, currentAccount,
	)
}

func errBlueprintPushPermissionCheck(account, name string, cause error) error {
	return fmt.Errorf("could not check permission to push Blueprint %q to account %q: %w", name, account, cause)
}

func errBlueprintPushPermissionVerdict(account, name string, status int) error {
	return fmt.Errorf(
		"could not check permission to push Blueprint %q to account %q: server returned unexpected status %d",
		name, account, status,
	)
}

func msgBlueprintRenamedUpdateSpec(from, to string) string {
	return fmt.Sprintf("%s→%s pushed as %q instead of %q; update astropods.yml's name field to keep it that way next time", colorCyan, colorReset, to, from)
}

func msgBlueprintExistenceCheckInconclusive(name, account string, cause error) string {
	return fmt.Sprintf("could not confirm whether %q already exists in %q, treating it as if it does: %v", name, account, cause)
}

func errBlueprintExistenceCheckUnexpectedStatus(name, account string, status int) error {
	return fmt.Errorf("unexpected status %d checking whether %q already exists in %q", status, name, account)
}

func errBlueprintCreateFailed(name, account string, cause error) error {
	return fmt.Errorf("failed to reserve %q in %q: %w", name, account, cause)
}

func errRegistrationFailed(cause error) error {
	return fmt.Errorf("registration failed: %w", cause)
}

func errServerStatus(status int, summary string) error {
	if summary == "" {
		return fmt.Errorf("server returned status %d", status)
	}
	return fmt.Errorf("server returned status %d: %s", status, summary)
}

func errRegistrationUnauthorized(summary string) error {
	return fmt.Errorf("%w\n%s", errServerStatus(http.StatusUnauthorized, summary), msgReauthenticate())
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

func errTraceEvaluationFilter(value string) error {
	return fmt.Errorf("--evaluation %q is not valid; use evaluated or not_evaluated", value)
}

func errEvalSetNotFound(name, account string) error {
	return fmt.Errorf("blueprint %q not found in account %q", name, account)
}

func errEvaluationNotConfigured() error {
	return fmt.Errorf("evaluation is not configured in this environment")
}

func msgTraceEvaluationUnavailable(err error) string {
	return fmt.Sprintf("Could not load the trace's evaluation: %v", err)
}

func errEvalRunTraceWithOutdated() error {
	return fmt.Errorf("--include-outdated applies to the batch run and can't be combined with --trace-id")
}

func errEvalRunAlreadyActive(traceID string) error {
	return fmt.Errorf("an evaluation is already running for trace %q", traceID)
}

func msgEvalRunQueued(queued, failed, limit int) string {
	if queued == 0 && failed == 0 {
		return "No traces to evaluate"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Queued %d trace evaluations", queued)
	if failed > 0 {
		fmt.Fprintf(&b, ", %d failed to queue", failed)
	}
	fmt.Fprintf(&b, ". Check progress with `%s eval status`.", buildinfo.BinaryName)
	if queued >= limit {
		fmt.Fprintf(&b, " Up to %d traces run per call, so more may remain: run it again.", limit)
	}
	return b.String()
}

func msgEvalRunTraceQueued(traceID, runID, status, deploymentID string) string {
	return fmt.Sprintf("Queued evaluation for trace %s (run %s, %s). Check the result with `%s agent trace --id %s -t %s`.",
		traceID, runID, status, buildinfo.BinaryName, deploymentID, traceID)
}

func errEvalReviewTraceRequired() error {
	return fmt.Errorf("--trace-id is required")
}

func errEvalReviewSetRequired() error {
	return fmt.Errorf("at least one --set or --set-string key=value is required")
}

func errEvalSetFlagFormat(flag, pair string) error {
	return fmt.Errorf("--%s %q must be key=value", flag, pair)
}

func errEvalSetFlagDuplicate(key string) error {
	return fmt.Errorf("--set %q is given more than once", key)
}

func errEvalReviewInvalid(message string) error {
	return fmt.Errorf("review rejected: %s", message)
}

func errEvalReviewConflict(message, traceID string) error {
	return fmt.Errorf("review rejected: %s. Run `%s eval run -t %s` to evaluate against the current evaluation set, then review again",
		message, buildinfo.BinaryName, traceID)
}

func msgEvalReviewSaved(traceID string, evaluators int, datasetRequested, datasetUpdated bool) string {
	msg := fmt.Sprintf("Saved review for trace %s (%d evaluators).", traceID, evaluators)
	switch {
	case datasetUpdated:
		msg += " Dataset item updated."
	case datasetRequested:
		msg += " Dataset item not updated."
	}
	return msg
}

func msgNoEvaluators(name string) string {
	return fmt.Sprintf("No evaluators in the active evaluation set for %s", name)
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
	const msg = `the agent never bound :%d, so no turn can be delivered

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
  3. The agent crashed on boot, which its log will show.`
	return fmt.Errorf(msg, //nolint:staticcheck
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

func msgNoAgentSpend() string {
	return "No metered compute this period"
}

func msgNoModelSpend() string {
	return "No metered AI Gateway spend this period"
}

func msgNoModelAgentSpend(model string) string {
	return fmt.Sprintf("No metered spend on %s this period", model)
}

func msgNoFeatureSpend() string {
	return "No platform-feature AI Gateway spend this period"
}

func msgFeatureSpendUnavailable() string {
	return "Platform-feature spend is not available in this environment"
}

func errUnknownNetworkDirection(direction string) error {
	return fmt.Errorf("--direction %q is not a network direction; use inbound, outbound, or database", direction)
}

func msgNoNetworkFlows() string {
	return "No peers in this window"
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
	return line + ". Earlier crashes may be missing from the lines below."
}

func msgAlertLine(severity, title, workload, state string, since string) string {
	line := fmt.Sprintf("%s  %s  %s", severity, title, state)
	if workload != "" {
		line = fmt.Sprintf("%s  %s  %s  %s", severity, title, workload, state)
	}
	if since != "" {
		line = fmt.Sprintf("%s since %s", line, since)
	}
	return line
}

func msgUsageWindow(days int) string {
	if days <= 0 {
		return "No usage window reported"
	}
	return fmt.Sprintf("Last %d days", days)
}

func msgUsageDollars(amount float64) string {
	return fmt.Sprintf("$%.2f", amount)
}

func msgUsageComputeHours(cuHours float64) string {
	return fmt.Sprintf("%.3f CU-hours", cuHours)
}

func msgUsageLastTrace(at string) string {
	return fmt.Sprintf("Last trace %s", at)
}

func errInvalidSchedule(raw string) error {
	return fmt.Errorf(`invalid --schedule %q: expected <job>=<cron expression>, e.g. --schedule weekly-sync="0 3 * * *"`, raw)
}

func errDuplicateSchedule(name string) error {
	return fmt.Errorf("--schedule %s was given more than once", name)
}

func errInvalidCronExpression(name, cron string) error {
	return fmt.Errorf(`invalid cron expression %q for --schedule %s: expected five fields (minute hour day-of-month month day-of-week), e.g. "0 3 * * *"`, cron, name)
}

func errAgentNoJobs(label string) error {
	return fmt.Errorf("%s runs no jobs, so there is nothing to trigger", label)
}

func errAgentUnknownJob(name string, available []string) error {
	return fmt.Errorf("no job named %s (available: %s)", name, strings.Join(available, ", "))
}

func msgAgentTriggering(name, label string) string {
	return fmt.Sprintf("Triggering %s on %s", name, label)
}

func msgAgentTriggered(name string) string {
	return fmt.Sprintf("%s triggered", name)
}

func msgAgentJobsHeader(label string) string {
	return fmt.Sprintf("Jobs on %s:", label)
}

func errUnknownJobSchedule(unknown, available []string) error {
	if len(available) == 0 {
		return fmt.Errorf("this blueprint runs no job on a schedule, so --schedule %s has nothing to set", strings.Join(unknown, ", "))
	}
	return fmt.Errorf("no scheduled job named %s (available: %s)", strings.Join(unknown, ", "), strings.Join(available, ", "))
}

func msgAdapterSkipped(adapter, envVar string) string {
	return fmt.Sprintf(
		"⚠ %s adapter listed but %s not set, skipping (run '%s project configure' to add it)",
		adapter, envVar, buildinfo.BinaryName,
	)
}

func errRedeployLatestWithBuild() error {
	return fmt.Errorf("--latest and --build are mutually exclusive: --latest resolves the newest build, --build pins one")
}

func errBlueprintNotFound(name, account string) error {
	return fmt.Errorf("blueprint %q not found in account %q", name, account)
}

func errBlueprintNoPublishedBuild(name string) error {
	return fmt.Errorf("blueprint %q has no published build", name)
}

func errBlueprintBuildsNotFound(name, account string) error {
	return fmt.Errorf("blueprint %q not found in account %q, or you lack the edit access its build history needs", name, account)
}

func msgNoBlueprintBuilds(name string) string {
	return fmt.Sprintf("No builds found for blueprint %s", name)
}

func errBlueprintBuildArgs(got int) error {
	return fmt.Errorf("expected <blueprint name> and an optional [build-id], but got %d arguments", got)
}

func errUnknownSeverity(value string) error {
	return fmt.Errorf("unknown severity %q: use critical, high, medium, low, or unknown", value)
}

func errBuildNotFound(name, buildID string, searched int) error {
	return fmt.Errorf("build %s not found in the last %d builds of blueprint %q (list them with: %s blueprint builds list %s)", buildID, searched, name, buildinfo.BinaryName, name)
}

func errBlueprintNoServerBuilds(name string) error {
	return fmt.Errorf("blueprint %q has no server-side builds; builds pushed with the CLI run on your machine, so their logs stay there", name)
}

func errBuildLogsNotFound(name, buildID string) error {
	return fmt.Errorf("no server-side logs for build %s of blueprint %q: builds pushed with the CLI run on your machine, so their logs stay there (reading build logs also needs edit access)", buildID, name)
}

func msgNoBuildLogsYet(buildID, phase string) string {
	return fmt.Sprintf("No logs yet for build %s (%s)", buildID, phase)
}

func msgBuildFinished(buildID, phase string) string {
	return fmt.Sprintf("Build %s %s", buildID, phase)
}

func errBuildFailed(buildID string) error {
	return fmt.Errorf("build %s failed", buildID)
}

func errBuildCancelled(buildID string) error {
	return fmt.Errorf("build %s was cancelled", buildID)
}

func errRebuildCancelsRunningBuild(buildID, status string) error {
	return fmt.Errorf("build %s is still %s, and a rebuild cancels it; run again with --yes to rebuild anyway", buildID, status)
}

func errRebuildUnavailable(name, account string) error {
	return fmt.Errorf("blueprint %q in account %q has no connected GitHub repository or hosted source to rebuild, or you lack the operate access a rebuild needs", name, account)
}

func errRebuildGitHubNotConnected() error {
	return fmt.Errorf("your GitHub account is not connected in this organization, so the server cannot read the branch head; connect GitHub in the web app and try again")
}

func msgRebuildStarted(buildID, commitSHA, commitMessage string) string {
	return strings.TrimSpace(fmt.Sprintf("Rebuild started: build %s from commit %s %s", buildID, shortSHA(commitSHA), commitTitle(commitMessage)))
}

func msgTailBuildLogs(name, buildID string) string {
	return fmt.Sprintf("Stream its logs with: %s blueprint builds logs %s %s --tail", buildinfo.BinaryName, name, buildID)
}

func errEnvironmentsUnavailable(blueprint string) error {
	return fmt.Errorf("blueprint %q is public; environments are only available for private blueprints", blueprint)
}

func errEnvironmentNotFound(name, blueprint string, available []string) error {
	if len(available) == 0 {
		return fmt.Errorf("blueprint %q has no environment %q; create one with:\n  %s env create %s --blueprint %s",
			blueprint, name, buildinfo.BinaryName, name, blueprint)
	}
	return fmt.Errorf("blueprint %q has no environment %q (environments: %s)", blueprint, name, strings.Join(available, ", "))
}

func errEnvironmentNameTaken(name, blueprint string) error {
	return fmt.Errorf("blueprint %q already has an environment named %q", blueprint, name)
}

func errEnvironmentHasAgent(name, blueprint string) error {
	return fmt.Errorf("environment %q has an agent; delete the agent first:\n  %s agent delete --blueprint %s --env %s",
		name, buildinfo.BinaryName, blueprint, name)
}

func errEnvironmentOccupied(name, blueprint string) error {
	return fmt.Errorf("environment %q already has an agent; redeploy it instead:\n  %s agent redeploy --blueprint %s --env %s",
		name, buildinfo.BinaryName, blueprint, name)
}

func errEnvironmentHasNoAgent(name, blueprint string) error {
	return fmt.Errorf("environment %q has no agent; deploy into it with:\n  %s deploy %s --env %s",
		name, buildinfo.BinaryName, blueprint, name)
}

func errBlueprintRequired() error {
	return fmt.Errorf("name a blueprint, or run this in a project directory with an astropods.yml")
}

func errAgentTargetAmbiguous(target string, matches []string) error {
	return fmt.Errorf("%q matches %d agents; pick one with --id, or with --blueprint and --env:\n  %s",
		target, len(matches), strings.Join(matches, "\n  "))
}

func msgEnvironmentCreated(name, blueprint string) string {
	return fmt.Sprintf("Created environment %q for %s. Deploy into it with:\n  %s deploy %s --env %s",
		name, blueprint, buildinfo.BinaryName, blueprint, name)
}

func msgEnvironmentRenamed(from, to string) string {
	return fmt.Sprintf("Renamed environment %q to %q", from, to)
}

func msgEnvironmentDeleted(name string) string {
	return fmt.Sprintf("Deleted environment %q and its variables and secrets", name)
}

func msgNoEnvironments(blueprint string) string {
	return fmt.Sprintf("Blueprint %s has no environments yet. Deploying it creates one.", blueprint)
}

func errBlueprintNeedsEnvironment() error {
	return fmt.Errorf("--blueprint needs --env to say which environment to use")
}

func msgOverridesAccountValue() string {
	return "overrides the account value"
}

func msgDeployed(environment string) string {
	if environment == "" {
		return "deployed"
	}
	return "deployed into environment " + environment
}

func errPrivateLinkNeedsCluster() error {
	return fmt.Errorf("--private-link needs at least one --cluster to create an endpoint on")
}

// AI Gateway (ast gateway).

func errGatewayNotEnabled(account string) error {
	return fmt.Errorf("the AI Gateway isn't turned on for %s; an admin can turn it on in Settings → AI Gateway", account)
}

func errGatewayUnavailable() error {
	return fmt.Errorf("the AI Gateway isn't available in this environment")
}

func errGatewaySetupInProgress() error {
	return fmt.Errorf("another setup for this device is already running; wait a moment and retry")
}

func errGatewayConnectIncomplete(binary string, err error) error {
	return fmt.Errorf("couldn't finish connecting this device (%w); run `%s gateway connect` again", err, binary)
}

func errGatewayRequestFailed(action string, err error) error {
	return fmt.Errorf("could not %s: %w", action, err)
}

func errGatewayForeignBaseURL(path, current string) error {
	return fmt.Errorf(
		"%s already routes Claude Code to another gateway (%s); rerun with --replace-existing to replace it",
		path, current,
	)
}

func errGatewayOtherAccount(current, wanted string) error {
	return fmt.Errorf(
		"this machine reports to %s; a machine reports to one account, so rerun with --replace-existing to switch it to %s",
		current, wanted,
	)
}

func errGatewayKeyNotFound(key string) error {
	return fmt.Errorf("no device key matches %q; run '%s gateway devices' to list them", key, buildinfo.BinaryName)
}

func errGatewayKeyAmbiguous(key string, n int) error {
	return fmt.Errorf("%q matches %d device keys; use more of the key prefix or the full key id", key, n)
}

func msgGatewayConnecting(account string) string {
	return fmt.Sprintf("Connecting this machine to the AI Gateway for %s…", account)
}

func msgGatewayConfirmForeignBaseURL(path, current string) (title, description string) {
	return fmt.Sprintf("%s already routes Claude Code to another gateway. Replace it?", path),
		fmt.Sprintf("It points at %s. That gateway will stop receiving Claude Code traffic from this machine.", current)
}

func msgGatewayConfirmOtherAccount(current, wanted string) (title, description string) {
	return fmt.Sprintf("Switch this machine from %s to %s?", current, wanted),
		fmt.Sprintf("A machine reports to one account. Its usage will appear in %s from now on.", wanted)
}

func msgGatewayConnected(account, binary string) string {
	return fmt.Sprintf(
		"Claude Code keeps your current login and billing. Your usage now appears in %s's Insights.\n"+
			"Send any prompt in Claude Code, then run `%s gateway status` to confirm.",
		account, binary,
	)
}

func msgGatewayNotConnected(binary string) string {
	return fmt.Sprintf("This machine isn't connected to an AI Gateway. Run `%s gateway connect` to connect it.", binary)
}

func msgGatewaySwitchRevokeFailed(account, prefix, binary string, err error) string {
	return fmt.Sprintf(
		"! This device's old key on %s could not be revoked (%v).\n  Revoke it with `%s gateway revoke %s` while signed in to %s.",
		account, err, binary, prefix, account,
	)
}

func msgGatewayDisconnectRevokeFailed(prefix, binary string, err error) string {
	return fmt.Sprintf(
		"! Settings restored, but the key could not be revoked (%v).\n  Retry with `%s gateway revoke %s`.",
		err, binary, prefix,
	)
}

func msgGatewayRevokedThisDevice(binary string) string {
	return fmt.Sprintf("That was this machine's key. Run `%s gateway disconnect` to also remove it from your Claude Code settings.", binary)
}

func msgGatewayAutoConnectPick(binary string) (title, description string) {
	return "Which organization should Claude Code on this machine report usage to?",
		fmt.Sprintf("A machine reports to one organization. Change it later with `%s gateway connect --account`.", binary)
}

func msgGatewayAutoConnectSkipped(binary string, accounts []string) string {
	return fmt.Sprintf(
		"Several of your organizations use the AI Gateway (%s).\nRun `%s gateway connect --account <name>` to choose one for this machine.",
		strings.Join(accounts, ", "), binary,
	)
}

func msgGatewayAutoConnectFailed(binary string, err error) string {
	return fmt.Sprintf("! Could not connect this machine to the AI Gateway (%v). Run `%s gateway connect` to retry.", err, binary)
}

func msgGatewayStatusConnected(account, device, prefix, lastUsed string) string {
	return fmt.Sprintf("AI Gateway: connected (%s)\n\n  This device   %s · %s… · last used %s", account, device, prefix, lastUsed)
}

func msgGatewayStatusUnreachable(account string, err error) string {
	return fmt.Sprintf("AI Gateway: connected to %s (could not reach the server: %v)\n", account, err)
}

func msgGatewayStatusKeyGone(account, binary string) string {
	return fmt.Sprintf("AI Gateway: this device's key no longer exists in %s. Run `%s gateway connect` to set it up again.\n", account, binary)
}

func msgGatewayStatusCollectionOff(account, binary string) string {
	return fmt.Sprintf("  ! %s turned off the AI Gateway, so it no longer collects this device's usage. Claude Code still routes through it until you run `%s gateway disconnect`.", account, binary)
}

func msgGatewayStatusRevoked(when, binary string) string {
	return fmt.Sprintf("AI Gateway: disconnected. This device's key was revoked %s. Run `%s gateway connect` to set it up again.\n", when, binary)
}

func msgGatewayStatusNoRouting(binary string) string {
	return fmt.Sprintf("  Routing       not set, so Claude Code talks to Anthropic directly. Run `%s gateway connect`.", binary)
}

func msgGatewayStatusOverridden(layer string) string {
	return fmt.Sprintf("  ! %s points somewhere else, so Claude Code here bypasses the gateway.", layer)
}

func msgGatewayStatusShellExport(key, value string) string {
	return fmt.Sprintf("  ! Your shell exports %s=%s, which may override the settings above.", key, value)
}

func msgGatewayStatusSettingsMissing(binary string) string {
	return fmt.Sprintf("  ! Your Claude Code settings no longer hold what connect wrote for this device. Run `%s gateway connect` to repair them.", binary)
}

func msgGatewayConnectTarget(device, goos, goarch, settingsPath string) string {
	return fmt.Sprintf("  Device     %s (%s %s)\n  Settings   %s\n", device, goos, goarch, settingsPath)
}

func msgGatewayKeyIssued(prefix string) string {
	return fmt.Sprintf("✓ Issued a key for this device (%s…)", prefix)
}

func msgGatewaySettingsUpdated(path string) string {
	return fmt.Sprintf("✓ Updated %s", path)
}

func msgGatewaySettingWritten(key, label string) string {
	return fmt.Sprintf("    %-26s %s", key, label)
}

func msgGatewaySettingHidden() string {
	return "(set)"
}

func msgGatewaySettingsRestored(path string) string {
	return fmt.Sprintf("✓ Restored %s to how it was before connect", path)
}

func msgGatewaySettingKept(key string) string {
	return fmt.Sprintf("  %s changed after connect, so it was left as it is", key)
}

func msgGatewayThisDeviceKeyRevoked(prefix string) string {
	return fmt.Sprintf("✓ Revoked this device's key (%s…)", prefix)
}

func msgGatewayNoDevices(account string) string {
	return fmt.Sprintf("No devices are connected to %s's AI Gateway.", account)
}

func msgGatewayDeviceKeyRevoked(device, prefix string) string {
	return fmt.Sprintf("✓ Revoked %s's key (%s…)", device, prefix)
}

func msgGatewayStatusRouting(url, setBy string, alsoIn []string) string {
	line := fmt.Sprintf("  Routing       %s\n  Set by        %s", url, setBy)
	if len(alsoIn) > 0 {
		line += ", also in " + strings.Join(alsoIn, ", ")
	}
	return line
}

func msgGatewaySettingsScope(scope claudesettings.Scope) string {
	switch scope {
	case claudesettings.ScopeManaged:
		return "managed settings (your organization)"
	case claudesettings.ScopeProjectLocal:
		return "this project's local settings"
	case claudesettings.ScopeProject:
		return "this project's shared settings"
	}
	return "your user settings"
}
