package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDatasetSnapshotPreservesReferenceValuesAndRequiresAnExplicitSubset(t *testing.T) {
	for _, condition := range []string{"complete", "large", "subset", "outdated"} {
		t.Run(condition, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "snapshot.json")
			calls := 0
			setupDatasetTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				if !strings.HasSuffix(r.URL.Path, "/items") {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"datasets": []any{datasetRow(1, "dummy")}, "total": 1}))
					return
				}
				require.Equal(t, "/api/v1/datasets/ds-1/items", r.URL.Path)
				total := 2
				if condition == "large" || condition == "subset" {
					total = 100
				}
				require.NoError(t, json.NewEncoder(w).Encode(datasetItemsResponse{TotalItems: total, Items: []datasetItem{
					{ID: "two", Input: json.RawMessage(`9007199254740993`), ExpectedOutput: json.RawMessage(`"answer"`), Outdated: condition == "outdated", EvaluatorOutputs: []datasetItemOutput{{Key: "accuracy", Value: json.RawMessage(`true`)}}},
					{ID: "one", Input: json.RawMessage(`{"message":"hello"}`)},
				}}))
			}))
			command := newDatasetSnapshotCommand()
			args := []string{"eval-dep00000001", "--output", output}
			if condition == "subset" {
				args = append(args, "--limit", "2")
			}
			command.SetArgs(args)
			command.SetContext(t.Context())
			command.SilenceErrors = true
			command.SilenceUsage = true
			var out bytes.Buffer
			command.SetOut(&out)
			err := command.Execute()
			switch condition {
			case "large":
				require.EqualError(t, err, errDatasetSnapshotSelection().Error())
			case "outdated":
				require.EqualError(t, err, errDatasetSnapshotOutdated("two").Error())
			default:
				require.NoError(t, err)
				raw, err := os.ReadFile(output)
				require.NoError(t, err)
				var snapshot releaseDataset
				require.NoError(t, json.Unmarshal(raw, &snapshot))
				require.Equal(t, "ds-1", snapshot.SourceDatasetID)
				require.Equal(t, "one", snapshot.Cases[0].ID)
				require.JSONEq(t, `9007199254740993`, string(snapshot.Cases[1].Input))
				require.JSONEq(t, `"answer"`, string(snapshot.Cases[1].ExpectedOutput))
				require.JSONEq(t, `true`, string(snapshot.Cases[1].ExpectedValues["accuracy"]))
				info, err := os.Stat(output)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0600), info.Mode().Perm())
			}
			require.Equal(t, 2, calls)
		})
	}
}
