package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
)

// agentServerURLOverride is set in tests to redirect API calls to a test server.
var agentServerURLOverride string

func exactValidAgentDeploymentName(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("this command expected exactly one argument <agent deployment name>, but got %d", len(args))
	}
	return nil
}

func agentBaseURL() string {
	if agentServerURLOverride != "" {
		return strings.TrimSuffix(agentServerURLOverride, "/")
	}
	return strings.TrimSuffix(auth.DefaultServerURL, "/")
}

var agentCmd = &cobra.Command{
	Use:     "agent",
	Aliases: []string{"agents"},
	Short:   "Manage deployed agents",
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List deployed agents in the active account",
	RunE:  runAgentList,
}

var agentGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get details for a deployed agent",
	Args:  exactValidAgentDeploymentName,
	RunE:  runAgentGet,
}

var agentPauseCmd = &cobra.Command{
	Use:   "pause <name>",
	Short: "Pause a running agent",
	Args:  exactValidAgentDeploymentName,
	RunE:  runAgentPause,
}

var agentResumeCmd = &cobra.Command{
	Use:   "resume <name>",
	Short: "Resume a paused agent",
	Args:  exactValidAgentDeploymentName,
	RunE:  runAgentResume,
}

var agentDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a deployed agent",
	Args:  exactValidAgentDeploymentName,
	RunE:  runAgentDelete,
}

var agentHistoryCmd = &cobra.Command{
	Use:   "history <name>",
	Short: "List deployment history for an agent",
	Args:  exactValidAgentDeploymentName,
	RunE:  runAgentHistory,
}

var agentRestartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Restart a running agent",
	Args:  exactValidAgentDeploymentName,
	RunE:  runAgentRestart,
}

var agentLogsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Fetch logs for a deployed agent",
	Args:  exactValidAgentDeploymentName,
	RunE:  runAgentLogs,
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentGetCmd)
	agentCmd.AddCommand(agentPauseCmd)
	agentCmd.AddCommand(agentResumeCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	agentCmd.AddCommand(agentHistoryCmd)
	agentCmd.AddCommand(agentRestartCmd)
	agentCmd.AddCommand(agentLogsCmd)

	agentListCmd.Flags().Bool("json", false, "Print raw JSON output")
	agentGetCmd.Flags().Bool("json", false, "Print raw JSON output")
	agentPauseCmd.Flags().String("id", "", "Deployment ID (skips name lookup)")
	agentResumeCmd.Flags().String("id", "", "Deployment ID (skips name lookup)")
	agentDeleteCmd.Flags().String("confirm", "", "Skip prompt by passing the agent name as confirmation (--confirm <name>)")
	agentHistoryCmd.Flags().Bool("json", false, "Print raw JSON output")
	agentRestartCmd.Flags().String("id", "", "Deployment ID (skips name lookup)")
	agentRestartCmd.Flags().String("component", "", "Component to restart (agent)")
	agentRestartCmd.MarkFlagRequired("component") //nolint:errcheck,gosec
	agentLogsCmd.Flags().String("id", "", "Deployment ID (skips name lookup)")
	agentLogsCmd.Flags().String("container", "", "Container to fetch logs from: app or messaging")
	agentLogsCmd.MarkFlagRequired("container") //nolint:errcheck,gosec
	agentLogsCmd.Flags().BoolP("tail", "t", false, "Stream logs in real time")
}

