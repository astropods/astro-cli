package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
)

// datasetServerURLOverride is set in tests to redirect API calls to a test server.
var datasetServerURLOverride string

func datasetBaseURL() string {
	if datasetServerURLOverride != "" {
		return strings.TrimSuffix(datasetServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

// datasetMaxLimit is the server's page-size cap; a larger --limit is clamped there.
const datasetMaxLimit = 100

var datasetCmd = &cobra.Command{
	Use:   "dataset",
	Short: "Manage an agent's evaluation datasets",
}

var datasetGetCmd = &cobra.Command{
	Use:   "get <dataset-name>",
	Short: "Show a dataset's item count and evaluator values",
	Long:  "Prints the dataset's item count and, for each evaluator, how many items have each value. Run `dataset list` to see dataset names.",
	Args:  cobra.ExactArgs(1),
	RunE:  runDatasetGet,
}

var datasetItemsCmd = &cobra.Command{
	Use:   "items <dataset-name>",
	Short: "List a dataset's items",
	Long: "Lists a page of the dataset's items with their input, expected output, and evaluator values. " +
		"--offset must be a multiple of --limit. Use --json for the full values.",
	Args: cobra.ExactArgs(1),
	RunE: runDatasetItems,
}

var datasetAddCmd = &cobra.Command{
	Use:   "add <dataset-name>",
	Short: "Add a trace to a dataset",
	Long:  "Adds the trace to the dataset as an item, with its current evaluator values. The trace must belong to the dataset's deployment and have an input.",
	Args:  cobra.ExactArgs(1),
	RunE:  runDatasetAdd,
}

var datasetEditCmd = &cobra.Command{
	Use:   "edit <dataset-name>",
	Short: "Replace a dataset item's evaluator values",
	Long: "Replaces the evaluator values on the dataset item for a trace. Evaluators you leave out are cleared. " +
		"Run `eval get <blueprint>` to see keys and accepted values. Use --set-string for a string that looks like a number or a boolean.",
	Example: "  ast dataset edit eval-dep12345678 -t <trace-id> --set helpful=true --set tone=warm",
	Args:    cobra.ExactArgs(1),
	RunE:    runDatasetEdit,
}

var datasetRemoveCmd = &cobra.Command{
	Use:   "remove <dataset-name>",
	Short: "Remove a trace from a dataset",
	Long: "Removes the trace's item and its evaluator values from the dataset. The trace and its evaluations are kept. " +
		"Asks for confirmation, or pass --confirm <trace-id> to skip the prompt.",
	Args: cobra.ExactArgs(1),
	RunE: runDatasetRemove,
}

var datasetDownloadCmd = &cobra.Command{
	Use:   "download <dataset-name>",
	Short: "Download a dataset as a zip of JSONL",
	Long: "Saves the dataset as a zip file containing one JSONL file of its items. " +
		"Writes <dataset-name>.zip to the current directory, or to --output. Use --output - to write the zip to stdout.",
	Args: cobra.ExactArgs(1),
	RunE: runDatasetDownload,
}

var datasetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the account's datasets",
	Long:  "Lists the account's datasets, newest first, with the blueprint and deployment each one belongs to.",
	Args:  cobra.NoArgs,
	RunE:  runDatasetList,
}

func init() {
	rootCmd.AddCommand(datasetCmd)
	datasetCmd.AddCommand(datasetListCmd)
	datasetCmd.AddCommand(datasetGetCmd)
	datasetGetCmd.Flags().Bool("json", false, "Print raw JSON output")
	datasetCmd.AddCommand(datasetDownloadCmd)
	datasetDownloadCmd.Flags().StringP("output", "o", "", "File or directory to write to, or - for stdout (default: <dataset-name>.zip)")
	datasetCmd.AddCommand(datasetRemoveCmd)
	datasetRemoveCmd.Flags().StringP("trace-id", "t", "", "Trace to remove (required)")
	datasetRemoveCmd.Flags().String("confirm", "", "Skip the prompt by passing the trace ID as confirmation")
	datasetRemoveCmd.Flags().Bool("json", false, "Print raw JSON output")
	datasetCmd.AddCommand(datasetEditCmd)
	datasetEditCmd.Flags().StringP("trace-id", "t", "", "Trace whose dataset item to edit (required)")
	registerEvalValueFlags(datasetEditCmd)
	datasetEditCmd.Flags().Bool("json", false, "Print raw JSON output")
	datasetCmd.AddCommand(datasetAddCmd)
	datasetAddCmd.Flags().StringP("trace-id", "t", "", "Trace to add (required)")
	datasetAddCmd.Flags().Bool("json", false, "Print raw JSON output")
	datasetCmd.AddCommand(datasetItemsCmd)
	datasetItemsCmd.Flags().Int("limit", 50, "Number of items to list (max 100)")
	datasetItemsCmd.Flags().Int("offset", 0, "Pagination offset, a multiple of --limit")
	datasetItemsCmd.Flags().Bool("json", false, "Print raw JSON output")
	datasetListCmd.Flags().Int("limit", 50, "Number of datasets to list (max 100)")
	datasetListCmd.Flags().Int("offset", 0, "Pagination offset for list")
	datasetListCmd.Flags().Bool("json", false, "Print raw JSON output")
}

type datasetListItem struct {
	ID           string `json:"id"`
	DatasetName  string `json:"dataset_name"`
	DeploymentID string `json:"deployment_id"`
	AgentName    string `json:"agent_name"`
	CreatedAt    string `json:"created_at"`
}

type datasetListResponse struct {
	Datasets []datasetListItem `json:"datasets"`
	Total    int               `json:"total"`
}

func runDatasetList(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	if err := validateListPagination(limit, offset); err != nil {
		return err
	}
	if limit > datasetMaxLimit {
		return errDatasetLimit(datasetMaxLimit)
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("offset", fmt.Sprintf("%d", offset))
	u := apiPath(datasetBaseURL(), at.Account, "accounts", "datasets") + "?" + q.Encode()
	var result datasetListResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}
	if len(result.Datasets) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoDatasets(), colorReset) //nolint:errcheck,gosec
		return nil
	}

	rows := make([][]string, len(result.Datasets))
	for i, ds := range result.Datasets {
		rows[i] = []string{ds.DatasetName, ds.AgentName, ds.DeploymentID}
	}
	writeTable(w, []string{"Name", "Blueprint", "Deployment"}, rows)

	if result.Total > offset+len(result.Datasets) {
		fmt.Fprintf(w, "%s\nShowing %d–%d of %d. Page with --offset %d.%s\n", colorDim, //nolint:errcheck,gosec
			offset+1, offset+len(result.Datasets), result.Total, offset+limit, colorReset)
	}
	return nil
}

