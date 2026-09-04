package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
)

// agentLogsTailLines is how many recent lines `ast agent logs` fetches. The
// server defaults to 200, which is a few seconds of a debug-level sidecar.
const agentLogsTailLines = 500

// agentServerURLOverride is set in tests to redirect API calls to a test server.
var agentServerURLOverride string

func agentBaseURL() string {
	if agentServerURLOverride != "" {
		return strings.TrimSuffix(agentServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
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
	Use:   "get",
	Short: "Get details for a deployed agent",
	Args:  agentTargetArgs,
	RunE:  runAgentGet,
}

var agentPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause a running agent",
	Args:  agentTargetArgs,
	RunE:  runAgentPause,
}

var agentResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a paused agent",
	Args:  agentTargetArgs,
	RunE:  runAgentResume,
}

var agentDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a deployed agent",
	Args:  agentTargetArgs,
	RunE:  runAgentDelete,
}

var agentHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "List deployment history for an agent",
	Args:  agentTargetArgs,
	RunE:  runAgentHistory,
}

var agentRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart a running agent",
	Args:  agentTargetArgs,
	RunE:  runAgentRestart,
}

var agentLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Fetch logs for a deployed agent",
	Args:  agentTargetArgs,
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
	registerAgentTargetFlags(agentGetCmd)
	registerAgentTargetFlags(agentPauseCmd)
	registerAgentTargetFlags(agentResumeCmd)
	registerAgentTargetFlags(agentDeleteCmd)
	registerAgentTargetFlags(agentHistoryCmd)
	registerAgentTargetFlags(agentRestartCmd)
	registerAgentTargetFlags(agentLogsCmd)
	agentDeleteCmd.Flags().String("confirm", "", "Skip prompt by passing the agent name or ID as confirmation")
	agentHistoryCmd.Flags().Bool("json", false, "Print raw JSON output")
	agentRestartCmd.Flags().String("component", "", "Component to restart (agent)")
	agentRestartCmd.MarkFlagRequired("component") //nolint:errcheck,gosec
	agentLogsCmd.Flags().String("workload", "", "Workload to read logs from, optionally with a container suffix (workload[/container]). Workload accepts the full name (e.g. my-agent-knowledge-vectors), an entry-name suffix (e.g. vectors), or a component (agent, messaging, collector). Container picks a specific container in the pod (e.g. agent/messaging); omitting it reads logs from all containers in the workload. Defaults to the agent workload.")
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

// resolveWorkloadTarget picks a workload (and optional container) from the
// deployment detail based on a user-supplied identifier of the form
// "workload[/container]". The workload portion may be:
//   - empty: defaults to the "agent" component
//   - an exact workload name (e.g. "my-agent-knowledge-vectors")
//   - an entry-name suffix (e.g. "vectors" — matches "*-knowledge-vectors",
//     "*-model-vectors", etc.)
//   - a component label (e.g. "agent", "messaging", "knowledge", "collector")
//
// When the identifier could match multiple workloads (e.g. component "knowledge"
// with several entries), the error message lists the candidates. The optional
// container suffix is validated against the resolved workload's container list
// when that list is known; otherwise it is passed through for the server to
// validate.
func resolveWorkloadTarget(workloads []workloadDetail, requested string) (string, string, error) {
	workloadReq, container, _ := strings.Cut(requested, "/")
	available := workloadNames(workloads)

	var resolved *workloadDetail
	if workloadReq == "" {
		for i, wl := range workloads {
			if wl.Component == "agent" {
				resolved = &workloads[i]
				break
			}
		}
		if resolved == nil {
			return "", "", errNoAgentWorkload(available)
		}
	} else {
		// Exact name match.
		for i, wl := range workloads {
			if wl.Name == workloadReq {
				resolved = &workloads[i]
				break
			}
		}
		if resolved == nil {
			// Match by component or by entry-name suffix.
			matches := []workloadDetail{}
			for _, wl := range workloads {
				if wl.Component == workloadReq || strings.HasSuffix(wl.Name, "-"+workloadReq) {
					matches = append(matches, wl)
				}
			}
			switch len(matches) {
			case 1:
				resolved = &matches[0]
			case 0:
				return "", "", errWorkloadNotFound(workloadReq, available)
			default:
				names := make([]string, 0, len(matches))
				for _, wl := range matches {
					names = append(names, wl.Name)
				}
				return "", "", errWorkloadAmbiguous(workloadReq, names)
			}
		}
	}

	if container != "" && len(resolved.Containers) > 0 {
		found := false
		names := make([]string, 0, len(resolved.Containers))
		for _, c := range resolved.Containers {
			names = append(names, c.Name)
			if c.Name == container {
				found = true
			}
		}
		if !found {
			return "", "", errContainerNotInWorkload(container, resolved.Name, names)
		}
	}

	return resolved.Name, container, nil
}

