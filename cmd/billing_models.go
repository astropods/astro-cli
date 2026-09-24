package cmd

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
)

var billingModelsCmd = &cobra.Command{
	Use:   "models [model | --features [feature]]",
	Short: "Break down the account's AI Gateway spend by model, or one model's spend by agent",
	Long: `Break down the account's AI Gateway spend by model, or one model's spend by agent.

With --features, break down instead the AI Gateway spend Astropods features
make on the account's behalf (the eval judge, the governance agent, local
dev), or one feature's spend by model.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBillingModels,
}

func init() {
	billingCmd.AddCommand(billingModelsCmd)
	billingModelsCmd.Flags().Bool("json", false, "Print raw JSON output")
	billingModelsCmd.Flags().Bool("features", false, "Break down platform-feature spend instead of model spend")
}

type modelSpend struct {
	Model   string  `json:"model"`
	CostUSD float64 `json:"cost_usd"`
}

type modelsSpendResponse struct {
	Models          []modelSpend `json:"models"`
	CostUSD         float64      `json:"cost_usd"`
	UnattributedUSD float64      `json:"unattributed_usd"`
}

type featureSpend struct {
	Feature string  `json:"feature"`
	Name    string  `json:"name"`
	CostUSD float64 `json:"cost_usd"`
}

type featuresSpendResponse struct {
	Features []featureSpend `json:"features"`
	CostUSD  float64        `json:"cost_usd"`
}

type featureByModelResponse struct {
	Feature string       `json:"feature"`
	Name    string       `json:"name"`
	Models  []modelSpend `json:"models"`
	CostUSD float64      `json:"cost_usd"`
}

type modelDeploymentSpend struct {
	DeploymentID string  `json:"deployment_id"`
	Name         string  `json:"name"`
	Deleted      bool    `json:"deleted,omitempty"`
	CostUSD      float64 `json:"cost_usd"`
}

type modelByAgentResponse struct {
	Model   string                 `json:"model"`
	Agents  []modelDeploymentSpend `json:"agents"`
	CostUSD float64                `json:"cost_usd"`
}

func runBillingModels(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	if features, _ := cmd.Flags().GetBool("features"); features {
		if len(args) == 1 {
			return runBillingFeatureByModel(cmd, at, verbose, args[0])
		}
		return runBillingFeatures(cmd, at, verbose)
	}
	if len(args) == 1 {
		return runBillingModelByAgent(cmd, at, verbose, args[0])
	}

	var resp modelsSpendResponse
	available, err := billingRead(cmd, at, verbose, "usage/models", &resp)
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
	if len(resp.Models) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoModelSpend(), colorReset) //nolint:errcheck,gosec
		return nil
	}

	// Highest spend first: that is almost always what a reader wants to see.
	models := append([]modelSpend(nil), resp.Models...)
	sort.SliceStable(models, func(i, j int) bool { return models[i].CostUSD > models[j].CostUSD })

	accent := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	accent.Fprintf(w, "%s\n", at.Account) //nolint:errcheck,gosec

	for _, m := range models {
		fmt.Fprintf(w, "  %-28s %s\n", m.Model, msgUsageDollars(m.CostUSD)) //nolint:errcheck,gosec
	}
	fmt.Fprintln(w)                                                        //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %-28s %s\n", "Total", msgUsageDollars(resp.CostUSD)) //nolint:errcheck,gosec
	if resp.UnattributedUSD != 0 {
		dim.Fprintf(w, "  %-28s %s\n", "Unattributed", msgUsageDollars(resp.UnattributedUSD)) //nolint:errcheck,gosec
	}
	dim.Fprintf(w, "\n%s\n", msgDrillIntoModel()) //nolint:errcheck,gosec
	return nil
}

func runBillingModelByAgent(cmd *cobra.Command, at AccountToken, verbose bool, model string) error {
	var resp modelByAgentResponse
	resource := fmt.Sprintf("usage/models/%s/by-agent", url.PathEscape(model))
	available, err := billingRead(cmd, at, verbose, resource, &resp)
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
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoModelAgentSpend(model), colorReset) //nolint:errcheck,gosec
		return nil
	}

	agents := append([]modelDeploymentSpend(nil), resp.Agents...)
	sort.SliceStable(agents, func(i, j int) bool { return agents[i].CostUSD > agents[j].CostUSD })

	accent := color.New(theme.PrimaryFatihAttr)
	accent.Fprintf(w, "%s: %s\n", at.Account, model) //nolint:errcheck,gosec

	for _, agent := range agents {
		name := agent.Name
		if agent.Deleted {
			name += " (deleted)"
		}
		fmt.Fprintf(w, "  %-28s %s\n", name, msgUsageDollars(agent.CostUSD)) //nolint:errcheck,gosec
	}
	fmt.Fprintln(w)                                                        //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %-28s %s\n", "Total", msgUsageDollars(resp.CostUSD)) //nolint:errcheck,gosec
	return nil
}

func runBillingFeatures(cmd *cobra.Command, at AccountToken, verbose bool) error {
	var resp featuresSpendResponse
	available, err := billingRead(cmd, at, verbose, "usage/features", &resp)
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
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgFeatureSpendUnavailable(), colorReset) //nolint:errcheck,gosec
		return nil
	}
	if len(resp.Features) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoFeatureSpend(), colorReset) //nolint:errcheck,gosec
		return nil
	}

	features := append([]featureSpend(nil), resp.Features...)
	sort.SliceStable(features, func(i, j int) bool { return features[i].CostUSD > features[j].CostUSD })

	accent := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	accent.Fprintf(w, "%s\n", at.Account) //nolint:errcheck,gosec

	// The kind is the drill-down argument, so it shows whenever the label hides it.
	for _, f := range features {
		label := f.Name
		if f.Name != f.Feature {
			label = fmt.Sprintf("%s (%s)", f.Name, f.Feature)
		}
		fmt.Fprintf(w, "  %-28s %s\n", label, msgUsageDollars(f.CostUSD)) //nolint:errcheck,gosec
	}
	fmt.Fprintln(w)                                                        //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %-28s %s\n", "Total", msgUsageDollars(resp.CostUSD)) //nolint:errcheck,gosec
	dim.Fprintf(w, "\n%s\n", msgDrillIntoFeature())                        //nolint:errcheck,gosec
	return nil
}

func runBillingFeatureByModel(cmd *cobra.Command, at AccountToken, verbose bool, feature string) error {
	var resp featureByModelResponse
	resource := fmt.Sprintf("usage/features/%s/by-model", url.PathEscape(feature))
	available, err := billingRead(cmd, at, verbose, resource, &resp)
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
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgFeatureSpendUnavailable(), colorReset) //nolint:errcheck,gosec
		return nil
	}
	if len(resp.Models) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoModelAgentSpend(feature), colorReset) //nolint:errcheck,gosec
		return nil
	}

	models := append([]modelSpend(nil), resp.Models...)
	sort.SliceStable(models, func(i, j int) bool { return models[i].CostUSD > models[j].CostUSD })

	accent := color.New(theme.PrimaryFatihAttr)
	accent.Fprintf(w, "%s: %s\n", at.Account, resp.Name) //nolint:errcheck,gosec

	for _, m := range models {
		fmt.Fprintf(w, "  %-28s %s\n", m.Model, msgUsageDollars(m.CostUSD)) //nolint:errcheck,gosec
	}
	fmt.Fprintln(w)                                                        //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %-28s %s\n", "Total", msgUsageDollars(resp.CostUSD)) //nolint:errcheck,gosec
	return nil
}

func msgDrillIntoFeature() string {
	return fmt.Sprintf("Drill into one feature: %s billing models --features <feature>", buildinfo.BinaryName)
}

func msgDrillIntoModel() string {
	return fmt.Sprintf("Drill into one model: %s billing models <model>", buildinfo.BinaryName)
}