type datasetValueCount struct {
	Value json.RawMessage `json:"value"`
	Count int             `json:"count"`
}

type datasetEvaluatorSummary struct {
	Key          string              `json:"key"`
	Label        string              `json:"label"`
	Distribution []datasetValueCount `json:"distribution"`
}

type datasetSummaryResponse struct {
	ID          string                    `json:"id"`
	DatasetName string                    `json:"dataset_name"`
	ItemCount   int                       `json:"item_count"`
	Evaluators  []datasetEvaluatorSummary `json:"evaluators"`
}

// resolveDataset finds the account's dataset with this exact name.
func resolveDataset(cmd *cobra.Command, at AccountToken, verbose bool, name string) (*datasetListItem, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("limit", "2")
	u := apiPath(datasetBaseURL(), at.Account, "accounts", "datasets") + "?" + q.Encode()
	var result datasetListResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	switch len(result.Datasets) {
	case 0:
		return nil, errDatasetNotFound(name)
	case 1:
		return &result.Datasets[0], nil
	}
	ids := make([]string, len(result.Datasets))
	for i, d := range result.Datasets {
		ids[i] = d.ID
	}
	return nil, errDatasetAmbiguous(name, ids)
}

func runDatasetGet(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	dataset, err := resolveDataset(cmd, at, verbose, name)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/api/v1/datasets/%s", datasetBaseURL(), url.PathEscape(dataset.ID))
	var summary datasetSummaryResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &summary)
	if status == http.StatusNotFound {
		return errDatasetNotFound(name)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, summary)
	}

	accent := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	accent.Fprintf(w, "%s\n", summary.DatasetName)             //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Blueprint:  %s\n", dataset.AgentName)    //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Deployment: %s\n", dataset.DeploymentID) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Items:      %d\n", summary.ItemCount)    //nolint:errcheck,gosec
	for _, e := range summary.Evaluators {
		dim.Fprintf(w, "\n%s (%s)\n", e.Label, e.Key) //nolint:errcheck,gosec
		if len(e.Distribution) == 0 {
			fmt.Fprintln(w, "  No values yet") //nolint:errcheck,gosec
			continue
		}
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, v := range e.Distribution {
			fmt.Fprintf(tw, "  %s\t%d\n", rawValue(v.Value), v.Count) //nolint:errcheck,gosec
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

type datasetItemOutput struct {
	Key   string          `json:"key"`
	Label string          `json:"label"`
	Value json.RawMessage `json:"value"`
}

type datasetItem struct {
	ID               string              `json:"id"`
	Input            json.RawMessage     `json:"input"`
	ExpectedOutput   json.RawMessage     `json:"expected_output"`
	SourceTraceID    string              `json:"source_trace_id"`
	CreatedAt        string              `json:"created_at"`
	EvaluationRef    string              `json:"evaluation_ref,omitempty"`
	Outdated         bool                `json:"outdated"`
	VerifiedByUserID string              `json:"verified_by_user_id,omitempty"`
	EvaluatorOutputs []datasetItemOutput `json:"evaluator_outputs"`
}

type datasetItemsResponse struct {
	Items      []datasetItem `json:"items"`
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	TotalItems int           `json:"total_items"`
	TotalPages int           `json:"total_pages"`
}

func runDatasetItems(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	if err := validateListPagination(limit, offset); err != nil {
		return err
	}
	if limit > datasetMaxLimit {
		return errDatasetLimit(datasetMaxLimit)
	}
	if offset%limit != 0 {
		return errDatasetItemsOffset(limit)
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	dataset, err := resolveDataset(cmd, at, verbose, name)
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", offset/limit+1))
	q.Set("limit", fmt.Sprintf("%d", limit))
	u := fmt.Sprintf("%s/api/v1/datasets/%s/items?%s", datasetBaseURL(), url.PathEscape(dataset.ID), q.Encode())
	var result datasetItemsResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result)
	if status == http.StatusNotFound {
		return errDatasetNotFound(name)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}
	if len(result.Items) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoDatasetItems(name), colorReset) //nolint:errcheck,gosec
		return nil
	}

	const traceIDWidth = 32
	const valueWidth = 40
	rows := make([][]string, len(result.Items))
	for i, item := range result.Items {
		rows[i] = []string{
			truncate(item.SourceTraceID, traceIDWidth),
			truncate(oneLine(item.Input), valueWidth),
			truncate(oneLine(item.ExpectedOutput), valueWidth),
			evaluatorCell(item),
		}
	}
	writeTable(w, []string{"Trace ID", "Input", "Expected output", "Evaluators"}, rows)

	if result.TotalItems > offset+len(result.Items) {
		fmt.Fprintf(w, "%s\nShowing %d–%d of %d. Page with --offset %d.%s\n", colorDim, //nolint:errcheck,gosec
			offset+1, offset+len(result.Items), result.TotalItems, offset+limit, colorReset)
	}
	return nil
}

