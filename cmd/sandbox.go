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
	Short: "Work with the sandboxes your dev session holds",
	Long:  "Work with the sandboxes that your 'ast dev' session for an agent holds. The session must be open: run 'ast dev' in another terminal first.",
}

var sandboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the sandboxes your dev session holds",
	Args:  cobra.NoArgs,
	RunE:  runSandboxList,
}

var sandboxShellCmd = &cobra.Command{
	Use:   "shell [name]",
	Short: "Open a shell in a sandbox your dev session holds",
	Long:  "Open an interactive root shell in a sandbox that your 'ast dev' session holds. With no name, the shell opens in the session's only sandbox. The shell ends when you run 'exit' or when the dev session closes.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSandboxShell,
}

func init() {
	for _, c := range []*cobra.Command{sandboxListCmd, sandboxShellCmd} {
		c.Flags().String("agent", "", "Agent name (defaults to the agent in the current directory's spec)")
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

func runSandboxList(cmd *cobra.Command, _ []string) error {
	w := cmd.OutOrStdout()
	agent, err := sandboxAgentName(cmd)
	if err != nil {
		return err
	}
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return errSandboxRequiresLogin(err)
	}
	sandboxes, err := listDevSandboxes(cmd, at, agent)
	if err != nil {
		return err
	}
	if len(sandboxes) == 0 {
		fmt.Fprintln(w, msgSandboxNoneYet(agent)) //nolint:errcheck
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATE") //nolint:errcheck
	for _, s := range sandboxes {
		fmt.Fprintf(tw, "%s\t%s\n", s.Name, s.State) //nolint:errcheck
	}
	return tw.Flush()
}

func runSandboxShell(cmd *cobra.Command, args []string) error {
	agent, err := sandboxAgentName(cmd)
	if err != nil {
		return err
	}
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return errSandboxRequiresLogin(err)
	}

	name := ""
	if len(args) == 1 {
		name = args[0]
	} else {
		sandboxes, err := listDevSandboxes(cmd, at, agent)
		if err != nil {
			return err
		}
		switch len(sandboxes) {
		case 0:
			return errSandboxShellNoSandbox(agent)
		case 1:
			name = sandboxes[0].Name
		default:
			names := make([]string, 0, len(sandboxes))
			for _, s := range sandboxes {
				names = append(names, s.Name)
			}
			return errSandboxShellWhich(names)
		}
	}

	u := apiPath(sandboxBaseURL(), at.Account, "accounts", "dev-sessions",
		url.PathEscape(agent), "sandboxes", url.PathEscape(name), "terminal")
	return runShell(cmd.Context(), u, at.Token, sandboxBaseURL(), cmd.InOrStdin(), cmd.OutOrStdout())
}
