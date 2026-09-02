package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	spec "github.com/astropods/astro-spec"

	"github.com/astropods/astro-cli/internal/agentcore"
)

// AgentCore deploy, selected by the spec.
//
// The runtime is read from astropods.yml (agent.annotations.runtime), so there
// is no target flag and no runtime-specific command: `ast deploy` dispatches on
// what the spec declares. A default (kubernetes) spec takes the server-mediated
// path; an agentcore spec is created here, client-side, through the
// authenticated `aws` CLI.
//
// This path is not equivalent to a server-mediated deploy and cannot be. Vault
// secret references are write-only and resolve server-side, and the AI Gateway
// key is minted server-side, so secrets are supplied literally instead
// (--secret / --secrets-file), matching AWS's reference wrapper. It also needs
// AWS credentials for the account that owns the runtime, so in practice it runs
// from a workstation or the ops bastion rather than by a tenant.
//
// Environment values come from the AWS environment rather than flags, because
// an IAM role ARN or a region is not blueprint data:
//
//	AGENTCORE_EXEC_ROLE_ARN  execution role the runtime assumes (same name astro-server uses)
//	AGENTCORE_SUBNETS        comma-separated VPC subnet ids; set => networkMode VPC
//	AGENTCORE_SECURITY_GROUPS comma-separated security group ids (required with subnets)
//	AGENTCORE_DEPENDENCY_HOSTS comma-separated from=to host rewrites (e.g. an in-cluster
//	                         service name -> a VPC-resolvable endpoint)
//	AWS_REGION, AWS_DEFAULT_REGION
//	AWS_PROFILE              read natively by the aws CLI
//
// The image URI stays a flag because it identifies the artifact being deployed:
// it changes per build and belongs to neither the blueprint nor the environment.

// agentCoreOnlyFlags are read only by the agentcore deploy path.
var agentCoreOnlyFlags = []string{"file", "image", "secret", "secrets-file"}

// registerAgentCoreDeployFlags adds the flags the agentcore path needs to a
// deploy command, hidden because only a spec that opts in reads them.
func registerAgentCoreDeployFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	f.String("image", "", "ECR image URI (agentcore runtime)")
	f.StringArray("secret", nil, "Secret NAME=VALUE resolving an @SECRET: placeholder (agentcore runtime; repeatable, never logged)")
	f.String("secrets-file", "", "Load agentcore secrets from a .env file (KEY=VALUE lines)")
	for _, name := range agentCoreOnlyFlags {
		_ = f.MarkHidden(name)
	}
}

// rejectAgentCoreOnlyFlags fails a server-path deploy that was given a flag only
// the agentcore path can honor, rather than accepting and discarding it.
func rejectAgentCoreOnlyFlags(cmd *cobra.Command) error {
	var passed []string
	for _, name := range agentCoreOnlyFlags {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			passed = append(passed, "--"+name)
		}
	}
	if len(passed) == 0 {
		return nil
	}
	return errAgentCoreOnlyFlags(passed)
}

// maybeAgentCoreDeploy runs the agentcore deploy when the local spec selects
// that runtime. It reports handled=false with no error when there is no local
// spec or the spec uses the default runtime, so `ast deploy <name>` keeps
// working against the server without a local checkout.
func maybeAgentCoreDeploy(cmd *cobra.Command, args []string) (handled bool, err error) {
	specFile, _ := cmd.Flags().GetString("file")
	specPath, _, err := resolveSpecPathAndCwd(specFile)
	if err != nil {
		// No local spec: this is a name-based server deploy.
		return false, nil
	}
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		// An explicit --file that will not parse is a real error; otherwise fall
		// through rather than failing a server deploy over a stray file.
		if specFile != "" {
			return true, err
		}
		return false, nil
	}
	if astroSpec.Agent.Runtime() != spec.AgentCoreRuntime {
		return false, nil
	}
	// This path deploys the local spec, so a name argument names a target it
	// cannot reach.
	if len(args) > 0 {
		return true, errAgentCoreDeployTakesNoName(args[0], astroSpec.Name)
	}
	return true, runAgentCoreDeploy(cmd, astroSpec, specPath)
}

// execRoleEnv names the execution role the runtime assumes. Deliberately the
// same variable astro-server reads, so an operator and the server can be
// pointed at one role without two spellings of the same setting.
const execRoleEnv = "AGENTCORE_EXEC_ROLE_ARN"