func oneLine(v json.RawMessage) string {
	return strings.Join(strings.Fields(rawValue(v)), " ")
}

func evaluatorCell(item datasetItem) string {
	if len(item.EvaluatorOutputs) == 0 {
		return "-"
	}
	parts := make([]string, len(item.EvaluatorOutputs))
	for i, o := range item.EvaluatorOutputs {
		parts[i] = o.Key + "=" + rawValue(o.Value)
	}
	cell := strings.Join(parts, " ")
	if item.Outdated {
		cell += " (outdated)"
	}
	return cell
}

type datasetAddResponse struct {
	EvalDatasetID string `json:"eval_dataset_id"`
	TraceID       string `json:"trace_id"`
	EvaluationRef string `json:"evaluation_ref"`
}

func runDatasetAdd(cmd *cobra.Command, args []string) error {
	traceID, _ := cmd.Flags().GetString("trace-id")
	if traceID == "" {
		return errTraceIDRequired()
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	dataset, err := resolveDataset(cmd, at, verbose, name)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/api/v1/datasets/%s/items", datasetBaseURL(), url.PathEscape(dataset.ID))
	var resp datasetAddResponse
	status, err := apiCall(cmd.Context(), http.MethodPost, u, map[string]any{"trace_id": traceID}, at.Token, verbose, &resp)
	switch status {
	case http.StatusBadRequest:
		return errDatasetAddRejected(apiErrorMessage(err))
	case http.StatusNotFound:
		return errDatasetTraceNotFound(traceID, name)
	case http.StatusForbidden:
		return errDatasetAddWrongDeployment(traceID, name)
	case http.StatusConflict:
		return errDatasetAddAlreadyAdded(traceID, name)
	case http.StatusUnprocessableEntity:
		return errDatasetAddNoInput(traceID)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, resp)
	}
	fmt.Fprintln(w, msgDatasetAdded(traceID, name)) //nolint:errcheck,gosec
	return nil
}