func workloadNames(workloads []workloadDetail) []string {
	parts := make([]string, 0, len(workloads))
	for _, wl := range workloads {
		parts = append(parts, wl.Name)
	}
	return parts
}

type deploymentDetailResponse struct {
	Deployment deploymentDetail `json:"deployment"`
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
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	full, fullErr := getAgentDeploymentFull(cmd.Context(), dep.ID, at, verbose)

	// A failed read drops its own section instead of failing the command.
	status, _ := getDeploymentStatus(cmd.Context(), dep.ID, at, verbose)
	runtime, _ := getDeploymentRuntime(cmd.Context(), dep.ID, at, verbose)
	alerts, _ := getDeploymentAlerts(cmd.Context(), dep.ID, at, verbose)

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		if fullErr == nil {
			return writeJSON(w, agentGetOutput{
				agentDeploymentFull: *full,
				Status:              status,
				Runtime:             runtime,
				Alerts:              alerts,
			})
		}
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

	fmt.Fprintln(w, bold.Render(deploymentLabel(dep))+"  "+dim.Render(at.Account)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Status:     %s\n", statusStyle.Render(dep.Status))           //nolint:errcheck,gosec
	printStatusDetail(w, status)
	fmt.Fprintf(w, "  Build:      %s\n", accent.Render(dep.BuildID)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Deployed:   %s\n", deployed)                   //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Namespace:  %s\n", dep.Namespace)              //nolint:errcheck,gosec
	fmt.Fprintf(w, "  ID:         %s\n", dim.Render(dep.ID))         //nolint:errcheck,gosec

	if fullErr == nil {
		if messaging := messagingEndpoint(full.ExternalURLs); messaging != nil && messaging.URL != "" {
			fmt.Fprintf(w, "  %s\n", msgLaunchURLLine(messaging.URL)) //nolint:errcheck,gosec
			if messaging.Ready {
				fmt.Fprintf(w, "  %s\n", msgLaunchURLReady()) //nolint:errcheck,gosec
			} else {
				fmt.Fprintf(w, "  %s\n", msgLaunchURLPending(messaging.Message)) //nolint:errcheck,gosec
			}
		}
	}

	var workloads []workloadDetail
	if fullErr == nil {
		workloads = full.Workloads
	} else if legacy, legacyErr := getDeploymentDetail(cmd, dep.ID, at, verbose); legacyErr == nil {
		workloads = legacy.Workloads
	}
	if len(workloads) > 0 {
		fmt.Fprintf(w, "  Components:\n") //nolint:errcheck,gosec
		for _, wl := range workloads {
			component := wl.Component
			if component == "" {
				component = wl.Name
			}
			// Show workload name alongside component so users can pass it to
			// `agent logs --workload <name>` for non-agent components.
			if wl.Name != "" && wl.Name != component {
				fmt.Fprintf(w, "    %s %s\n", dim.Render(component), dim.Render("("+wl.Name+")")) //nolint:errcheck,gosec
			} else {
				fmt.Fprintf(w, "    %s\n", dim.Render(component)) //nolint:errcheck,gosec
			}
			printContainerStates(w, wl.Name, runtime)
		}
	}
	printAlerts(w, alerts)
	return nil
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
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	label := deploymentLabel(dep)

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Pausing %s%s%s\n", colorCyan, colorReset, colorBold, label, colorReset) //nolint:errcheck,gosec

	u := fmt.Sprintf("%s/api/v1/deployments/%s/stop", agentBaseURL(), url.PathEscape(dep.ID))
	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return errAgentDeploymentNotFound(label)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s%s%s paused\n", colorGreen, colorReset, colorBold, label, colorReset) //nolint:errcheck,gosec
	return nil
}

func runAgentResume(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	label := deploymentLabel(dep)

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Resuming %s%s%s\n", colorCyan, colorReset, colorBold, label, colorReset) //nolint:errcheck,gosec

	u := fmt.Sprintf("%s/api/v1/deployments/%s/wakeup", agentBaseURL(), url.PathEscape(dep.ID))
	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return errAgentDeploymentNotFound(label)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s%s%s resumed\n", colorGreen, colorReset, colorBold, label, colorReset) //nolint:errcheck,gosec
	return nil
}

func runAgentDelete(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	label := deploymentLabel(dep)

	confirm, _ := cmd.Flags().GetString("confirm")
	if confirm != "" && confirm != label && confirm != dep.ID {
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "%sCancelled. Confirmation does not match.%s\n", colorDim, colorReset) //nolint:errcheck,gosec
		return nil
	}
	if confirm != label && confirm != dep.ID {
		var confirmed bool
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Delete agent %q?", label)).
					Description("This will permanently remove the deployment and cannot be undone.").
					Value(&confirmed),
			),
		)
		if err := runForm(form); err != nil || !confirmed {
			printCancelled(cmd.OutOrStdout())
			return nil
		}
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Deleting %s%s%s\n", colorCyan, colorReset, colorBold, label, colorReset) //nolint:errcheck,gosec

	u := agentBaseURL() + "/api/v1/undeploy"
	status, err := apiCall(cmd.Context(), http.MethodPost, u, map[string]any{"deployment_id": dep.ID}, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return errAgentDeploymentNotFound(label)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s%s%s deleted\n", colorGreen, colorReset, colorBold, label, colorReset) //nolint:errcheck,gosec
	return nil
}

