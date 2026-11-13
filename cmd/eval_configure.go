package cmd

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var evalConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Show or change evaluation settings",
	Long: "With no setting flags, prints the deployment's evaluation settings. " +
		"Auto-add admits a trace to the deployment's dataset once its evaluation run completes and every evaluator " +
		"is at or above the minimum confidence. Only the flags you pass change; the rest keep their current values. " +
		"Changing a setting needs permission to run the agent's evaluations.",
	Example: "  ast eval configure --name my-agent\n" +
		"  ast eval configure --name my-agent --auto-add on --auto-add-min-confidence 0.8 --auto-add-update-on-reevaluation\n" +
		"  ast eval configure --name my-agent --auto-add off",
	Args: agentTargetArgs,
	RunE: runEvalConfigure,
}

func init() {
	evalCmd.AddCommand(evalConfigureCmd)
	registerAgentTargetFlags(evalConfigureCmd)
	evalConfigureCmd.Flags().String("auto-add", "", "Turn auto-add to the dataset on or off")
	evalConfigureCmd.Flags().String("auto-add-min-confidence", "", "Lowest evaluator confidence that adds a trace, from 0 to 1")
	evalConfigureCmd.Flags().Bool("auto-add-update-on-reevaluation", false, "Let a re-evaluation update an auto-added item (=false turns it off)")
	evalConfigureCmd.Flags().Bool("json", false, "Print raw JSON output")
}

type datasetAutoAddSettings struct {
	Enabled              bool    `json:"enabled"`
	MinConfidence        float64 `json:"min_confidence"`
	UpdateOnReevaluation bool    `json:"update_on_reevaluation"`
}

type evalConfigureChanges struct {
	autoAdd              string
	minConfidence        *float64
	updateOnReevaluation *bool
}

func (c evalConfigureChanges) any() bool {
	return c.autoAdd != "" || c.minConfidence != nil || c.updateOnReevaluation != nil
}

func (c evalConfigureChanges) settingsChanged() bool {
	return c.minConfidence != nil || c.updateOnReevaluation != nil
}

func parseMinConfidence(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || parsed < 0 || parsed > 1 {
		return 0, errEvalConfigureMinConfidence(value)
	}
	return parsed, nil
}

func parseEvalConfigureFlags(cmd *cobra.Command) (evalConfigureChanges, error) {
	var changes evalConfigureChanges
	flags := cmd.Flags()
	if flags.Changed("auto-add") {
		changes.autoAdd, _ = flags.GetString("auto-add")
		if changes.autoAdd != "on" && changes.autoAdd != "off" {
			return changes, errEvalConfigureAutoAddValue(changes.autoAdd)
		}
	}
	if flags.Changed("auto-add-min-confidence") {
		raw, _ := flags.GetString("auto-add-min-confidence")
		value, err := parseMinConfidence(raw)
		if err != nil {
			return changes, err
		}
		changes.minConfidence = &value
	}
	if flags.Changed("auto-add-update-on-reevaluation") {
		value, _ := flags.GetBool("auto-add-update-on-reevaluation")
		changes.updateOnReevaluation = &value
	}
	if changes.autoAdd == "off" && changes.settingsChanged() {
		return changes, errEvalConfigureOffWithSettings()
	}
	return changes, nil
}

func runEvalConfigure(cmd *cobra.Command, _ []string) error {
	changes, err := parseEvalConfigureFlags(cmd)
	if err != nil {
		return err
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	path := apiPath(evalBaseURL(), at.Account, "agents", dep.Name, "evaluations", "dataset-auto-add")
	query := "?" + url.Values{"deployment_id": {dep.ID}}.Encode()

	var settings datasetAutoAddSettings
	status, err := apiCall(cmd.Context(), http.MethodGet, path+query, nil, at.Token, verbose, &settings)
	if err := evalConfigureError(status, err, dep); err != nil {
		return err
	}

	switch {
	case !changes.any():
	case changes.autoAdd == "off" && !settings.Enabled:
	case changes.autoAdd == "off":
		status, err = apiCall(cmd.Context(), http.MethodDelete, path+query, nil, at.Token, verbose, &settings)
		if err := evalConfigureError(status, err, dep); err != nil {
			return err
		}
	default:
		if !settings.Enabled && changes.autoAdd != "on" {
			return errEvalConfigureAutoAddIsOff(deploymentLabel(dep))
		}
		body := map[string]any{
			"deployment_id":          dep.ID,
			"min_confidence":         settings.MinConfidence,
			"update_on_reevaluation": settings.UpdateOnReevaluation,
		}
		if changes.minConfidence != nil {
			body["min_confidence"] = *changes.minConfidence
		}
		if changes.updateOnReevaluation != nil {
			body["update_on_reevaluation"] = *changes.updateOnReevaluation
		}
		status, err = apiCall(cmd.Context(), http.MethodPut, path, body, at.Token, verbose, &settings)
		if err := evalConfigureError(status, err, dep); err != nil {
			return err
		}
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, settings)
	}
	writeEvalSettings(cmd, dep, settings)
	return nil
}

func evalConfigureError(status int, err error, dep *agentDeployment) error {
	if isConcealed(err) {
		return err
	}
	switch status {
	case http.StatusNotFound:
		return errAgentDeploymentNotFound(deploymentLabel(dep))
	case http.StatusBadRequest:
		return errEvalConfigureInvalid(apiErrorMessage(err))
	case http.StatusServiceUnavailable:
		return errEvaluationNotConfigured()
	}
	return err
}

func writeEvalSettings(cmd *cobra.Command, dep *agentDeployment, settings datasetAutoAddSettings) {
	w := cmd.OutOrStdout()
	dim := color.New(color.Faint)
	dim.Fprintf(w, "Evaluation settings for %s\n", deploymentLabel(dep)) //nolint:errcheck,gosec
	if !settings.Enabled {
		fmt.Fprintln(w, "  Auto-add to dataset:      off") //nolint:errcheck,gosec
		return
	}
	fmt.Fprintln(w, "  Auto-add to dataset:      on")                                         //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Minimum confidence:       %s\n", formatPercent(settings.MinConfidence)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Update on re-evaluation:  %s\n", onOff(settings.UpdateOnReevaluation))  //nolint:errcheck,gosec
}

func formatPercent(v float64) string {
	whole, frac, _ := strings.Cut(strconv.FormatFloat(v, 'f', -1, 64), ".")
	frac += "00"
	whole = strings.TrimLeft(whole+frac[:2], "0")
	if whole == "" {
		whole = "0"
	}
	if rest := strings.TrimRight(frac[2:], "0"); rest != "" {
		whole += "." + rest
	}
	return whole + "%"
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
