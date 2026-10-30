package cmd

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	deployspec "github.com/astropods/astro-spec/deploy"
)

// deployFileExcludedFlags set what the deploy file owns, so -f refuses them.
var deployFileExcludedFlags = []string{"adapter", "var", "vars-file", "from-local", "schedule", "cluster", "name", "image", "secret", "secrets-file"}

func runDeployFile(cmd *cobra.Command, args []string, path string) error {
	if len(args) > 0 {
		return errDeployFileTakesNoName(args[0])
	}
	var excluded []string
	for _, name := range deployFileExcludedFlags {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			excluded = append(excluded, "--"+name)
		}
	}
	if len(excluded) > 0 {
		return errDeployFileFlags(excluded)
	}

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return errReadDeployFile(path, err)
	}
	doc, err := deployspec.Parse(string(data))
	if err != nil {
		return errInvalidDeployFile(path, err)
	}

	dir := filepath.Dir(path)
	specPath, err := resolveSpecPath("", dir)
	if err != nil {
		return errDeployFileNoSpec(dir)
	}
	name, err := specFileBlueprintName(specPath)
	if err != nil {
		return err
	}

	selected, _ := cmd.Flags().GetStringArray("env")
	if len(selected) == 0 {
		selected = doc.Names()
	}
	for _, env := range selected {
		if _, ok := doc.Environments[env]; !ok {
			return errDeployFileUnknownEnv(env, path, doc.Names())
		}
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	existing, err := listBlueprintEnvironments(cmd.Context(), at, at.Account, name, verbose)
	if err != nil {
		return err
	}

	build, _ := cmd.Flags().GetString("build")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	w := cmd.OutOrStdout()
	var failed []string
	for _, env := range selected {
		if err := deployFileEnvironment(cmd, at, verbose, name, env, doc.Environments[env], existing, build, dryRun); err != nil {
			color.New(color.FgRed).Fprintf(w, "  ✗ %s\n", msgDeployFileEnvironmentFailed(env, err)) //nolint:errcheck,gosec
			failed = append(failed, env)
		}
	}
	if len(failed) > 0 {
		return errDeployFileFailed(failed, len(selected))
	}
	return nil
}

func deployFileEnvironment(cmd *cobra.Command, at AccountToken, verbose bool, blueprint, envName string, entry deployspec.Environment, existing []blueprintEnvironment, build string, dryRun bool) error {
	var environment *blueprintEnvironment
	if i := slices.IndexFunc(existing, func(e blueprintEnvironment) bool { return e.Name == envName }); i >= 0 {
		environment = &existing[i]
	}

	req := deployFileTemplateRequest(entry)
	req.Build = build
	req.TargetAccount = at.Account

	displayName := blueprint + "-" + envName
	var target *blueprintEnvironment
	switch {
	case environment != nil && environment.DeploymentID != "":
		dep, err := getAgentDeploymentFull(cmd.Context(), environment.DeploymentID, at, verbose)
		if err != nil {
			return err
		}
		req.DeploymentID = dep.ID
		displayName = dep.DisplayName
	case environment != nil:
		target = environment
	case !dryRun:
		created, err := createBlueprintEnvironment(cmd.Context(), at, blueprint, envName, verbose)
		if err != nil {
			return err
		}
		target = created
	default:
		target = &blueprintEnvironment{Name: envName}
	}

	return runDeployWithRequest(cmd, at, verbose, at.Account, blueprint, displayName, target, req, dryRun, func(tmpl deployTemplateResponse) error {
		for varName, v := range req.Variables {
			if v.Value != "" && tmpl.Variables[varName].Secret {
				return errDeployFileSecretLiteral(varName)
			}
		}
		return nil
	})
}

func deployFileTemplateRequest(entry deployspec.Environment) deployTemplateRequest {
	tr := entry.TemplateRequest()
	req := deployTemplateRequest{
		ClusterID:    tr.ClusterID,
		Schedules:    tr.Schedules,
		Models:       tr.Models,
		Bindings:     tr.Bindings,
		Provisioning: tr.Provisioning,
	}
	if len(tr.Variables) > 0 {
		req.Variables = make(map[string]deployVarInput, len(tr.Variables))
		for name, v := range tr.Variables {
			req.Variables[name] = deployVarInput(v)
		}
	}
	if tr.Interfaces != nil {
		req.Interfaces = &deployTemplateInterfaces{Adapters: tr.Interfaces.Adapters, Auth: tr.Interfaces.Auth}
	}
	return req
}