// Network placement is a property of the account deployed into, not the blueprint.
const (
	subnetsEnv         = "AGENTCORE_SUBNETS"
	securityGroupsEnv  = "AGENTCORE_SECURITY_GROUPS"
	dependencyHostsEnv = "AGENTCORE_DEPENDENCY_HOSTS"
	idleTimeoutEnv     = "AGENTCORE_IDLE_TIMEOUT"
	maxLifetimeEnv     = "AGENTCORE_MAX_LIFETIME"
)

// A create or update is accepted before the runtime is usable, so the deploy
// polls until the control plane reports a terminal state.
const (
	agentCoreReadyTimeout   = 10 * time.Minute
	agentCoreStatusInterval = 5 * time.Second
)

// Placeholders let --dry-run render a complete plan with no AWS details set. A
// real deploy rejects them rather than sending them to AWS.
const (
	placeholderImage = "<ECR-IMAGE-URI>"
	placeholderRole  = "<EXECUTION-ROLE-ARN>"
)

func runAgentCoreDeploy(cmd *cobra.Command, astroSpec *spec.AstroSpec, specPath string) error {
	w := cmd.OutOrStdout()
	image, _ := cmd.Flags().GetString("image")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	secretFlags, _ := cmd.Flags().GetStringArray("secret")
	secretsFile, _ := cmd.Flags().GetString("secrets-file")

	depHosts, err := parseHostPairs(os.Getenv(dependencyHostsEnv))
	if err != nil {
		return fmt.Errorf("%s: %w", dependencyHostsEnv, err)
	}
	idleTimeout, err := parseSeconds(os.Getenv(idleTimeoutEnv))
	if err != nil {
		return fmt.Errorf("%s: %w", idleTimeoutEnv, err)
	}
	maxLifetime, err := parseSeconds(os.Getenv(maxLifetimeEnv))
	if err != nil {
		return fmt.Errorf("%s: %w", maxLifetimeEnv, err)
	}
	subnets := splitCSV(os.Getenv(subnetsEnv))
	groups := splitCSV(os.Getenv(securityGroupsEnv))
	// Subnets alone build a VPC plan AWS rejects, and drop the ingress rules
	// the plan derives from the first security group.
	if len(subnets) > 0 && len(groups) == 0 {
		return fmt.Errorf("%s is set but %s is empty: a VPC runtime needs both", subnetsEnv, securityGroupsEnv)
	}

	opts := agentcore.Options{
		Region:          awsRegionFromEnv(),
		ImageURI:        orPlaceholder(image, placeholderImage),
		ExecutionRole:   orPlaceholder(os.Getenv(execRoleEnv), placeholderRole),
		Subnets:         subnets,
		SecurityGroups:  groups,
		DependencyHosts: depHosts,

		IdleTimeoutSeconds: idleTimeout,
		MaxLifetimeSeconds: maxLifetime,
	}
	plan, err := agentcore.Build(astroSpec, opts)
	if err != nil {
		var rej *agentcore.RejectionError
		if errors.As(err, &rej) {
			return errAgentCoreRejected(astroSpec.Name, rej.Reason)
		}
		return err
	}

	secrets, err := collectAgentCoreSecrets(secretFlags, secretsFile)
	if err != nil {
		return err
	}
	secretKeys := make(map[string]bool, len(secrets))
	for k := range secrets {
		secretKeys[k] = true
	}

	if dryRun {
		return renderAgentCorePlan(w, plan, secrets, secretKeys, opts.Region)
	}

	// A real deploy resolves secrets first and fails closed, so an unresolved
	// @SECRET: placeholder can never be written to the runtime as a literal.
	if unresolved := agentcore.ResolveSecrets(plan, secrets); len(unresolved) > 0 {
		return errAgentCoreMissingSecrets(unresolved)
	}
	if opts.ImageURI == placeholderImage {
		return errAgentCoreMissingImage()
	}
	if opts.ExecutionRole == placeholderRole {
		return errAgentCoreMissingExecRole()
	}

	rt := &agentcore.AWSCLIRuntime{Region: opts.Region, SecretKeys: secretKeys}
	res, err := agentcore.Run(plan, agentcore.TargetAWS, rt, opts.Region, "")
	if err != nil {
		return err
	}
	out, err := res.JSON()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, out)
	if err := waitAgentCoreReady(cmd.Context(), rt, res, w); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n# messaging sidecar env for this runtime (from %s):\n", specPath)
	fmt.Fprint(w, res.EnvExports())
	return nil
}

