package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"text/tabwriter"

	"github.com/spf13/cobra"

	spec "github.com/astropods/astro-spec"
)

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "List and open a shell in an agent's sandboxes",
	Long:  "List and open a shell in an agent's sandboxes. With --id, --env or --name, the sandboxes of that deployed agent. Without them, the sandboxes your 'ast dev' session holds, so run 'ast dev' in another terminal first.",
}

var sandboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List a deployed agent's or your dev session's sandboxes",
	Args:  cobra.NoArgs,
	RunE:  runSandboxList,
}

var sandboxShellCmd = &cobra.Command{
	Use:   "shell [name]",
	Short: "Open a shell in a deployed agent's or your dev session's sandbox",
	Long:  "Open an interactive root shell in a sandbox. With --id, --env or --name, a deployed agent's sandbox, which needs operate access to the agent. Without them, a sandbox your 'ast dev' session holds. With no name, the shell opens in the only sandbox. It ends when you run 'exit'. Piped input runs as commands, and the shell exits when the input ends: echo 'ls' | ast sandbox shell",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSandboxShell,
}

func init() {
	for _, c := range []*cobra.Command{sandboxListCmd, sandboxShellCmd} {
		c.Flags().String("agent", "", "Agent of your dev session (defaults to the agent in the current directory's spec)")
		addAgentTargetFlags(c)
		c.MarkFlagsMutuallyExclusive("agent", "name", "id", "env") //nolint:errcheck,gosec
		sandboxCmd.AddCommand(c)
	}
	rootCmd.AddCommand(sandboxCmd)
}

type devSandbox struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type devSandboxList struct {
	Sandboxes []devSandbox `json:"sandboxes"`
}

func sandboxAgentName(cmd *cobra.Command) (string, error) {
	if name, _ := cmd.Flags().GetString("agent"); name != "" {
		return name, nil
	}
	specPath, err := resolveSpecPathFromCwd("")
	if err != nil {
		return "", errSandboxAgentUnknown(err)
	}
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return "", errSandboxAgentUnknown(err)
	}
	if name := specAgentName(astroSpec); name != "" {
		return name, nil
	}
	return "", errSandboxAgentUnknown(nil)
}

func listDevSandboxes(cmd *cobra.Command, at AccountToken, agent string) ([]devSandbox, error) {
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	u := apiPath(sandboxBaseURL(), at.Account, "accounts", "dev-sessions", url.PathEscape(agent), "sandboxes")
	var resp devSandboxList
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &resp)
	if status == http.StatusNotFound {
		return nil, errSandboxNoDevSession(agent)
	}
	if err != nil {
		return nil, errSandboxListFailed(err)
	}
	return resp.Sandboxes, nil
}

// sandboxTarget is whose sandboxes a sandbox command works on: a deployment's,
// or the caller's own dev session's for an agent.
type sandboxTarget struct {
	deployment *agentDeployment
	agent      string
}

func (t sandboxTarget) label() string {
	if t.deployment != nil {
		return deploymentLabel(t.deployment)
	}
	return t.agent
}

func targetsDeployment(cmd *cobra.Command) bool {
	return flagString(cmd, "id") != "" || flagString(cmd, "env") != "" || flagString(cmd, "name") != ""
}

func resolveSandboxTarget(cmd *cobra.Command, at AccountToken) (sandboxTarget, error) {
	if targetsDeployment(cmd) {
		verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
		dep, err := resolveAgentTarget(cmd, at, verbose)
		if err != nil {
			return sandboxTarget{}, err
		}
		return sandboxTarget{deployment: dep}, nil
	}
	agent, err := sandboxAgentName(cmd)
	if err != nil {
		return sandboxTarget{}, err
	}
	return sandboxTarget{agent: agent}, nil
}

type deploymentSandboxesResponse struct {
	Runtime struct {
		Sandboxes *struct {
			Total  int          `json:"total"`
			Recent []devSandbox `json:"recent"`
		} `json:"sandboxes"`
	} `json:"runtime"`
}

func listDeploymentSandboxes(cmd *cobra.Command, at AccountToken, dep *agentDeployment) ([]devSandbox, int, error) {
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	u := fmt.Sprintf("%s/api/v1/deployments/%s/runtime", agentBaseURL(), url.PathEscape(dep.ID))
	var resp deploymentSandboxesResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &resp); err != nil {
		return nil, 0, errSandboxListFailed(err)
	}
	if resp.Runtime.Sandboxes == nil {
		return nil, 0, nil
	}
	return resp.Runtime.Sandboxes.Recent, resp.Runtime.Sandboxes.Total, nil
}

func listTargetSandboxes(cmd *cobra.Command, at AccountToken, t sandboxTarget) ([]devSandbox, int, error) {
	if t.deployment != nil {
		return listDeploymentSandboxes(cmd, at, t.deployment)
	}
	sandboxes, err := listDevSandboxes(cmd, at, t.agent)
	return sandboxes, len(sandboxes), err
}

func runSandboxList(cmd *cobra.Command, _ []string) error {
	w := cmd.OutOrStdout()
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return errSandboxRequiresLogin(err)
	}
	target, err := resolveSandboxTarget(cmd, at)
	if err != nil {
		return err
	}
	sandboxes, total, err := listTargetSandboxes(cmd, at, target)
	if err != nil {
		return err
	}
	if len(sandboxes) == 0 {
		fmt.Fprintln(w, msgSandboxNoneYet(target.label())) //nolint:errcheck
		return nil
	}
	defer func() {
		if total > len(sandboxes) {
			fmt.Fprintln(w, msgSandboxListTruncated(len(sandboxes), total)) //nolint:errcheck
		}
	}()
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATE") //nolint:errcheck
	for _, s := range sandboxes {
		fmt.Fprintf(tw, "%s\t%s\n", s.Name, s.State) //nolint:errcheck
	}
	return tw.Flush()
}

func runSandboxShell(cmd *cobra.Command, args []string) error {
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return errSandboxRequiresLogin(err)
	}
	target, err := resolveSandboxTarget(cmd, at)
	if err != nil {
		return err
	}

	name := ""
	if len(args) == 1 {
		name = args[0]
	} else {
		sandboxes, _, err := listTargetSandboxes(cmd, at, target)
		if err != nil {
			return err
		}
		open := openSandboxes(sandboxes)
		switch len(open) {
		case 0:
			return errSandboxShellNoSandbox(target.label())
		case 1:
			name = open[0].Name
		default:
			names := make([]string, 0, len(open))
			for _, s := range open {
				names = append(names, s.Name)
			}
			return errSandboxShellWhich(names)
		}
	}

	if target.deployment != nil {
		u := fmt.Sprintf("%s/api/v1/deployments/%s/sandboxes/%s/terminal",
			agentBaseURL(), url.PathEscape(target.deployment.ID), url.PathEscape(name))
		return runShell(cmd.Context(), u, at.Token, agentBaseURL(), errDeploymentSandboxShellNotFound(), cmd.InOrStdin(), cmd.OutOrStdout())
	}
	u := apiPath(sandboxBaseURL(), at.Account, "accounts", "dev-sessions",
		url.PathEscape(target.agent), "sandboxes", url.PathEscape(name), "terminal")
	return runShell(cmd.Context(), u, at.Token, sandboxBaseURL(), errSandboxShellNotFound(), cmd.InOrStdin(), cmd.OutOrStdout())
}

// openSandboxes drops stopped sandboxes, which a deployment's list keeps.
func openSandboxes(all []devSandbox) []devSandbox {
	var out []devSandbox
	for _, s := range all {
		if s.State != "stopped" {
			out = append(out, s)
		}
	}
	return out
}
