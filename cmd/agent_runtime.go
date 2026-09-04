package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type workloadIssue struct {
	Workload  string `json:"workload"`
	Component string `json:"component,omitempty"`
	Phase     string `json:"phase"`
	Message   string `json:"message,omitempty"`
	Title     string `json:"title,omitempty"`
	Guidance  string `json:"guidance,omitempty"`
}

type deploymentStatus struct {
	Value        string          `json:"value"`
	Reason       string          `json:"reason"`
	Details      string          `json:"details"`
	ErrorMessage string          `json:"error_message,omitempty"`
	WaitingOn    []workloadIssue `json:"waiting_on,omitempty"`
	FailedOn     []workloadIssue `json:"failed_on,omitempty"`
}

type runtimeContainer struct {
	Name         string `json:"name"`
	State        string `json:"state"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	Message      string `json:"message,omitempty"`
}

type runtimeWorkload struct {
	Name       string             `json:"name"`
	Phase      string             `json:"phase,omitempty"`
	PodName    string             `json:"pod_name,omitempty"`
	Containers []runtimeContainer `json:"containers,omitempty"`
}

type deploymentRuntime struct {
	Ready     int32             `json:"ready"`
	Replicas  int32             `json:"replicas"`
	Workloads []runtimeWorkload `json:"workloads,omitempty"`
}

type deploymentAlert struct {
	Workload    string  `json:"workload,omitempty"`
	Name        string  `json:"name"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Severity    string  `json:"severity"`
	State       string  `json:"state"`
	ActiveSince *string `json:"active_since,omitempty"`
}

type deploymentAlertsResponse struct {
	Alerts []deploymentAlert `json:"alerts"`
}

// The record is embedded so its keys stay at the top level of --json output.
type agentGetOutput struct {
	agentDeploymentFull
	Status  *deploymentStatus  `json:"status,omitempty"`
	Runtime *deploymentRuntime `json:"runtime,omitempty"`
	Alerts  []deploymentAlert  `json:"alerts,omitempty"`
}

func getDeploymentStatus(ctx context.Context, id string, at AccountToken, verbose bool) (*deploymentStatus, error) {
	u := fmt.Sprintf("%s/api/v1/deployments/%s/status", agentBaseURL(), url.PathEscape(id))
	var result deploymentStatus
	if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// The runtime endpoint wraps its payload; the status endpoint does not.
type deploymentRuntimeResponse struct {
	Runtime deploymentRuntime `json:"runtime"`
}

func getDeploymentRuntime(ctx context.Context, id string, at AccountToken, verbose bool) (*deploymentRuntime, error) {
	u := fmt.Sprintf("%s/api/v1/deployments/%s/runtime", agentBaseURL(), url.PathEscape(id))
	var result deploymentRuntimeResponse
	if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	return &result.Runtime, nil
}

// Alert state is tracked per workload, and the endpoint matches the workload
// query exactly, so a deployment-wide read has to ask per component.
func getDeploymentAlerts(ctx context.Context, id string, components []string, at AccountToken, verbose bool) ([]deploymentAlert, error) {
	var all []deploymentAlert
	var firstErr error
	for _, component := range components {
		q := url.Values{}
		q.Set("workload", component)
		u := fmt.Sprintf("%s/api/v1/deployments/%s/alerts?%s", agentBaseURL(), url.PathEscape(id), q.Encode())
		var result deploymentAlertsResponse
		if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, a := range result.Alerts {
			a.Workload = component
			all = append(all, a)
		}
	}
	if all == nil && firstErr != nil {
		return nil, firstErr
	}
	return all, nil
}

func (c runtimeContainer) healthy() bool {
	return c.Ready && c.RestartCount == 0 && (c.State == "" || c.State == "Running")
}

func (r *deploymentRuntime) workload(name string) *runtimeWorkload {
	if r == nil {
		return nil
	}
	for i := range r.Workloads {
		if r.Workloads[i].Name == name {
			return &r.Workloads[i]
		}
	}
	return nil
}

// An empty name means `agent logs` reads every container in the workload.
func (w *runtimeWorkload) container(name string) *runtimeContainer {
	if w == nil {
		return nil
	}
	for i := range w.Containers {
		if name != "" && w.Containers[i].Name != name {
			continue
		}
		if name != "" || !w.Containers[i].healthy() {
			return &w.Containers[i]
		}
	}
	return nil
}

func printStatusDetail(out io.Writer, status *deploymentStatus) {
	if status == nil || status.Value == "active" {
		return
	}
	dim := lipgloss.NewStyle().Faint(true)
	if status.Details != "" {
		fmt.Fprintf(out, "  %-11s %s\n", "Reason:", dim.Render(status.Details)) //nolint:errcheck,gosec
	}
	issues := append(append([]workloadIssue{}, status.FailedOn...), status.WaitingOn...)
	for _, issue := range issues {
		fmt.Fprintf(out, "    %s\n", dim.Render(msgWorkloadIssueLine(issue.Workload, issue.Component, issue.Phase, issue.Message))) //nolint:errcheck,gosec
	}
}

func printContainerStates(out io.Writer, workloadName string, runtime *deploymentRuntime) {
	wl := runtime.workload(workloadName)
	if wl == nil {
		return
	}
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	for _, c := range wl.Containers {
		if c.healthy() {
			continue
		}
		line := msgContainerStateLine(c.Name, c.State, c.RestartCount, c.Message)
		fmt.Fprintf(out, "      %s\n", warn.Render(line)) //nolint:errcheck,gosec
	}
}

func printAlerts(out io.Writer, alerts []deploymentAlert) {
	firing := make([]deploymentAlert, 0, len(alerts))
	for _, a := range alerts {
		if a.State != "" && a.State != "ok" {
			firing = append(firing, a)
		}
	}
	if len(firing) == 0 {
		return
	}
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	fmt.Fprintf(out, "  Alerts:\n") //nolint:errcheck,gosec
	for _, a := range firing {
		style := warn
		if a.Severity == "critical" {
			style = red
		}
		since := ""
		if a.ActiveSince != nil {
			since = *a.ActiveSince
		}
		fmt.Fprintf(out, "    %s\n", style.Render(msgAlertLine(a.Severity, a.Title, a.Workload, a.State, since))) //nolint:errcheck,gosec
		if a.Description != "" {
			fmt.Fprintf(out, "      %s\n", lipgloss.NewStyle().Faint(true).Render(a.Description)) //nolint:errcheck,gosec
		}
	}
}

func warnIfRestarting(out io.Writer, cmd *cobra.Command, at AccountToken, verbose bool, deploymentID, workload, container string) {
	runtime, err := getDeploymentRuntime(cmd.Context(), deploymentID, at, verbose)
	if err != nil {
		return
	}
	c := runtime.workload(workload).container(container)
	if c == nil || c.healthy() {
		return
	}
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	fmt.Fprintf(out, "%s\n", yellow.Render(msgContainerRestartWarning(c.Name, c.State, c.RestartCount, c.Message))) //nolint:errcheck,gosec
}

func workloadComponents(workloads []workloadDetail) []string {
	seen := make(map[string]bool, len(workloads))
	components := make([]string, 0, len(workloads))
	for _, wl := range workloads {
		component := wl.Component
		if component == "" {
			component = wl.Name
		}
		if component == "" || seen[component] {
			continue
		}
		seen[component] = true
		components = append(components, component)
	}
	return components
}
