package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/theme"
)

var agentNetworkCmd = &cobra.Command{
	Use:   "network",
	Short: "Show an agent's inbound, outbound, and database network traffic",
	Args:  agentTargetArgs,
	RunE:  runAgentNetwork,
}

func init() {
	agentCmd.AddCommand(agentNetworkCmd)
	registerAgentTargetFlags(agentNetworkCmd)
	agentNetworkCmd.Flags().String("direction", "", "Also list top peers for one direction: inbound, outbound, or database")
	agentNetworkCmd.Flags().Int("limit", 10, "Number of peers to list with --direction")
	agentNetworkCmd.Flags().Bool("json", false, "Print raw JSON output")
}

var networkDirections = []string{"inbound", "outbound", "database"}

type networkDirectionSummary struct {
	RequestCount    int      `json:"request_count"`
	ErrorCount      int      `json:"error_count"`
	ErrorRate       float64  `json:"error_rate"`
	LatencyP50Ms    *float64 `json:"latency_p50_ms"`
	LatencyP95Ms    *float64 `json:"latency_p95_ms"`
	LatencyP99Ms    *float64 `json:"latency_p99_ms"`
	UniquePeerCount int      `json:"unique_peer_count"`
	BytesTotal      int64    `json:"bytes_total"`
}

type networkSummary struct {
	Inbound    networkDirectionSummary `json:"inbound"`
	Outbound   networkDirectionSummary `json:"outbound"`
	Database   networkDirectionSummary `json:"database"`
	WindowFrom string                  `json:"window_from"`
	WindowTo   string                  `json:"window_to"`
}

type networkFlow struct {
	Peer              string   `json:"peer"`
	PeerKind          string   `json:"peer_kind"`
	RequestCount      int      `json:"request_count"`
	ErrorCount        int      `json:"error_count"`
	ErrorRate         float64  `json:"error_rate"`
	LatencyP50Ms      *float64 `json:"latency_p50_ms"`
	LatencyP95Ms      *float64 `json:"latency_p95_ms"`
	BytesTotal        int64    `json:"bytes_total"`
	RegistrableDomain string   `json:"registrable_domain,omitempty"`
}

type networkFlowsResponse struct {
	Direction string        `json:"direction"`
	Flows     []networkFlow `json:"flows"`
}

func runAgentNetwork(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	direction, _ := cmd.Flags().GetString("direction")
	if direction != "" && !slices.Contains(networkDirections, direction) {
		return errUnknownNetworkDirection(direction)
	}

	u := fmt.Sprintf("%s/api/v1/deployments/%s/network/summary", agentBaseURL(), url.PathEscape(dep.ID))
	var summary networkSummary
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &summary)
	if status == http.StatusNotFound {
		return errAgentDeploymentNotFound(deploymentLabel(dep))
	}
	if err != nil {
		return err
	}

	var flows networkFlowsResponse
	if direction != "" {
		limit, _ := cmd.Flags().GetInt("limit")
		if limit <= 0 {
			return errPositiveIntFlag("limit")
		}
		fu := fmt.Sprintf("%s/api/v1/deployments/%s/network/flows?direction=%s&limit=%d",
			agentBaseURL(), url.PathEscape(dep.ID), url.QueryEscape(direction), limit)
		if _, err := apiCall(cmd.Context(), http.MethodGet, fu, nil, at.Token, verbose, &flows); err != nil {
			return err
		}
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		if direction == "" {
			return writeJSON(w, summary)
		}
		return writeJSON(w, struct {
			Summary networkSummary       `json:"summary"`
			Flows   networkFlowsResponse `json:"flows"`
		}{summary, flows})
	}

	bold := color.New(color.Bold)
	dim := color.New(color.Faint)
	accent := color.New(theme.PrimaryFatihAttr)

	bold.Fprint(w, deploymentLabel(dep)) //nolint:errcheck,gosec
	fmt.Fprint(w, "  ")                  //nolint:errcheck,gosec
	dim.Fprintf(w, "%s\n", at.Account)   //nolint:errcheck,gosec

	printNetworkDirection(w, accent, "Inbound", summary.Inbound)
	printNetworkDirection(w, accent, "Outbound", summary.Outbound)
	printNetworkDirection(w, accent, "Database", summary.Database)

	if direction == "" {
		return nil
	}
	fmt.Fprintln(w)                                  //nolint:errcheck,gosec
	accent.Fprintf(w, "  Top %s peers\n", direction) //nolint:errcheck,gosec
	if len(flows.Flows) == 0 {
		dim.Fprintf(w, "    %s\n", msgNoNetworkFlows()) //nolint:errcheck,gosec
		return nil
	}
	for _, f := range flows.Flows {
		label := f.Peer
		if f.RegistrableDomain != "" {
			label = f.RegistrableDomain
		}
		fmt.Fprintf(w, "    %-40s %8s reqs   %6s errs   p95 %s\n", //nolint:errcheck,gosec
			label, thousands(f.RequestCount), formatErrorRate(f.ErrorRate), formatLatencyMs(f.LatencyP95Ms))
	}
	return nil
}

func printNetworkDirection(w io.Writer, accent *color.Color, label string, d networkDirectionSummary) {
	accent.Fprintf(w, "  %s\n", label)                                                                   //nolint:errcheck,gosec
	fmt.Fprintf(w, "    Requests:     %s\n", thousands(d.RequestCount))                                  //nolint:errcheck,gosec
	fmt.Fprintf(w, "    Errors:       %s (%s)\n", thousands(d.ErrorCount), formatErrorRate(d.ErrorRate)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "    Latency p95:  %s\n", formatLatencyMs(d.LatencyP95Ms))                            //nolint:errcheck,gosec
	fmt.Fprintf(w, "    Peers:        %s\n", thousands(d.UniquePeerCount))                               //nolint:errcheck,gosec
	fmt.Fprintf(w, "    Bytes:        %s\n", formatBytes(d.BytesTotal))                                  //nolint:errcheck,gosec
}

func formatLatencyMs(ms *float64) string {
	if ms == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.0fms", *ms)
}

func formatErrorRate(rate float64) string {
	return fmt.Sprintf("%.1f%%", rate*100)
}
