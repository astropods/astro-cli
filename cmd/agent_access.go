package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/auth"
)

var agentAccessCmd = &cobra.Command{
	Use:   "access",
	Short: "Show or set who can use an agent on each interface",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var agentAccessGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show who can use an agent on each interface",
	Args:  agentTargetArgs,
	RunE:  runAgentAccess,
}

var agentAccessSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set who can use an agent on the interfaces you name",
	Long: `Replaces the grants of each interface you name. Interfaces you leave out keep their grants.
Each of --web, --slack, and --custom repeats and takes one subject:
anyone, user=<user id>, org=<account name>, or audience=<audience arn>.`,
	Example: `  ast agent access set --name support-bot --web org=acme --slack anyone`,
	Args:    agentTargetArgs,
	RunE:    runAgentAccessSet,
}

var accessInterfaces = []string{"web", "slack", "custom"}

func init() {
	agentCmd.AddCommand(agentAccessCmd)
	agentAccessCmd.AddCommand(agentAccessGetCmd)
	agentAccessCmd.AddCommand(agentAccessSetCmd)

	registerAgentTargetFlags(agentAccessGetCmd)
	agentAccessGetCmd.Flags().Bool("json", false, "Print the grants as JSON")

	registerAgentTargetFlags(agentAccessSetCmd)
	agentAccessSetCmd.Flags().StringArray("web", nil, "Who can use web chat (repeatable)")
	agentAccessSetCmd.Flags().StringArray("slack", nil, "Who can use the Slack app (repeatable)")
	agentAccessSetCmd.Flags().StringArray("custom", nil, "Who can use the agent's own app (repeatable)")
	agentAccessSetCmd.Flags().Bool("json", false, "Print the saved grants as JSON")
	agentAccessSetCmd.MarkFlagsOneRequired(accessInterfaces...) //nolint:errcheck,gosec
}

// interfaceAccess is the body of GET and PUT /deployments/:id/interface-access.
// On a PUT, a nil interface keeps its grants.
type interfaceAccess struct {
	Web    *interfaceGrants `json:"web,omitempty"`
	Slack  *interfaceGrants `json:"slack,omitempty"`
	Custom *interfaceGrants `json:"custom,omitempty"`
}

type interfaceGrants struct {
	Grants []accessGrant `json:"grants"`
}

type accessGrant struct {
	Org      string `json:"org,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	Audience string `json:"audience,omitempty"`
	Anyone   bool   `json:"anyone,omitempty"`
}

func (a *interfaceAccess) byInterface() map[string]*interfaceGrants {
	return map[string]*interfaceGrants{"web": a.Web, "slack": a.Slack, "custom": a.Custom}
}

func interfaceAccessURL(deploymentID string) string {
	return fmt.Sprintf("%s/api/v1/deployments/%s/interface-access", agentBaseURL(), url.PathEscape(deploymentID))
}

func runAgentAccess(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	var access interfaceAccess
	status, err := apiCall(cmd.Context(), http.MethodGet, interfaceAccessURL(dep.ID), nil, at.Token, verbose, &access)
	if isNotFound(status, err) {
		return errAgentDeploymentNotFound(deploymentLabel(dep))
	}
	if err != nil {
		return err
	}
	return printInterfaceAccess(cmd, dep, &access)
}

func runAgentAccessSet(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	var body interfaceAccess
	for _, iface := range accessInterfaces {
		if !cmd.Flags().Changed(iface) {
			continue
		}
		values, _ := cmd.Flags().GetStringArray(iface)
		grants, err := parseAccessGrants(iface, values)
		if err != nil {
			return err
		}
		switch iface {
		case "web":
			body.Web = &interfaceGrants{Grants: grants}
		case "slack":
			body.Slack = &interfaceGrants{Grants: grants}
		case "custom":
			body.Custom = &interfaceGrants{Grants: grants}
		}
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	var saved interfaceAccess
	status, err := apiCall(cmd.Context(), http.MethodPut, interfaceAccessURL(dep.ID), body, at.Token, verbose, &saved)
	if isNotFound(status, err) {
		return errAgentDeploymentNotFound(deploymentLabel(dep))
	}
	if err != nil {
		return err
	}
	return printInterfaceAccess(cmd, dep, &saved)
}

// parseAccessGrants turns one interface's flag values into grants. The server
// accepts an org by account name and stores its ID.
func parseAccessGrants(iface string, values []string) ([]accessGrant, error) {
	grants := make([]accessGrant, 0, len(values))
	for _, raw := range values {
		subject := strings.TrimSpace(raw)
		kind, value, hasValue := strings.Cut(subject, "=")
		var g accessGrant
		switch {
		case subject == "anyone":
			g.Anyone = true
		case hasValue && kind == "user" && value != "":
			g.UserID = value
		case hasValue && kind == "audience" && value != "":
			g.Audience = value
		case hasValue && kind == "org" && value != "":
			g.Org = value
		default:
			return nil, errAccessInvalidSubject(iface, raw)
		}
		grants = append(grants, g)
	}
	return grants, nil
}

func storedAccounts(storage *auth.Storage) []auth.StoredAccount {
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return nil
	}
	return profile.Accounts
}

func printInterfaceAccess(cmd *cobra.Command, dep *agentDeployment, access *interfaceAccess) error {
	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, access)
	}

	accounts := storedAccounts(accountNewStorage())
	fmt.Fprintf(w, "%s→%s Interface access for %s%s%s\n", colorCyan, colorReset, colorBold, deploymentLabel(dep), colorReset) //nolint:errcheck,gosec
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	byInterface := access.byInterface()
	for _, iface := range accessInterfaces {
		var subjects []string
		if g := byInterface[iface]; g != nil {
			for _, grant := range g.Grants {
				subjects = append(subjects, accessSubject(grant, accounts))
			}
		}
		line := msgAccessNoGrants()
		if len(subjects) > 0 {
			line = strings.Join(subjects, ", ")
		}
		fmt.Fprintf(tw, "  %s\t%s\n", iface, line) //nolint:errcheck,gosec
	}
	return tw.Flush()
}

func accessSubject(g accessGrant, accounts []auth.StoredAccount) string {
	switch {
	case g.Anyone:
		return "anyone"
	case g.UserID != "":
		return "user=" + g.UserID
	case g.Audience != "":
		return "audience=" + g.Audience
	default:
		for _, a := range accounts {
			if a.ID == g.Org {
				return "org=" + a.Name
			}
		}
		return "org=" + g.Org
	}
}