func runAgentHistory(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	label := deploymentLabel(dep)

	u := apiPath(agentBaseURL(), at.Account, "agents", dep.Name, "deployment", "history")
	var result deploymentHistoryResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}

	if result.Count == 0 {
		fmt.Fprintf(w, "%sNo deployment history found for %s%s\n", colorDim, label, colorReset) //nolint:errcheck,gosec
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
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	label := deploymentLabel(dep)

	component, _ := cmd.Flags().GetString("component")

	w := cmd.OutOrStdout()

	if component != "agent" {
		return fmt.Errorf("unknown component %q — must be: agent", component)
	}

	detail, err := getDeploymentDetail(cmd, dep.ID, at, verbose)
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
		return fmt.Errorf("no pod found for component %q — use 'agent get --name %s' to see available components", component, label)
	}

	fmt.Fprintf(w, "%s→%s Restarting %s%s%s component of %s%s%s\n", colorCyan, colorReset, colorBold, component, colorReset, colorBold, label, colorReset) //nolint:errcheck,gosec
	u := fmt.Sprintf("%s/api/v1/deployments/%s/pods/%s/restart", agentBaseURL(), url.PathEscape(dep.ID), url.PathEscape(podName))

	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return errAgentDeploymentNotFound(label)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s component restarted\n", colorGreen, colorReset, component) //nolint:errcheck,gosec
	return nil
}

func runAgentLogs(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	label := deploymentLabel(dep)

	detail, err := getDeploymentDetail(cmd, dep.ID, at, verbose)
	if err != nil {
		return fmt.Errorf("fetching deployment detail: %w", err)
	}
	requested, _ := cmd.Flags().GetString("workload")
	workload, container, err := resolveWorkloadTarget(detail.Workloads, requested)
	if err != nil {
		return err
	}

	since := time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339)
	tail, _ := cmd.Flags().GetBool("tail")

	w := cmd.OutOrStdout()

	warnIfRestarting(w, cmd, at, verbose, dep.ID, workload, container)

	if tail {
		q := url.Values{}
		q.Set("workload", workload)
		q.Set("container", container)
		q.Set("since", time.Now().UTC().Format(time.RFC3339))
		streamURL := fmt.Sprintf("%s/api/v1/deployments/%s/logs/stream?%s", agentBaseURL(), url.PathEscape(dep.ID), q.Encode())

		for {
			httpStatus, body, err := apiStream(cmd.Context(), streamURL, at.Token, verbose)
			if httpStatus == http.StatusNotFound {
				return errAgentDeploymentNotFound(label)
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
	// Page backwards from now: the server's default direction is forward, which
	// returns the *oldest* lines in the window. On a chatty container that hides
	// whatever just happened, which is the reason to run this command at all.
	q.Set("direction", "backward")
	q.Set("tailLines", strconv.Itoa(agentLogsTailLines))
	batchURL := fmt.Sprintf("%s/api/v1/deployments/%s/logs?%s", agentBaseURL(), url.PathEscape(dep.ID), q.Encode())

	var raw json.RawMessage
	httpStatus, err := apiCall(cmd.Context(), http.MethodGet, batchURL, nil, at.Token, verbose, &raw)
	if httpStatus == http.StatusNotFound {
		return errAgentDeploymentNotFound(label)
	}
	if err != nil {
		return err
	}

	return printLogs(w, bytes.NewReader(raw))
}
