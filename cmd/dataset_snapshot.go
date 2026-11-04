package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
)

type releaseCase struct {
	ID             string                     `json:"id"`
	Input          json.RawMessage            `json:"input"`
	ExpectedOutput json.RawMessage            `json:"expected_output,omitempty"`
	ExpectedValues map[string]json.RawMessage `json:"expected_values,omitempty"`
}

type releaseDataset struct {
	Name            string        `json:"name"`
	SourceDatasetID string        `json:"source_dataset_id,omitempty"`
	Cases           []releaseCase `json:"cases"`
}

func newDatasetSnapshotCommand() *cobra.Command {
	command := &cobra.Command{Use: "snapshot <dataset-name>", Short: "Export a curated dataset for candidate build evaluation", Long: msgDatasetSnapshotHelp(), Args: cobra.ExactArgs(1), RunE: runDatasetSnapshot}
	command.Flags().String("output", "dataset.json", "Snapshot file, or - for stdout")
	command.Flags().Int("limit", 50, "Cases to export, from 1 to 50")
	command.Flags().Int("offset", 0, "Pagination offset, a multiple of limit")
	return command
}

func init() { datasetCmd.AddCommand(newDatasetSnapshotCommand()) }

func runDatasetSnapshot(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	output, _ := cmd.Flags().GetString("output")
	if limit < 1 || limit > 50 || offset < 0 || offset%limit != 0 || output == "" {
		return errDatasetSnapshotFlags()
	}
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	dataset, err := resolveDataset(cmd, at, verbose, args[0])
	if err != nil {
		return err
	}
	query := url.Values{"page": {strconv.Itoa(offset/limit + 1)}, "limit": {strconv.Itoa(limit)}}
	var page datasetItemsResponse
	endpoint := apiPath(datasetBaseURL(), url.PathEscape(dataset.ID), "datasets", "items") + "?" + query.Encode()
	if _, err = apiCall(cmd.Context(), http.MethodGet, endpoint, nil, at.Token, verbose, &page); err != nil {
		return err
	}
	if page.TotalItems > limit && !cmd.Flags().Changed("limit") && !cmd.Flags().Changed("offset") {
		return errDatasetSnapshotSelection()
	}
	snapshot, err := snapshotDataset(dataset.DatasetName, dataset.ID, page.Items)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > releasePayloadLimit {
		return errReleaseJSONFile(output)
	}
	if output == "-" {
		return writeJSON(cmd.OutOrStdout(), snapshot)
	}
	if _, err = writeFileAtomically(output, bytes.NewReader(append(raw, '\n'))); err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msgDatasetSnapshotSaved(len(snapshot.Cases), output))
	return err
}

func snapshotDataset(name, id string, items []datasetItem) (releaseDataset, error) {
	result := releaseDataset{Name: name, SourceDatasetID: id, Cases: make([]releaseCase, 0, len(items))}
	if len(items) < 1 || len(items) > 50 {
		return result, errDatasetSnapshotEmpty()
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.Outdated {
			return result, errDatasetSnapshotOutdated(item.ID)
		}
		if item.ID == "" || len(item.ID) > 100 || seen[item.ID] || !json.Valid(item.Input) || bytes.Equal(bytes.TrimSpace(item.Input), []byte("null")) {
			return result, errDatasetSnapshotInput(item.ID)
		}
		seen[item.ID] = true
		c := releaseCase{ID: item.ID, Input: item.Input, ExpectedOutput: item.ExpectedOutput, ExpectedValues: map[string]json.RawMessage{}}
		for _, value := range item.EvaluatorOutputs {
			if _, duplicate := c.ExpectedValues[value.Key]; duplicate {
				return result, errDatasetSnapshotInput(item.ID)
			}
			c.ExpectedValues[value.Key] = value.Value
		}
		result.Cases = append(result.Cases, c)
	}
	sort.Slice(result.Cases, func(i, j int) bool { return result.Cases[i].ID < result.Cases[j].ID })
	return result, nil
}
