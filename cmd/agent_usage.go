package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/theme"
)

var agentUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show an agent's traces, model spend, and compute",
	Args:  agentTargetArgs,
	RunE:  runAgentUsage,
}

func init() {
	agentCmd.AddCommand(agentUsageCmd)
	registerAgentTargetFlags(agentUsageCmd)
	agentUsageCmd.Flags().Bool("json", false, "Print raw JSON output")
}

type deploymentUsage struct {
	TotalTraces    int       `json:"total_traces"`
	LastTraceAt    string    `json:"last_trace_at,omitempty"`
	RequestSeries  []int     `json:"request_series"`
	TokenSeries    []int     `json:"token_series"`
	CostSeries     []float64 `json:"cost_series"`
	CostUSD        float64   `json:"cost_usd"`
	ComputeSeries  []float64 `json:"compute_cu_hours_series"`
	ComputeCUHours float64   `json:"compute_cu_hours"`
}

func runAgentUsage(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/api/v1/deployments/%s/usage", agentBaseURL(), url.PathEscape(dep.ID))
	var usage deploymentUsage
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &usage)
	if status == http.StatusNotFound {
		return errAgentDeploymentNotFound(deploymentLabel(dep))
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, usage)
	}

	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	accent := lipgloss.NewStyle().Foreground(theme.Primary)

	fmt.Fprintln(w, bold.Render(deploymentLabel(dep))+"  "+dim.Render(at.Account))   //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %s\n", msgUsageWindow(len(usage.RequestSeries)))               //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Traces:    %s\n", accent.Render(thousands(usage.TotalTraces))) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Tokens:    %s\n", thousands(sumInts(usage.TokenSeries)))       //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Models:    %s\n", msgUsageDollars(usage.CostUSD))              //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Compute:   %s\n", msgUsageComputeHours(usage.ComputeCUHours))  //nolint:errcheck,gosec
	if usage.LastTraceAt != "" {
		fmt.Fprintf(w, "  %s\n", dim.Render(msgUsageLastTrace(usage.LastTraceAt))) //nolint:errcheck,gosec
	}
	return nil
}

func sumInts(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

// thousands groups digits so a token count reads at a glance.
func thousands(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	var out strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(digit)
	}
	return out.String()
}