// waitAgentCoreReady reports the runtime's progress to READY. A timeout is a
// warning because the deploy was accepted; a reported failure is an error.
func waitAgentCoreReady(ctx context.Context, rt agentcore.Runtime, res *agentcore.Result, w io.Writer) error {
	if res.RuntimeID == "" {
		return nil
	}
	fmt.Fprintln(w, "\n# waiting for the runtime to report READY")
	err := agentcore.WaitReady(ctx, rt, res.RuntimeID, res.Version,
		agentCoreReadyTimeout, agentCoreStatusInterval,
		func(status string) { fmt.Fprintf(w, "  %s\n", status) })
	if errors.Is(err, agentcore.ErrWaitTimeout) {
		fmt.Fprintf(w, "  %s\n", err)
		return nil
	}
	return err
}

// renderAgentCorePlan prints the plan, then the aws commands a real deploy would
// run. The plan is encoded BEFORE secrets are resolved so its env only ever
// shows @SECRET: placeholders; the commands mask secret values.
func renderAgentCorePlan(w io.Writer, plan *agentcore.Plan, secrets map[string]string, secretKeys map[string]bool, region string) error {
	// SetEscapeHTML(false) keeps placeholders like <ECR-IMAGE-URI> readable.
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(plan); err != nil {
		return err
	}
	if unresolved := agentcore.ResolveSecrets(plan, secrets); len(unresolved) > 0 {
		fmt.Fprintf(w, "\n# secrets still needed (--secret NAME=VALUE / --secrets-file): %s\n",
			strings.Join(unresolved, ", "))
	}
	fmt.Fprintln(w, "\n# aws commands a real deploy would run (not executed; secret values masked):")
	rt := &agentcore.AWSCLIRuntime{
		Region: region, DryRun: true, SecretKeys: secretKeys,
		Out: func(line string) { fmt.Fprintln(w, line) },
	}
	res, err := agentcore.Run(plan, agentcore.TargetAWS, rt, region, "")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "\n# messaging sidecar env for this runtime (AGENT_RUNTIME_ARN filled on a real deploy):")
	fmt.Fprint(w, res.EnvExports())
	return nil
}

// awsRegionFromEnv follows the AWS CLI's own precedence so the region does not
// need a flag. An empty result leaves it to the aws CLI's remaining resolution
// (config file, instance metadata).
func awsRegionFromEnv() string {
	if r := strings.TrimSpace(os.Getenv("AWS_REGION")); r != "" {
		return r
	}
	return strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
}

func orPlaceholder(v, placeholder string) string {
	if strings.TrimSpace(v) == "" {
		return placeholder
	}
	return v
}

// splitCSV reads a comma-separated environment value, dropping blanks.
func splitCSV(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parseSeconds reads a positive whole number of seconds. Empty means unset, so
// the plan keeps its default.
func parseSeconds(v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q, want a number of seconds", v)
	}
	if n < agentcore.MinSessionSeconds || n > agentcore.MaxSessionSeconds {
		return 0, fmt.Errorf("%d seconds is out of range, want %d to %d",
			n, agentcore.MinSessionSeconds, agentcore.MaxSessionSeconds)
	}
	return n, nil
}

// parseHostPairs reads "from=to,from=to" host rewrites. A malformed entry is an
// error, because a dropped rewrite only surfaces as an unresolvable dependency.
func parseHostPairs(v string) (map[string]string, error) {
	pairs := splitCSV(v)
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		from, to, ok := strings.Cut(p, "=")
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		if !ok || from == "" || to == "" {
			return nil, fmt.Errorf("invalid host rewrite %q, want from=to", p)
		}
		out[from] = to
	}
	return out, nil
}

// collectAgentCoreSecrets merges a --secrets-file (KEY=VALUE lines, `#` comments
// and blanks ignored) with repeated --secret NAME=VALUE flags; inline flags win.
// Values are never printed by callers.
func collectAgentCoreSecrets(pairs []string, file string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(file) != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read secrets-file: %w", err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				return nil, errAgentCoreSecretsFileLine(i + 1)
			}
			out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, errAgentCoreInvalidSecret(p)
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, nil
}
