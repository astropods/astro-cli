package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
//	AWS_REGION, AWS_DEFAULT_REGION
//	AWS_PROFILE              read natively by the aws CLI
//
// The image URI stays a flag because it identifies the artifact being deployed:
// it changes per build and belongs to neither the blueprint nor the environment.

// registerAgentCoreDeployFlags adds the flags the agentcore path needs to a
// deploy command. All are inert on a default spec, which never reads them.
func registerAgentCoreDeployFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	f.String("image", "", "ECR image URI (agentcore runtime)")
	f.StringArray("secret", nil, "Secret NAME=VALUE resolving an @SECRET: placeholder (agentcore runtime; repeatable, never logged)")
	f.String("secrets-file", "", "Load agentcore secrets from a .env file (KEY=VALUE lines)")
}

// maybeAgentCoreDeploy runs the agentcore deploy when the local spec selects
// that runtime. It reports handled=false with no error when there is no local
// spec or the spec uses the default runtime, so `ast deploy <name>` keeps
// working against the server without a local checkout.
func maybeAgentCoreDeploy(cmd *cobra.Command) (handled bool, err error) {
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
	return true, runAgentCoreDeploy(cmd, astroSpec, specPath)
}

// execRoleEnv names the execution role the runtime assumes. Deliberately the
// same variable astro-server reads, so an operator and the server can be
// pointed at one role without two spellings of the same setting.
const execRoleEnv = "AGENTCORE_EXEC_ROLE_ARN"

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

	opts := agentcore.Options{
		Region:        awsRegionFromEnv(),
		ImageURI:      orPlaceholder(image, placeholderImage),
		ExecutionRole: orPlaceholder(os.Getenv(execRoleEnv), placeholderRole),
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
	fmt.Fprintf(w, "\n# messaging sidecar env for this runtime (from %s):\n", specPath)
	fmt.Fprint(w, res.EnvExports())
	return nil
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
