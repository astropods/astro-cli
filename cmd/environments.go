package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
)

// environmentsServerURLOverride is set in tests to redirect API calls to a test server.
var environmentsServerURLOverride string

func environmentsBaseURL() string {
	if environmentsServerURLOverride != "" {
		return strings.TrimSuffix(environmentsServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

type blueprintEnvironment struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	AccountName        string `json:"account_name"`
	VariablesAvailable bool   `json:"variables_available"`
	DeploymentID       string `json:"deployment_id,omitempty"`
}

var envCmd = &cobra.Command{
	Use:     "env",
	Aliases: []string{"envs", "environment", "environments"},
	Short:   "Manage a blueprint's environments",
	Long: `An environment is where one agent of a blueprint runs in the active account.
It has its own variables and secrets, which override the account's values with the same name.
Deleting the agent keeps the environment and its values, ready for the next deploy.
Environments are only available for private blueprints.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the blueprint's environments in the active account",
	Args:  cobra.NoArgs,
	RunE:  runEnvList,
}

var envCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create an empty environment",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvCreate,
}

var envRenameCmd = &cobra.Command{
	Use:   "rename <name> <new-name>",
	Short: "Rename an environment",
	Args:  cobra.ExactArgs(2),
	RunE:  runEnvRename,
}

var envDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an environment with no agent, and its variables and secrets",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvDelete,
}

func init() {
	envCmd.PersistentFlags().StringP("blueprint", "b", "", "Blueprint the environments belong to (default: the name in ./astropods.yml)")
	envListCmd.Flags().Bool("json", false, "Output as JSON")
	envDeleteCmd.Flags().String("confirm", "", "Skip the prompt by passing the environment name as confirmation")
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envCreateCmd)
	envCmd.AddCommand(envRenameCmd)
	envCmd.AddCommand(envDeleteCmd)
	rootCmd.AddCommand(envCmd)
}

// listBlueprintEnvironments returns the blueprint's environments in the
// active account. The server lists every account the viewer can see.
func listBlueprintEnvironments(ctx context.Context, at AccountToken, blueprint string, verbose bool) ([]blueprintEnvironment, error) {
	var bp blueprintItem
	status, err := apiCall(ctx, http.MethodGet, apiPath(environmentsBaseURL(), at.Account, "agents", blueprint), nil, at.Token, verbose, &bp)
	if status == http.StatusNotFound {
		return nil, errBlueprintNotFound(blueprint, at.Account)
	}
	if err != nil {
		return nil, err
	}
	if bp.Visibility == string(VisibilityPublic) {
		return nil, errEnvironmentsUnavailable(blueprint)
	}

	var result struct {
		Environments []blueprintEnvironment `json:"environments"`
	}
	if _, err := apiCall(ctx, http.MethodGet, apiPath(environmentsBaseURL(), at.Account, "agents", blueprint, "environments"), nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	envs := make([]blueprintEnvironment, 0, len(result.Environments))
	for _, e := range result.Environments {
		if e.AccountName == at.Account {
			envs = append(envs, e)
		}
	}
	return envs, nil
}

func findBlueprintEnvironment(ctx context.Context, at AccountToken, blueprint, name string, verbose bool) (*blueprintEnvironment, error) {
	envs, err := listBlueprintEnvironments(ctx, at, blueprint, verbose)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(envs))
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i], nil
		}
		names = append(names, envs[i].Name)
	}
	return nil, errEnvironmentNotFound(name, blueprint, names)
}

func environmentPath(at AccountToken, environmentID string, parts ...string) string {
	return apiPath(environmentsBaseURL(), at.Account, "accounts", append([]string{"environments", environmentID}, parts...)...)
}

type environmentListEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AgentID   string `json:"agent_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	Status    string `json:"status,omitempty"`
	Variables int    `json:"variables"`
	Secrets   int    `json:"secrets"`
}

func runEnvList(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	blueprint, err := resolveBlueprintName(flagString(cmd, "blueprint"))
	if err != nil {
		return err
	}
	envs, err := listBlueprintEnvironments(cmd.Context(), at, blueprint, verbose)
	if err != nil {
		return err
	}

	agents, err := deploymentsByID(cmd.Context(), at, envs, verbose)
	if err != nil {
		return err
	}

	entries := make([]environmentListEntry, 0, len(envs))
	for _, e := range envs {
		entry := environmentListEntry{ID: e.ID, Name: e.Name, AgentID: e.DeploymentID}
		if dep, ok := agents[e.DeploymentID]; ok {
			entry.AgentName = deploymentLabel(&dep)
			entry.Status = dep.Status
		}
		vars, err := listVaultVariables(cmd.Context(), vaultScope{account: at.Account, environment: &e}, verbose)
		if err != nil {
			return err
		}
		for _, v := range vars {
			if v.Secret {
				entry.Secrets++
			} else {
				entry.Variables++
			}
		}
		entries = append(entries, entry)
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, entries)
	}
	if len(entries) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoEnvironments(blueprint), colorReset) //nolint:errcheck,gosec
		return nil
	}

	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	t := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).
		StyleFunc(func(_, col int) lipgloss.Style {
			if col == 3 {
				return lipgloss.NewStyle()
			}
			return lipgloss.NewStyle().PaddingRight(2)
		}).
		Headers(dim.Sprint("Name"), dim.Sprint("Agent"), dim.Sprint("Status"), dim.Sprint("Variables & Secrets"))
	for _, e := range entries {
		agent, status := e.AgentName, e.Status
		if e.AgentID == "" {
			agent, status = "—", "empty"
		}
		t.Row(cyan.Sprint(e.Name), agent, deploymentStatusColor(status).Sprint(status), dim.Sprint(vaultCounts(e.Variables, e.Secrets)))
	}
	fmt.Fprintln(w, t.Render()) //nolint:errcheck,gosec
	return nil
}