type datasetEditResponse struct {
	EvalDatasetID    string            `json:"eval_dataset_id"`
	TraceID          string            `json:"trace_id"`
	EvaluationRef    string            `json:"evaluation_ref"`
	VerifiedByUserID string            `json:"verified_by_user_id"`
	EvaluatorOutputs []evalOutputValue `json:"evaluator_outputs"`
}

func runDatasetEdit(cmd *cobra.Command, args []string) error {
	traceID, _ := cmd.Flags().GetString("trace-id")
	if traceID == "" {
		return errTraceIDRequired()
	}
	inferred, _ := cmd.Flags().GetStringArray("set")
	strict, _ := cmd.Flags().GetStringArray("set-string")
	if len(inferred)+len(strict) == 0 {
		return errSetValueRequired()
	}
	edits, err := parseEvalSetFlags(inferred, strict)
	if err != nil {
		return err
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	dataset, err := resolveDataset(cmd, at, verbose, name)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/api/v1/datasets/%s/items/%s/evaluator-outputs",
		datasetBaseURL(), url.PathEscape(dataset.ID), url.PathEscape(traceID))
	var resp datasetEditResponse
	status, err := apiCall(cmd.Context(), http.MethodPut, u, map[string]any{"values": evalOutputValues(edits)}, at.Token, verbose, &resp)
	switch status {
	case http.StatusBadRequest:
		return errDatasetEditRejected(apiErrorMessage(err))
	case http.StatusNotFound:
		return errDatasetItemNotFound(traceID, name)
	case http.StatusConflict:
		return errDatasetEditOutdated(traceID, name)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, resp)
	}
	fmt.Fprintln(w, msgDatasetEdited(traceID, name, len(resp.EvaluatorOutputs))) //nolint:errcheck,gosec
	return nil
}

func runDatasetRemove(cmd *cobra.Command, args []string) error {
	traceID, _ := cmd.Flags().GetString("trace-id")
	if traceID == "" {
		return errTraceIDRequired()
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	dataset, err := resolveDataset(cmd, at, verbose, name)
	if err != nil {
		return err
	}

	if !confirmDelete(cmd,
		fmt.Sprintf("Remove trace %s from dataset %s?", traceID, name),
		fmt.Sprintf("This removes the item and its evaluator values from dataset %s. The trace and its evaluations are kept.", name),
		traceID) {
		return nil
	}

	u := fmt.Sprintf("%s/api/v1/datasets/%s/items/%s", datasetBaseURL(), url.PathEscape(dataset.ID), url.PathEscape(traceID))
	var resp datasetAddResponse
	status, err := apiCall(cmd.Context(), http.MethodDelete, u, nil, at.Token, verbose, &resp)
	if status == http.StatusNotFound {
		return errDatasetItemNotFound(traceID, name)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, resp)
	}
	color.New(color.FgGreen).Fprint(w, "✓ ")          //nolint:errcheck,gosec
	fmt.Fprintln(w, msgDatasetRemoved(traceID, name)) //nolint:errcheck,gosec
	return nil
}

func runDatasetDownload(cmd *cobra.Command, args []string) error {
	output := flagString(cmd, "output")

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	dataset, err := resolveDataset(cmd, at, verbose, name)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/api/v1/datasets/%s/download", datasetBaseURL(), url.PathEscape(dataset.ID))
	status, body, err := apiStream(cmd.Context(), u, at.Token, verbose)
	if status == http.StatusNotFound {
		return errDatasetNotFound(name)
	}
	if err != nil {
		return err
	}
	defer body.Close() //nolint:errcheck

	if output == "-" {
		if _, err := io.Copy(cmd.OutOrStdout(), body); err != nil {
			return errDatasetDownloadFailed(err)
		}
		return nil
	}

	path := datasetDownloadPath(output, name)
	written, err := writeFileAtomically(path, body)
	if err != nil {
		return errDatasetDownloadFailed(err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), msgDatasetDownloaded(name, path, written)) //nolint:errcheck,gosec
	return nil
}

// datasetDownloadPath resolves --output: empty means <name>.zip in the current
// directory, and an existing directory means <name>.zip inside it.
func datasetDownloadPath(output, name string) string {
	file := filepath.Base(name) + ".zip"
	if output == "" {
		return file
	}
	if info, err := os.Stat(output); err == nil && info.IsDir() {
		return filepath.Join(output, file)
	}
	return output
}

// writeFileAtomically streams r to a temporary file beside path and renames it
// into place, so a failed download leaves no partial file and keeps any
// existing one.
func writeFileAtomically(path string, r io.Reader) (int64, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".download-*.tmp")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck,gosec

	written, err := io.Copy(tmp, r)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return 0, err
	}
	return written, nil
}
