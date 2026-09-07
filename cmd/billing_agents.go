package cmd

import (
	"fmt"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/theme"
)

var billingAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Break down the account's compute spend by agent",
	Args:  cobra.NoArgs,
	RunE:  runBillingAgents,
}

func init() {
	billingCmd.AddCommand(billingAgentsCmd)
	billingAgentsCmd.Flags().Bool("json", false, "Print raw JSON output")
}

type computeAgentSpend struct {
	DeploymentID string  `json:"deployment_id"`
	Name         string  `json:"name"`
	Deleted      bool    `json:"deleted,omitempty"`
	CUHours      float64 `json:"cu_hours"`
	CostUSD      float64 `json:"cost_usd"`
}

type computeByAgentResponse struct {
	Agents          []computeAgentSpend `json:"agents"`
	CUHours         float64             `json:"cu_hours"`
	CostUSD         float64             `json:"cost_usd"`
	UnattributedUSD float64             `json:"unattributed_usd"`
}

func runBillingAgents(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	var resp computeByAgentResponse
	available, err := billingRead(cmd, at, verbose, "usage/compute-by-agent", &resp)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		if !available {
			return writeJSON(w, billingEnvelope{Available: false})
		}
		return writeJSON(w, resp)
	}
	if !available {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgBillingUnavailable(), colorReset) //nolint:errcheck,gosec
		return nil
	}
	if len(resp.Agents) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoAgentSpend(), colorReset) //nolint:errcheck,gosec
		return nil
	}

	// Highest spend first: that is almost always what a reader wants to see.
	agents := append([]computeAgentSpend(nil), resp.Agents...)
	sort.SliceStable(agents, func(i, j int) bool { return agents[i].CostUSD > agents[j].CostUSD })

	accent := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	accent.Fprintf(w, "%s\n", at.Account) //nolint:errcheck,gosec

	for _, agent := range agents {
		name := agent.Name
		if agent.Deleted {
			name += " (deleted)"
		}
		fmt.Fprintf(w, "  %-28s %14s   %s\n", name, msgUsageComputeHours(agent.CUHours), msgUsageDollars(agent.CostUSD)) //nolint:errcheck,gosec
	}
	fmt.Fprintln(w)                                                                                                   //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %-28s %14s   %s\n", "Total", msgUsageComputeHours(resp.CUHours), msgUsageDollars(resp.CostUSD)) //nolint:errcheck,gosec
	if resp.UnattributedUSD != 0 {
		dim.Fprintf(w, "  %-28s %14s   %s\n", "Unattributed", "", msgUsageDollars(resp.UnattributedUSD)) //nolint:errcheck,gosec
	}
	return nil
}
