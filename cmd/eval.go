package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	evalspec "github.com/astropods/astro-spec/eval"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/utils"
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

func init() {
	rootCmd.AddCommand(evalCmd)
	evalCmd.AddCommand(evalPushCmd)
	evalCmd.AddCommand(evalValidateCmd)
	evalPushCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	evalValidateCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
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

func resolveEvalAgentName(specPath string, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

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

	_, name := utils.ParseAgentName(doc.Name)
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