// deploymentsByID loads the account's deployments only when an environment
// has an agent to name.
func deploymentsByID(ctx context.Context, at AccountToken, envs []blueprintEnvironment, verbose bool) (map[string]agentDeployment, error) {
	byID := map[string]agentDeployment{}
	needed := false
	for _, e := range envs {
		needed = needed || e.DeploymentID != ""
	}
	if !needed {
		return byID, nil
	}
	result, err := listAccountDeployments(ctx, at, verbose)
	if err != nil {
		return nil, err
	}
	for _, d := range result.Deployments {
		byID[d.ID] = d
	}
	return byID, nil
}

func vaultCounts(variables, secrets int) string {
	if variables == 0 && secrets == 0 {
		return "none"
	}
	parts := []string{}
	if variables > 0 {
		parts = append(parts, plural(variables, "variable"))
	}
	if secrets > 0 {
		parts = append(parts, plural(secrets, "secret"))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func runEnvCreate(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	blueprint, err := resolveBlueprintName(flagString(cmd, "blueprint"))
	if err != nil {
		return err
	}

	var created blueprintEnvironment
	status, err := apiCall(cmd.Context(), http.MethodPost,
		apiPath(environmentsBaseURL(), at.Account, "agents", blueprint, "environments"),
		map[string]string{"name": name}, at.Token, verbose, &created)
	switch status {
	case http.StatusNotFound:
		return errBlueprintNotFound(blueprint, at.Account)
	case http.StatusConflict:
		return errEnvironmentNameTaken(name, blueprint)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	color.New(color.FgGreen).Fprint(w, "✓ ")                        //nolint:errcheck,gosec
	fmt.Fprintln(w, msgEnvironmentCreated(created.Name, blueprint)) //nolint:errcheck,gosec
	return nil
}

func runEnvRename(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name, newName := args[0], args[1]
	blueprint, err := resolveBlueprintName(flagString(cmd, "blueprint"))
	if err != nil {
		return err
	}
	env, err := findBlueprintEnvironment(cmd.Context(), at, blueprint, name, verbose)
	if err != nil {
		return err
	}

	var renamed blueprintEnvironment
	status, err := apiCallForAccount(cmd.Context(), http.MethodPatch, environmentPath(at, env.ID),
		map[string]string{"name": newName}, at.Account, verbose, &renamed)
	if status == http.StatusConflict {
		return errEnvironmentNameTaken(newName, blueprint)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	color.New(color.FgGreen).Fprint(w, "✓ ")                   //nolint:errcheck,gosec
	fmt.Fprintln(w, msgEnvironmentRenamed(name, renamed.Name)) //nolint:errcheck,gosec
	return nil
}

func runEnvDelete(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	blueprint, err := resolveBlueprintName(flagString(cmd, "blueprint"))
	if err != nil {
		return err
	}
	env, err := findBlueprintEnvironment(cmd.Context(), at, blueprint, name, verbose)
	if err != nil {
		return err
	}
	if env.DeploymentID != "" {
		return errEnvironmentHasAgent(name, blueprint)
	}

	if !confirmDelete(cmd,
		fmt.Sprintf("Delete environment %q?", name),
		"This permanently deletes its variables and secrets.",
		name) {
		return nil
	}

	status, err := apiCallForAccount(cmd.Context(), http.MethodDelete, environmentPath(at, env.ID), nil, at.Account, verbose, nil)
	if status == http.StatusConflict {
		return errEnvironmentHasAgent(name, blueprint)
	}
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	color.New(color.FgGreen).Fprint(w, "✓ ")     //nolint:errcheck,gosec
	fmt.Fprintln(w, msgEnvironmentDeleted(name)) //nolint:errcheck,gosec
	return nil
}