type agentDeployment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	BuildID     string `json:"build_id"`
	Namespace   string `json:"namespace"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type listDeploymentsResponse struct {
	Deployments []agentDeployment `json:"deployments"`
	Count       int               `json:"count"`
}

type containerStatus struct {
	Name string `json:"name"`
}

type workloadDetail struct {
	Name       string            `json:"name"`
	Component  string            `json:"component"`
	PodName    string            `json:"pod_name"`
	Containers []containerStatus `json:"containers"`
}

type deploymentDetail struct {
	ID        string           `json:"id"`
	Workloads []workloadDetail `json:"workloads"`
}

type deploymentDetailResponse struct {
	Deployment deploymentDetail `json:"deployment"`
}

// resolveDeploymentID returns the deployment ID from --id or by looking up the agent name.
func resolveDeploymentID(cmd *cobra.Command, name string, at AccountToken, verbose bool) (string, error) {
	if id, _ := cmd.Flags().GetString("id"); id != "" {
		return id, nil
	}
	dep, err := findDeploymentByName(cmd, name, at, verbose)
	if err != nil {
		return "", err
	}
	return dep.ID, nil
}

func getDeploymentDetail(cmd *cobra.Command, id string, at AccountToken, verbose bool) (*deploymentDetail, error) {
	u := fmt.Sprintf("%s/api/v1/deployments/%s", agentBaseURL(), url.PathEscape(id))
	var result deploymentDetailResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	return &result.Deployment, nil
}

type deploymentHistoryRecord struct {
	ID         string `json:"id"`
	AgentName  string `json:"agent_name"`
	Revision   int    `json:"revision"`
	BuildID    string `json:"build_id"`
	Status     string `json:"status"`
	DeployedAt string `json:"deployed_at"`
}

type deploymentHistoryResponse struct {
	Deployments []deploymentHistoryRecord `json:"deployments"`
	Count       int                       `json:"count"`
}

func runAgentGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := findDeploymentByName(cmd, name, at, verbose)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, dep)
	}

	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	accent := lipgloss.NewStyle().Foreground(theme.Primary)

	deployed := dep.CreatedAt

	statusStyle := dim
	switch dep.Status {
	case "active":
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case "failed":
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	}

	fmt.Fprintln(w, bold.Render(dep.DisplayName)+"  "+dim.Render(at.Account)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Status:     %s\n", statusStyle.Render(dep.Status))      //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Build:      %s\n", accent.Render(dep.BuildID))          //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Deployed:   %s\n", deployed)                            //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Namespace:  %s\n", dep.Namespace)                       //nolint:errcheck,gosec
	fmt.Fprintf(w, "  ID:         %s\n", dim.Render(dep.ID))                  //nolint:errcheck,gosec

	detail, err := getDeploymentDetail(cmd, dep.ID, at, verbose)
	if err == nil && len(detail.Workloads) > 0 {
		fmt.Fprintf(w, "  Components:\n") //nolint:errcheck,gosec
		for _, wl := range detail.Workloads {
			component := wl.Component
			if component == "" {
				component = wl.Name
			}
			fmt.Fprintf(w, "    %s\n", dim.Render(component)) //nolint:errcheck,gosec
		}
	}
	return nil
}

// findDeploymentByName searches the full deployment list (which includes all states)
// and returns the first deployment matching the given agent name.
func findDeploymentByName(cmd *cobra.Command, name string, at AccountToken, verbose bool) (*agentDeployment, error) {
	u := agentBaseURL() + "/api/v1/deployments?account=" + url.QueryEscape(at.Account)
	var result listDeploymentsResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	for i := range result.Deployments {
		if result.Deployments[i].DisplayName == name {
			return &result.Deployments[i], nil
		}
	}
	return nil, fmt.Errorf("no deployment found for agent %q", name)
}

func runAgentList(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	u := agentBaseURL() + "/api/v1/deployments?account=" + url.QueryEscape(at.Account)
	var result listDeploymentsResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}

	if result.Count == 0 {
		fmt.Fprintf(w, "%sNo deployments found in account %s%s\n", colorDim, at.Account, colorReset) //nolint:errcheck,gosec
		return nil
	}

	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	// compute dynamic column widths from data
	statusW := len("Status")
	blueprintW := len("Blueprint")
	for _, d := range result.Deployments {
		if n := len(d.Status); n > statusW {
			statusW = n
		}
		if n := min(len(d.Name), 20); n > blueprintW {
			blueprintW = n
		}
	}

	const tableIDWidth = 11
	dim.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %-*s  %s\n", tableTimeWidth, "Deployed", tableBuildWidth, "Build", statusW, "Status", blueprintW, "Blueprint", tableIDWidth, "ID", "Name") //nolint:errcheck,gosec

	for _, d := range result.Deployments {
		deployed := truncate(d.CreatedAt, tableTimeWidth)
		buildID := truncate(d.BuildID, tableBuildWidth)
		blueprint := truncate(d.Name, blueprintW)

		dim.Fprintf(w, "%-*s  %-*s  ", tableTimeWidth, deployed, tableBuildWidth, buildID) //nolint:errcheck,gosec

		switch d.Status {
		case "active":
			green.Fprintf(w, "%-*s  ", statusW, d.Status) //nolint:errcheck,gosec
		case "failed":
			red.Fprintf(w, "%-*s  ", statusW, d.Status) //nolint:errcheck,gosec
		default:
			dim.Fprintf(w, "%-*s  ", statusW, d.Status) //nolint:errcheck,gosec
		}

		dim.Fprintf(w, "%-*s  %-*s  ", blueprintW, blueprint, tableIDWidth, d.ID) //nolint:errcheck,gosec
		cyan.Fprintf(w, "%s\n", d.DisplayName)                                    //nolint:errcheck,gosec
	}
	return nil
}

func runAgentPause(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	id, err := resolveDeploymentID(cmd, name, at, verbose)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Pausing %s%s%s\n", colorCyan, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec

	u := fmt.Sprintf("%s/api/v1/deployments/%s/stop", agentBaseURL(), url.PathEscape(id))
	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return fmt.Errorf("no deployment found for agent %q", name)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s%s%s paused\n", colorGreen, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec
	return nil
}

func runAgentResume(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	id, err := resolveDeploymentID(cmd, name, at, verbose)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Resuming %s%s%s\n", colorCyan, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec

	u := fmt.Sprintf("%s/api/v1/deployments/%s/wakeup", agentBaseURL(), url.PathEscape(id))
	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return fmt.Errorf("no deployment found for agent %q", name)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s%s%s resumed\n", colorGreen, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec
	return nil
}

func runAgentDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := findDeploymentByName(cmd, name, at, verbose)
	if err != nil {
		return err
	}

	confirm, _ := cmd.Flags().GetString("confirm")
	if confirm != "" && confirm != name {
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "%sCancelled. Confirmation does not match.%s\n", colorDim, colorReset) //nolint:errcheck,gosec
		return nil
	}
	if confirm != name {
		var confirmed bool
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Delete agent %q?", name)).
					Description("This will permanently remove the deployment and cannot be undone.").
					Value(&confirmed),
			),
		)
		huhTheme := huh.ThemeCharm()
		huhTheme.Focused.Title = huhTheme.Focused.Title.Foreground(theme.Primary)
		form.WithTheme(huhTheme)
		if err := form.Run(); err != nil || !confirmed {
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%sCancelled.%s\n", colorDim, colorReset) //nolint:errcheck,gosec
			return nil
		}
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Deleting %s%s%s\n", colorCyan, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec

	u := agentBaseURL() + "/api/v1/undeploy"
	status, err := apiCall(cmd.Context(), http.MethodPost, u, map[string]any{"deployment_id": dep.ID}, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return fmt.Errorf("no deployment found for agent %q", name)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s%s%s deleted\n", colorGreen, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec
	return nil
}

func runAgentHistory(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	u := apiPath(agentBaseURL(), at.Account, "agents", name, "deployment", "history")
	var result deploymentHistoryResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}

	if result.Count == 0 {
		fmt.Fprintf(w, "%sNo deployment history found for %s%s\n", colorDim, name, colorReset) //nolint:errcheck,gosec
		return nil
	}

	dim := color.New(color.Faint)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	dim.Fprintf(w, "%-*s  %-*s  %-4s  %s\n", tableTimeWidth, "Deployed", tableBuildWidth, "Build", "Rev", "Status") //nolint:errcheck,gosec

	for _, d := range result.Deployments {
		deployed := truncate(d.DeployedAt, tableTimeWidth)
		buildID := truncate(d.BuildID, tableBuildWidth)

		dim.Fprintf(w, "%-*s  %-*s  %-4d  ", tableTimeWidth, deployed, tableBuildWidth, buildID, d.Revision) //nolint:errcheck,gosec

		switch d.Status {
		case "active":
			green.Fprintf(w, "%s\n", d.Status) //nolint:errcheck,gosec
		case "failed":
			red.Fprintf(w, "%s\n", d.Status) //nolint:errcheck,gosec
		default:
			dim.Fprintf(w, "%s\n", d.Status) //nolint:errcheck,gosec
		}
	}
	return nil
}

func runAgentRestart(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	id, err := resolveDeploymentID(cmd, name, at, verbose)
	if err != nil {
		return err
	}

	component, _ := cmd.Flags().GetString("component")

	w := cmd.OutOrStdout()

	if component != "agent" {
		return fmt.Errorf("unknown component %q — must be: agent", component)
	}

	detail, err := getDeploymentDetail(cmd, id, at, verbose)
	if err != nil {
		return fmt.Errorf("fetching deployment detail: %w", err)
	}
	podName := ""
	for _, wl := range detail.Workloads {
		if wl.Component == component {
			podName = wl.PodName
			break
		}
	}
	if podName == "" {
		return fmt.Errorf("no pod found for component %q — use 'agent get %s' to see available components", component, name)
	}

	fmt.Fprintf(w, "%s→%s Restarting %s%s%s component of %s%s%s\n", colorCyan, colorReset, colorBold, component, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec
	u := fmt.Sprintf("%s/api/v1/deployments/%s/pods/%s/restart", agentBaseURL(), url.PathEscape(id), url.PathEscape(podName))

	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return fmt.Errorf("no deployment found for agent %q", name)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s component restarted\n", colorGreen, colorReset, component) //nolint:errcheck,gosec
	return nil
}

func runAgentLogs(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	id, err := resolveDeploymentID(cmd, name, at, verbose)
	if err != nil {
		return err
	}

	detail, err := getDeploymentDetail(cmd, id, at, verbose)
	if err != nil {
		return fmt.Errorf("fetching deployment detail: %w", err)
	}
	workload := ""
	for _, wl := range detail.Workloads {
		if wl.Component == "agent" {
			workload = wl.Name
			break
		}
	}
	if workload == "" {
		return fmt.Errorf("no agent workload found for deployment %q", id)
	}

	container, _ := cmd.Flags().GetString("container")
	switch container {
	case "app", "messaging":
	default:
		return fmt.Errorf("unknown container %q — must be one of: app, messaging", container)
	}

	since := time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339)
	tail, _ := cmd.Flags().GetBool("tail")

	w := cmd.OutOrStdout()

	if tail {
		q := url.Values{}
		q.Set("workload", workload)
		q.Set("container", container)
		q.Set("since", time.Now().UTC().Format(time.RFC3339))
		streamURL := fmt.Sprintf("%s/api/v1/deployments/%s/logs/stream?%s", agentBaseURL(), url.PathEscape(id), q.Encode())

		for {
			httpStatus, body, err := apiStream(cmd.Context(), streamURL, at.Token, verbose)
			if httpStatus == http.StatusNotFound {
				return fmt.Errorf("no deployment found for agent %q", name)
			}
			if err != nil {
				return err
			}
			scanner := bufio.NewScanner(body)
			scanner.Buffer(make([]byte, 1<<20), 1<<20)
			namedEvent := false
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					namedEvent = false
					continue
				}
				// Track named events (ready/status/heartbeat/error); their data lines carry
				// control payloads, not log entries.
				if strings.HasPrefix(line, "event: ") {
					namedEvent = true
					continue
				}
				data, ok := strings.CutPrefix(line, "data: ")
				if !ok || namedEvent {
					continue
				}
				var e logEntry
				if json.Unmarshal([]byte(data), &e) != nil {
					continue
				}
				ts, level, msg := e.parts()
				if ts != "" {
					fmt.Fprintf(w, "%s %s %s\n", ts, level, msg) //nolint:errcheck,gosec
				} else {
					fmt.Fprintf(w, "%s %s\n", level, msg) //nolint:errcheck,gosec
				}
			}
			body.Close() //nolint:errcheck,gosec
			if err := scanner.Err(); err != nil {
				return err
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case <-time.After(time.Second):
			}
		}
	}

	q := url.Values{}
	q.Set("workload", workload)
	q.Set("container", container)
	q.Set("since", since)
	batchURL := fmt.Sprintf("%s/api/v1/deployments/%s/logs?%s", agentBaseURL(), url.PathEscape(id), q.Encode())

	var raw json.RawMessage
	httpStatus, err := apiCall(cmd.Context(), http.MethodGet, batchURL, nil, at.Token, verbose, &raw)
	if httpStatus == http.StatusNotFound {
		return fmt.Errorf("no deployment found for agent %q", name)
	}
	if err != nil {
		return err
	}

	return printLogs(w, bytes.NewReader(raw))
}
