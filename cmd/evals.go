package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

const evaluationFilename = "EVALUATION.yaml"

// evalsServerURLOverride is set in tests to redirect API calls to a test server.
var evalsServerURLOverride string

func evalsBaseURL() string {
	if evalsServerURLOverride != "" {
		return strings.TrimSuffix(evalsServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

var evalsCmd = &cobra.Command{
	Use:   "evals",
	Short: "Manage an agent's custom evaluators",
}

var evalsPushCmd = &cobra.Command{
	Use:   "push <name>",
	Short: "Activate the agent's EVALUATION.yaml as its evaluation set",
	Long:  "Reads EVALUATION.yaml beside astropods.yml and activates it as the agent's evaluation set on the server. This does not build or push a container image.",
	Args:  exactValidAgentName,
	RunE:  runEvalsPush,
}

func init() {
	rootCmd.AddCommand(evalsCmd)
	evalsCmd.AddCommand(evalsPushCmd)
	evalsPushCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
}

func runEvalsPush(cmd *cobra.Command, args []string) error {
	name := args[0]

	specPath, err := resolveSpecPathFromCwd(flagString(cmd, "file"))
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

	u := apiPath(evalsBaseURL(), at.Account, "agents", name, "evaluation-set")
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

// loadEvaluationDocument reads EVALUATION.yaml from workingDir and resolves
// every referenced prompt_file into a path→contents map.
func loadEvaluationDocument(workingDir string) (evaluationYAML string, promptFiles map[string]string, err error) {
	path := filepath.Join(workingDir, evaluationFilename)
	data, readErr := os.ReadFile(path) //nolint:gosec
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return "", nil, errNoEvaluationFile()
		}
		return "", nil, fmt.Errorf("failed to read %s: %w", evaluationFilename, readErr)
	}

	var doc struct {
		Evaluators []struct {
			PromptFile string `yaml:"prompt_file"`
		} `yaml:"evaluators"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", nil, fmt.Errorf("failed to parse %s: %w", evaluationFilename, err)
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
