package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	spec "github.com/astropods/astro-spec"

	"github.com/astropods/astro-cli/internal/agentcore"
)

// registerAgentCoreDeployFlags adds the AgentCore-specific deploy knobs. The
// runtime itself is chosen by meta.agentcore in the spec (no --target flag);
// these are the AWS/VPC parameters a real deploy needs. All are optional so a
// --dry-run renders with placeholders and no AWS details.
func registerAgentCoreDeployFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	cmd.Flags().String("region", "", "AWS region (AgentCore deploy)")
	cmd.Flags().String("role", "", "IAM execution role ARN for the AgentCore runtime")
	cmd.Flags().String("image", "", "ARM64 ECR image URI for the AgentCore runtime")
	cmd.Flags().String("profile", "", "AWS CLI profile (AgentCore deploy)")
	cmd.Flags().StringArray("dep-host", nil, "Rewrite an in-cluster host to a VPC-resolvable name: in-cluster=vpc-name (repeatable)")
	cmd.Flags().StringArray("secret", nil, "Secret NAME=VALUE resolving an @SECRET: placeholder (repeatable; AgentCore deploy)")
	cmd.Flags().String("secrets-file", "", "Load AgentCore deploy secrets from a .env file")
}

// maybeAgentCoreDeploy resolves the local astropods.yml and, if meta.agentcore
// is set, runs the client-side AgentCore deploy path. It returns handled=false
// (no error) to fall through to the default server-mediated deploy when there is
// no local spec or the spec doesn't opt into AgentCore.
func maybeAgentCoreDeploy(cmd *cobra.Command, _ []string) (bool, error) {
	specFile, _ := cmd.Flags().GetString("file")
	specPath, workingDir, err := resolveSpecPathAndCwd(specFile)
	if err != nil {
		// No local spec found: this is a name-based server deploy.
		return false, nil
	}
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		// An explicit --file that won't parse is a real error; otherwise fall
		// through to the server path.
		if specFile != "" {
			return true, err
		}
		return false, nil
	}
	if !astroSpec.Meta.AgentCore {
		return false, nil
	}
	return true, runAgentCoreDeploy(cmd, astroSpec, workingDir)
}

// runAgentCoreDeploy translates the spec into an AgentCore plan and either prints
// it (--dry-run: plan JSON + the aws CLI commands a real deploy would run, with
// secrets masked) or performs the deploy via the authenticated `aws` CLI.
func runAgentCoreDeploy(cmd *cobra.Command, s *spec.AstroSpec, _ string) error {
	w := cmd.OutOrStdout()
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	region, _ := cmd.Flags().GetString("region")
	role, _ := cmd.Flags().GetString("role")
	image, _ := cmd.Flags().GetString("image")
	profile, _ := cmd.Flags().GetString("profile")
	depHostFlags, _ := cmd.Flags().GetStringArray("dep-host")
	secretFlags, _ := cmd.Flags().GetStringArray("secret")
	secretsFile, _ := cmd.Flags().GetString("secrets-file")

	depHosts, err := parseDepHosts(depHostFlags)
	if err != nil {
		return err
	}

	opts := agentcore.Options{
		Region:          region,
		ImageURI:        firstNonEmpty(image, placeholderImage),
		ExecutionRole:   firstNonEmpty(role, placeholderRole),
		DependencyHosts: depHosts,
	}
	plan, err := agentcore.Build(s, opts)
	if err != nil {
		var rej *agentcore.RejectionError
		if errors.As(err, &rej) {
			return fmt.Errorf("cannot deploy %q to AgentCore Runtime: %s", s.Name, rej.Reason)
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
		// Print the plan BEFORE resolving secrets, so the plan JSON only ever
		// shows @SECRET: placeholders — never resolved secret values. Encode
		// without HTML-escaping so placeholders like <ECR-IMAGE-URI> stay readable.
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(plan); err != nil {
			return err
		}
		// Resolve now (mutates the plan env) only to build the masked aws
		// commands and report what's still missing; values stay masked in output.
		unresolved := agentcore.ResolveSecrets(plan, secrets)
		if len(unresolved) > 0 {
			fmt.Fprintf(w, "\n# secrets still needed (supply via --secret NAME=VALUE / --secrets-file): %s\n", strings.Join(unresolved, ", "))
		}
		fmt.Fprintln(w, "\n# aws CLI commands a real deploy would run (dry-run, not executed; secret values masked):")
		rt := &agentcore.AWSCLIRuntime{
			Profile: profile, Region: region, DryRun: true, SecretKeys: secretKeys,
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

	// Real deploy (the deployment stage): fail closed on unresolved secrets and
	// require a real image + role rather than the placeholders.
	unresolved := agentcore.ResolveSecrets(plan, secrets)
	if len(unresolved) > 0 {
		return fmt.Errorf("missing secret value(s): %s — supply via --secret NAME=VALUE or --secrets-file", strings.Join(unresolved, ", "))
	}
	if opts.ImageURI == placeholderImage || opts.ExecutionRole == placeholderRole {
		return fmt.Errorf("a real AgentCore deploy requires --image <ecr-uri> and --role <execution-role-arn> (use --dry-run to preview without them)")
	}
	rt := &agentcore.AWSCLIRuntime{Profile: profile, Region: region, SecretKeys: secretKeys}
	res, err := agentcore.Run(plan, agentcore.TargetAWS, rt, region, "")
	if err != nil {
		return err
	}
	out, err := res.JSON()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, out)
	fmt.Fprintln(w, "\n# messaging sidecar env for this runtime:")
	fmt.Fprint(w, res.EnvExports())
	return nil
}

const (
	placeholderImage = "<ECR-IMAGE-URI>"
	placeholderRole  = "<EXECUTION-ROLE-ARN>"
)

// parseDepHosts turns repeatable in-cluster=vpc-name flags into a rewrite map.
func parseDepHosts(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("invalid --dep-host %q: expected in-cluster=vpc-name", p)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, nil
}

// collectAgentCoreSecrets merges a --secrets-file (KEY=VALUE lines) with repeated
// --secret NAME=VALUE flags. Inline --secret overrides file entries. Values are
// never logged.
func collectAgentCoreSecrets(pairs []string, file string) (map[string]string, error) {
	out := map[string]string{}
	if file != "" {
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

func firstNonEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
