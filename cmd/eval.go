package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	spec "github.com/astropods/astro-spec"
	evalspec "github.com/astropods/astro-spec/eval"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

// evaluationFilenameAliases are filenames checked in order when discovering
// the evaluation document, mirroring SpecFileAliases.
var evaluationFilenameAliases = []string{"EVALUATION.yaml", "EVALUATION.yml"}

func resolveEvaluationPath(workingDir string) (string, error) {
	for _, name := range evaluationFilenameAliases {
		path := filepath.Join(workingDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", errNoEvaluationFile()
}

// evalServerURLOverride is set in tests to redirect API calls to a test server.
var evalServerURLOverride string

func evalBaseURL() string {
	if evalServerURLOverride != "" {
		return strings.TrimSuffix(evalServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Manage an agent's custom evaluators",
}

var evalPushCmd = &cobra.Command{
	Use:   "push [name]",
	Short: "Activate the agent's EVALUATION.yaml as its evaluation set",
	Long:  "Reads EVALUATION.yaml (or EVALUATION.yml) beside astropods.yml and activates it as the agent's evaluation set on the server. This does not build or push a container image.",
	Args:  optionalValidAgentName,
	RunE:  runEvalPush,
}

var evalValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate EVALUATION.yaml without activating it",
	Long:  "Reads EVALUATION.yaml (or EVALUATION.yml) beside astropods.yml and validates it locally, without contacting the server.",
	Args:  cobra.NoArgs,
	RunE:  runEvalValidate,
}

var evalGetCmd = &cobra.Command{
	Use:   "get <blueprint>",
	Short: "Show a blueprint's active evaluators",
	Long:  "Prints the evaluators in the blueprint's active evaluation set: key, label, type, and accepted values.",
	Args:  exactValidAgentName,
	RunE:  runEvalGet,
}

var evalStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show evaluation progress for a deployment",
	Long:  "Prints how many of a deployment's traces are queued, in progress, completed, failed, and outdated.",
	Args:  agentTargetArgs,
	RunE:  runEvalStatus,
}

var evalRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Queue evaluations for a deployment's traces",
	Long: "Queues up to 50 recent traces that have not been evaluated or whose last run failed. " +
		"With --include-outdated, traces evaluated against an older evaluation set also qualify. " +
		"With --trace-id, queues that one trace even if it already has a current result. " +
		"Runs finish in the background: check progress with `eval status`.",
	Args: agentTargetArgs,
	RunE: runEvalRun,
}

func init() {
	rootCmd.AddCommand(evalCmd)
	evalCmd.AddCommand(evalPushCmd)
	evalCmd.AddCommand(evalValidateCmd)
	evalCmd.AddCommand(evalGetCmd)
	evalCmd.AddCommand(evalStatusCmd)
	evalCmd.AddCommand(evalRunCmd)
	evalPushCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	evalValidateCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	evalGetCmd.Flags().Bool("json", false, "Print raw JSON output")
	registerAgentTargetFlags(evalStatusCmd)
	evalStatusCmd.Flags().Bool("json", false, "Print raw JSON output")
	registerAgentTargetFlags(evalRunCmd)
	evalRunCmd.Flags().StringP("trace-id", "t", "", "Evaluate this one trace")
	evalRunCmd.Flags().Bool("include-outdated", false, "Also queue traces evaluated against an older evaluation set")
	evalRunCmd.Flags().Bool("json", false, "Print raw JSON output")
}

type evalSetEvaluator struct {
	Key         string          `json:"key"`
	Label       string          `json:"label"`
	Description string          `json:"description,omitempty"`
	Type        string          `json:"type"`
	Output      evalspec.Output `json:"output"`
}

type evalSetResponse struct {
	EvaluationRef string             `json:"evaluation_ref"`
	Evaluators    []evalSetEvaluator `json:"evaluators"`
}

type evalSummaryResponse struct {
	Queued        int `json:"queued"`
	InProgress    int `json:"in_progress"`
	Completed     int `json:"completed"`
	Failed        int `json:"failed"`
	OutdatedCount int `json:"outdated_count"`
}

func runEvalPush(cmd *cobra.Command, args []string) error {
	specPath, err := resolveSpecPathFromCwd(flagString(cmd, "file"))
	if err != nil {
		return err
	}

	name, err := resolveEvalAgentName(specPath, args)
	if err != nil {
		return err
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	workingDir := filepath.Dir(specPath)
	evaluationYAML, promptFiles, err := loadEvaluationDocument(workingDir)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Activating evaluation set for %s%s%s\n", //nolint:errcheck,gosec
		colorCyan, colorReset, colorBold, name, colorReset)

	u := apiPath(evalBaseURL(), at.Account, "agents", name, "evaluation-set")
	var resp struct {
		EvaluationRef string `json:"evaluation_ref"`
	}
	status, err := apiCall(cmd.Context(), http.MethodPut, u, map[string]any{
		"evaluation_yaml": evaluationYAML,
		"prompt_files":    promptFiles,
	}, at.Token, verbose, &resp)
	if status == http.StatusNotFound {
		return fmt.Errorf("agent %q not found in account %q", name, at.Account)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s activated %s%s%s\n", //nolint:errcheck,gosec
		colorGreen, colorReset, colorDim, resp.EvaluationRef, colorReset)
	return nil
}

func runEvalValidate(cmd *cobra.Command, _ []string) error {
	specPath, err := resolveSpecPathFromCwd(flagString(cmd, "file"))
	if err != nil {
		return err
	}

	workingDir := filepath.Dir(specPath)
	evaluationYAML, promptFiles, err := loadEvaluationDocument(workingDir)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Validating evaluation set\n", //nolint:errcheck,gosec
		colorCyan, colorReset)

	result, err := evalspec.Parse(evaluationYAML, promptFiles)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s valid %s%s%s\n", //nolint:errcheck,gosec
		colorGreen, colorReset, colorDim, result.EvaluationRef, colorReset)
	return nil
}

func runEvalGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	u := apiPath(evalBaseURL(), at.Account, "agents", name, "evaluation-set")
	var set evalSetResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &set)
	if status == http.StatusNotFound {
		return errEvalSetNotFound(name, at.Account)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, set)
	}

	if len(set.Evaluators) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoEvaluators(name), colorReset) //nolint:errcheck,gosec
		return nil
	}

	dim := color.New(color.Faint)
	dim.Fprintf(w, "Evaluation set %s\n\n", set.EvaluationRef) //nolint:errcheck,gosec
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tLABEL\tTYPE\tACCEPTS") //nolint:errcheck,gosec
	for _, e := range set.Evaluators {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Key, e.Label, e.Type, describeEvalOutput(e.Output)) //nolint:errcheck,gosec
	}
	return tw.Flush()
}

func describeEvalOutput(o evalspec.Output) string {
	switch o.Type {
	case evalspec.OutputBoolean:
		return "true, false"
	case evalspec.OutputEnum:
		return strings.Join(o.Options, ", ")
	case evalspec.OutputNumber:
		switch {
		case o.Minimum != nil && o.Maximum != nil:
			return fmt.Sprintf("%g to %g", *o.Minimum, *o.Maximum)
		case o.Minimum != nil:
			return fmt.Sprintf("%g or more", *o.Minimum)
		case o.Maximum != nil:
			return fmt.Sprintf("%g or less", *o.Maximum)
		}
		return "any number"
	case evalspec.OutputString:
		if o.MaxLength != nil {
			return fmt.Sprintf("text, up to %d characters", *o.MaxLength)
		}
		return "text"
	}
	return string(o.Type)
}

func runEvalStatus(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/api/v1/deployments/%s/evaluations/summary", agentBaseURL(), url.PathEscape(dep.ID))
	var sum evalSummaryResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &sum)
	switch status {
	case http.StatusNotFound:
		return errAgentDeploymentNotFound(deploymentLabel(dep))
	case http.StatusServiceUnavailable:
		return errEvaluationNotConfigured()
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, sum)
	}

	dim := color.New(color.Faint)
	dim.Fprintf(w, "Evaluations for %s\n", deploymentLabel(dep)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Queued:       %d\n", sum.Queued)           //nolint:errcheck,gosec
	fmt.Fprintf(w, "  In progress:  %d\n", sum.InProgress)       //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Completed:    %d\n", sum.Completed)        //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Failed:       %d\n", sum.Failed)           //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Outdated:     %d\n", sum.OutdatedCount)    //nolint:errcheck,gosec
	return nil
}

type evalRunBatchResponse struct {
	EnqueuedTraceIDs []string `json:"enqueued_trace_ids"`
	FailedTraceIDs   []string `json:"failed_trace_ids"`
}

