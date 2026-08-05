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

// AgentCore operator deploy.
//
// `ast deploy` is server-mediated: astro-server resolves vault secret
// references, mints the AI Gateway key, and holds the AWS credentials. None of
// that is available client-side, so this command is not that path and does not
// try to be. It exists to stand a runtime up from a workstation or the ops
// bastion in order to validate AgentCore itself, and it reaches AWS through the
// authenticated `aws` CLI. That means it needs credentials in the Astro AWS
// account, which is why it is hidden rather than part of the tenant surface.
//
// Everything describing the agent comes from astropods.yml, including the
// choice to come here at all (agent.annotations.runtime). Environment values
// come from the standard AWS environment rather than flags, because an IAM role
// ARN or a region is not blueprint data:
//
//	AGENTCORE_EXEC_ROLE_ARN  execution role the runtime assumes (same name astro-server uses)
//	AWS_REGION, AWS_DEFAULT_REGION
//	AWS_PROFILE              read natively by the aws CLI
//
// The image URI stays a flag because it identifies the artifact being deployed:
// it changes per build and belongs to neither the blueprint nor the environment.
var agentcoreCmd = &cobra.Command{
	Use:    "agentcore",
	Short:  "Operator commands for the AWS Bedrock AgentCore runtime",
	Hidden: true,
}

var agentcoreDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Create or update the AgentCore runtime for a local agentcore spec",
	Long: `Create or update an AWS Bedrock AgentCore runtime from the local spec.

Requires AWS credentials in the Astro AWS account, so this is an operator tool,
not the tenant deploy path. Secret values are passed literally (--secret /
--secrets-file); vault references only resolve in a server-mediated deploy.

Reads AGENTCORE_EXEC_ROLE_ARN, AWS_REGION (or AWS_DEFAULT_REGION), and
AWS_PROFILE from the environment. Use --dry-run to render the plan and the aws
commands without calling AWS.`,
	Args: cobra.NoArgs,
	RunE: runAgentCoreOpsDeploy,
}

func init() {
	rootCmd.AddCommand(agentcoreCmd)
	agentcoreCmd.AddCommand(agentcoreDeployCmd)
	registerAgentCoreDeployFlags(agentcoreDeployCmd)
}

func registerAgentCoreDeployFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	f.String("image", "", "ARM64 ECR image URI to deploy")
	f.Bool("dry-run", false, "Print the plan and the aws commands without calling AWS")
	f.StringArray("secret", nil, "Secret NAME=VALUE resolving an @SECRET: placeholder (repeatable; never logged)")
	f.String("secrets-file", "", "Load secrets from a .env file (KEY=VALUE lines)")
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

func runAgentCoreOpsDeploy(cmd *cobra.Command, _ []string) error {
	w := cmd.OutOrStdout()
	specFile, _ := cmd.Flags().GetString("file")
	image, _ := cmd.Flags().GetString("image")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	secretFlags, _ := cmd.Flags().GetStringArray("secret")
	secretsFile, _ := cmd.Flags().GetString("secrets-file")

	astroSpec, specPath, err := loadAgentCoreSpec(specFile)
	if err != nil {
		return err
	}

	opts := agentcore.Options{
		Region:        awsRegionFromEnv(),
		ImageURI:      orPlaceholder(image, placeholderImage),
		ExecutionRole: orPlaceholder(os.Getenv(execRoleEnv), placeholderRole),
	}
	plan, err := agentcore.Build(astroSpec, opts)
	if err != nil {
		var rej *agentcore.RejectionError
		if errors.As(err, &rej) {
			return fmt.Errorf("cannot deploy %q to AgentCore Runtime: %s", astroSpec.Name, rej.Reason)
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
		return fmt.Errorf("missing secret value(s) %s: supply them with --secret NAME=VALUE or --secrets-file",
			strings.Join(unresolved, ", "))
	}
	if opts.ImageURI == placeholderImage {
		return fmt.Errorf("a real deploy needs --image <ecr-uri> (use --dry-run to preview without it)")
	}
	if opts.ExecutionRole == placeholderRole {
		return fmt.Errorf("a real deploy needs %s set to the execution role ARN (use --dry-run to preview without it)", execRoleEnv)
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

// loadAgentCoreSpec resolves and parses the local spec, rejecting one that does
// not select the agentcore runtime. Unlike `ast deploy` there is nothing to fall
// through to here, so a mismatch is an error rather than a different code path.
func loadAgentCoreSpec(specFile string) (*spec.AstroSpec, string, error) {
	specPath, _, err := resolveSpecPathAndCwd(specFile)
	if err != nil {
		return nil, "", fmt.Errorf("this command needs a local spec: %w", err)
	}
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return nil, "", err
	}
	switch rt := astroSpec.Agent.Runtime(); rt {
	case spec.AgentCoreRuntime:
		return astroSpec, specPath, nil
	case "":
		return nil, "", fmt.Errorf("%s does not select the agentcore runtime: set agent.annotations.runtime: %s",
			specPath, spec.AgentCoreRuntime)
	default:
		return nil, "", fmt.Errorf("%s selects the %q runtime, not %s", specPath, rt, spec.AgentCoreRuntime)
	}
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
				return nil, fmt.Errorf("secrets-file line %d: expected KEY=VALUE", i+1)
			}
			out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("invalid --secret %q: expected NAME=VALUE", p)
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, nil
}