type evalRunTraceResponse struct {
	EvaluationRunID string `json:"evaluation_run_id"`
	Status          string `json:"status"`
}

const evalRunBatchLimit = 50

func runEvalRun(cmd *cobra.Command, _ []string) error {
	traceID, _ := cmd.Flags().GetString("trace-id")
	includeOutdated, _ := cmd.Flags().GetBool("include-outdated")
	if traceID != "" && includeOutdated {
		return errEvalRunTraceWithOutdated()
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	if traceID != "" {
		return runEvalRunTrace(cmd, dep, traceID, at, verbose)
	}
	return runEvalRunBatch(cmd, dep, includeOutdated, at, verbose)
}

func runEvalRunBatch(cmd *cobra.Command, dep *agentDeployment, includeOutdated bool, at AccountToken, verbose bool) error {
	u := apiPath(evalBaseURL(), at.Account, "agents", dep.Name, "evaluations")
	var resp evalRunBatchResponse
	status, err := apiCall(cmd.Context(), http.MethodPost, u, map[string]any{
		"deployment_id":         dep.ID,
		"include_outdated_runs": includeOutdated,
	}, at.Token, verbose, &resp)
	switch status {
	case http.StatusNotFound:
		return errAgentDeploymentNotFound(deploymentLabel(dep))
	case http.StatusServiceUnavailable:
		return errEvaluationNotConfigured()
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, resp)
	}
	fmt.Fprintln(w, msgEvalRunQueued(len(resp.EnqueuedTraceIDs), len(resp.FailedTraceIDs), evalRunBatchLimit)) //nolint:errcheck,gosec
	return nil
}

func runEvalRunTrace(cmd *cobra.Command, dep *agentDeployment, traceID string, at AccountToken, verbose bool) error {
	u := apiPath(evalBaseURL(), at.Account, "agents", dep.Name, "evaluations", traceID)
	var resp evalRunTraceResponse
	status, err := apiCall(cmd.Context(), http.MethodPost, u, map[string]any{
		"deployment_id": dep.ID,
	}, at.Token, verbose, &resp)
	switch status {
	case http.StatusNotFound:
		return errAgentTraceNotFound(traceID, deploymentLabel(dep))
	case http.StatusConflict:
		return errEvalRunAlreadyActive(traceID)
	case http.StatusServiceUnavailable:
		return errEvaluationNotConfigured()
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, resp)
	}
	fmt.Fprintln(w, msgEvalRunTraceQueued(traceID, resp.EvaluationRunID, resp.Status, dep.ID)) //nolint:errcheck,gosec
	return nil
}

func resolveEvalAgentName(specPath string, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	return specFileBlueprintName(specPath)
}

// specFileBlueprintName reads only the spec's name, so a spec that fails full
// validation still names its blueprint.
func specFileBlueprintName(specPath string) (string, error) {
	data, err := os.ReadFile(specPath) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filepath.Base(specPath), err)
	}
	var doc struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", filepath.Base(specPath), err)
	}

	_, name := spec.SplitAgentName(doc.Name)
	if name == "" {
		return "", fmt.Errorf("%s has no name field; pass the agent name explicitly", filepath.Base(specPath))
	}
	return name, nil
}

// loadEvaluationDocument reads the evaluation document from workingDir and
// resolves every referenced prompt_file into a path→contents map.
func loadEvaluationDocument(workingDir string) (evaluationYAML string, promptFiles map[string]string, err error) {
	path, err := resolveEvaluationPath(workingDir)
	if err != nil {
		return "", nil, err
	}
	data, readErr := os.ReadFile(path) //nolint:gosec
	if readErr != nil {
		return "", nil, fmt.Errorf("failed to read %s: %w", filepath.Base(path), readErr)
	}

	var doc struct {
		Evaluators []struct {
			PromptFile string `yaml:"prompt_file"`
		} `yaml:"evaluators"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", nil, fmt.Errorf("failed to parse %s: %w", filepath.Base(path), err)
	}

	promptFiles = map[string]string{}
	for _, evaluator := range doc.Evaluators {
		if evaluator.PromptFile == "" || promptFiles[evaluator.PromptFile] != "" {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(workingDir, evaluator.PromptFile)) //nolint:gosec
		if err != nil {
			return "", nil, fmt.Errorf("failed to read prompt_file %q: %w", evaluator.PromptFile, err)
		}
		promptFiles[evaluator.PromptFile] = string(contents)
	}

	return string(data), promptFiles, nil
}
