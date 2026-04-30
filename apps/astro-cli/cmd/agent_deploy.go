package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// deployTemplateRequest is the POST body for /agents/:account/:name/deployment-template.
type deployTemplateRequest struct {
	Build        string                    `json:"build,omitempty"`
	DeploymentID string                    `json:"deployment_id,omitempty"`
	Interfaces   *deployTemplateInterfaces `json:"interfaces,omitempty"`
	Variables    map[string]deployVarInput `json:"variables,omitempty"`
}

type deployTemplateInterfaces struct {
	Adapters []string              `json:"adapters"`
	Auth     *deployInterfacesAuth `json:"auth,omitempty"`
}

type deployInterfacesAuth struct {
	Web *deployWebAuth `json:"web,omitempty"`
}

type deployWebAuth struct {
	Type string `json:"type,omitempty"`
}

type deployVarInput struct {
	Value string `json:"value,omitempty"`
	Ref   string `json:"ref,omitempty"`
}

type deployValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type deployTemplateValidation struct {
	Valid  bool                    `json:"valid"`
	Errors []deployValidationError `json:"errors,omitempty"`
}

type deployTemplateResponse struct {
	Template   json.RawMessage          `json:"template"`
	Validation deployTemplateValidation `json:"validation"`
}

type agentDeployResult struct {
	Status       string `json:"status"`
	DeploymentID string `json:"deployment_id"`
	Name         string `json:"name"`
	BuildID      string `json:"build_id"`
}

var blueprintDeployCmd = &cobra.Command{
	Use:   "deploy <name>",
	Short: "Deploy a blueprint",
	Args:  exactValidAgentName,
	RunE:  runBlueprintDeploy,
}

// registerDeployCommonFlags registers adapter/var/build/dry-run/json flags shared by deploy and redeploy.
func registerDeployCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("adapter", nil, "Adapter to enable: web, insecure-web, slack (default: web; repeatable)")
	cmd.Flags().StringArray("var", nil, "Variable: KEY=VALUE, KEY=@SECRET_NAME, or KEY=@ (secret named KEY); escape literal @ with \\@ (repeatable)")
	cmd.Flags().String("vars-file", "", "Load variables from a .env file")
	cmd.Flags().String("build", "", "Pin to a specific build ID")
	cmd.Flags().Bool("dry-run", false, "Validate inputs without deploying")
	cmd.Flags().Bool("json", false, "Print JSON output on success")
}

func registerDeployFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("name", "n", "", "Display name for the deployment")
	registerDeployCommonFlags(cmd)
}

func init() {
	blueprintCmd.AddCommand(blueprintDeployCmd)
	registerDeployFlags(blueprintDeployCmd)

	topLevelDeployCmd := &cobra.Command{
		Use:   blueprintDeployCmd.Use,
		Short: blueprintDeployCmd.Short,
		Args:  exactValidAgentName,
		RunE:  runBlueprintDeploy,
	}
	registerDeployFlags(topLevelDeployCmd)
	rootCmd.AddCommand(topLevelDeployCmd)
}

// parseDeployVars parses --var flags into a variables map.
// KEY=VALUE sets an inline value.
// KEY=@SECRET sets a reference to an account variable named SECRET.
// KEY=@ sets a self-referencing secret reference using KEY as the secret name.
// KEY=\@VALUE sets an inline literal value starting with @.
func parseDeployVars(varFlags []string) (map[string]deployVarInput, error) {
	result := make(map[string]deployVarInput)
	for _, v := range varFlags {
		key, val, ok := strings.Cut(v, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --var %q: must be KEY=VALUE or KEY=@SECRET_NAME", v)
		}
		switch {
		case strings.HasPrefix(val, `\@`):
			result[key] = deployVarInput{Value: strings.TrimPrefix(val, `\`)}
		case val == "@":
			result[key] = deployVarInput{Ref: key}
		case strings.HasPrefix(val, "@"):
			result[key] = deployVarInput{Ref: strings.TrimPrefix(val, "@")}
		default:
			result[key] = deployVarInput{Value: val}
		}
	}
	return result, nil
}

// buildDeployInterfaces translates CLI adapter names to the server-side interfaces request.
// Valid names: web, insecure-web, slack. web and insecure-web are mutually exclusive.
// Both web and insecure-web send "web" to the server; the only difference is that
// web enables OIDC auth while insecure-web does not.
// When no adapters are given, defaults to web with OIDC auth.
func buildDeployInterfaces(adapters []string) (*deployTemplateInterfaces, error) {
	if len(adapters) == 0 {
		return &deployTemplateInterfaces{
			Adapters: []string{"web"},
			Auth:     &deployInterfacesAuth{Web: &deployWebAuth{Type: "oidc"}},
		}, nil
	}

	serverAdapters := make([]string, 0, len(adapters))
	secureWeb := false
	webVariants := 0
	for _, a := range adapters {
		switch a {
		case "web":
			secureWeb = true
			serverAdapters = append(serverAdapters, "web")
			webVariants++
		case "insecure-web":
			serverAdapters = append(serverAdapters, "web")
			webVariants++
		case "slack":
			serverAdapters = append(serverAdapters, "slack")
		default:
			return nil, fmt.Errorf("unknown adapter %q: must be one of: web, insecure-web, slack", a)
		}
	}

	if webVariants > 1 {
		return nil, fmt.Errorf("--adapter web and --adapter insecure-web are mutually exclusive")
	}

	iface := &deployTemplateInterfaces{Adapters: serverAdapters}
	if secureWeb {
		iface.Auth = &deployInterfacesAuth{Web: &deployWebAuth{Type: "oidc"}}
	}
	return iface, nil
}

// parseDeployVarsFromCmd reads --var and --vars-file flags and merges them.
// Inline --var entries take precedence over file entries for the same key.
func parseDeployVarsFromCmd(cmd *cobra.Command) (map[string]deployVarInput, error) {
	varFlags, _ := cmd.Flags().GetStringArray("var")
	vars, err := parseDeployVars(varFlags)
	if err != nil {
		return nil, err
	}

	varsFile, _ := cmd.Flags().GetString("vars-file")
	if varsFile != "" {
		f, err := os.Open(varsFile) //nolint:gosec
		if err != nil {
			return nil, fmt.Errorf("opening vars file: %w", err)
		}
		defer f.Close() //nolint:errcheck,gosec
		fileVars, err := godotenv.Parse(f)
		if err != nil {
			return nil, fmt.Errorf("parsing vars file: %w", err)
		}
		for k, v := range fileVars {
			if _, exists := vars[k]; !exists {
				vars[k] = deployVarInput{Value: v}
			}
		}
	}

	return vars, nil
}

func runBlueprintDeploy(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	adapters, _ := cmd.Flags().GetStringArray("adapter")
	build, _ := cmd.Flags().GetString("build")
	displayName, _ := cmd.Flags().GetString("name")
	if displayName == "" {
		displayName = name
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	vars, err := parseDeployVarsFromCmd(cmd)
	if err != nil {
		return err
	}

	iface, err := buildDeployInterfaces(adapters)
	if err != nil {
		return err
	}

	req := deployTemplateRequest{Build: build, Interfaces: iface}
	if len(vars) > 0 {
		req.Variables = vars
	}

	return runDeployWithRequest(cmd, at, verbose, name, displayName, req, dryRun)
}

// runDeployWithRequest handles the template POST → validation → deploy POST flow.
// Shared by blueprint deploy and agent redeploy; set req.DeploymentID to retarget an existing deployment.
// displayName is patched into template.target.display_name before the deploy POST when non-empty.
func runDeployWithRequest(cmd *cobra.Command, at AccountToken, verbose bool, name, displayName string, req deployTemplateRequest, dryRun bool) error {
	w := cmd.OutOrStdout()
	verb := "Deploying"
	if req.DeploymentID != "" {
		verb = "Redeploying"
	}
	fmt.Fprintf(w, "%s→%s %s blueprint %s%s%s as agent %s%s%s\n", colorCyan, colorReset, verb, colorBold, name, colorReset, colorBold, displayName, colorReset) //nolint:errcheck,gosec

	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "deployment-template")
	var tmplResp deployTemplateResponse
	if status, err := apiCall(cmd.Context(), http.MethodPost, u, req, at.Token, verbose, &tmplResp); err != nil {
		if status == http.StatusNotFound {
			return notFoundFromTemplateErr(err, at.Account, name, req.Build)
		}
		return err
	}

	if !tmplResp.Validation.Valid {
		fmt.Fprintf(w, "  %s✗%s validation failed:\n", colorRed, colorReset) //nolint:errcheck,gosec
		for _, e := range tmplResp.Validation.Errors {
			field := strings.TrimPrefix(e.Field, "variables.")
			field = strings.ReplaceAll(field, ".", " ")
			fmt.Fprintf(w, "    %svariable %s%s%s%s:%s %s\n", colorDim, colorReset, colorBold, field, colorDim, colorReset, e.Message) //nolint:errcheck,gosec
		}
		return fmt.Errorf("deployment validation failed")
	}

	if dryRun {
		fmt.Fprintf(w, "  %s✓%s template valid\n", colorGreen, colorReset) //nolint:errcheck,gosec
		return nil
	}

	template := tmplResp.Template
	if displayName != "" {
		patched, err := patchTemplateDisplayName(template, displayName)
		if err != nil {
			return fmt.Errorf("patching display name: %w", err)
		}
		template = patched
	}

	deployURL := blueprintBaseURL() + "/api/v1/deploy"
	var result agentDeployResult
	if status, err := apiCall(cmd.Context(), http.MethodPost, deployURL, template, at.Token, verbose, &result); err != nil {
		if status == http.StatusNotFound {
			return fmt.Errorf("agent deployment %q no longer exists", displayName)
		}
		if status == http.StatusConflict {
			return errDeployNameConflict(displayName)
		}
		return err
	}

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}

	dim := color.New(color.Faint)
	green := color.New(color.FgGreen)
	green.Fprintf(w, "  ✓ deployed") //nolint:errcheck,gosec
	if result.DeploymentID != "" {
		dim.Fprintf(w, "  %s\n", result.DeploymentID) //nolint:errcheck,gosec
	} else {
		fmt.Fprintln(w) //nolint:errcheck,gosec
	}
	return nil
}

// Server-side error_code values returned by /deployment-template on 404.
// Kept in sync with apps/astro-server/handlers/deploy.go.
const (
	errCodeAccountNotFound   = "account_not_found"
	errCodeBlueprintNotFound = "blueprint_not_found"
	errCodeBuildNotFound     = "build_not_found"
)

// notFoundFromTemplateErr maps the deployment-template 404 body to a more
// actionable CLI error. The endpoint returns 404 for three distinct reasons
// (unknown account, unknown blueprint or private+non-member, blueprint exists
// but the requested build is missing), each tagged with a typed error_code on
// the response. The legacy text-based fallback runs only if the server is too
// old to populate error_code so older deploy clients still get a useful
// message.
func notFoundFromTemplateErr(err error, account, name, buildID string) error {
	code, body := apiErrorCodeAndBody(err)
	switch code {
	case errCodeBuildNotFound:
		if buildID != "" {
			return fmt.Errorf("build %q not found for blueprint %q (run `ast push %s` or pick an existing build with `ast blueprint builds %s`)", buildID, name, name, name)
		}
		return fmt.Errorf("blueprint %q has no builds yet (run `ast push %s` first)", name, name)
	case errCodeAccountNotFound:
		return fmt.Errorf("account %q not found", account)
	case errCodeBlueprintNotFound:
		return fmt.Errorf("blueprint %q not found in account %q", name, account)
	}
	switch {
	case strings.Contains(body, "no builds found for agent"):
		if buildID != "" {
			return fmt.Errorf("build %q not found for blueprint %q (run `ast push %s` or pick an existing build with `ast blueprint builds %s`)", buildID, name, name, name)
		}
		return fmt.Errorf("blueprint %q has no builds yet (run `ast push %s` first)", name, name)
	case strings.Contains(body, "account not found"):
		return fmt.Errorf("account %q not found", account)
	case strings.Contains(body, "agent not found"):
		return fmt.Errorf("blueprint %q not found in account %q", name, account)
	default:
		return fmt.Errorf("blueprint %q not found", name)
	}
}

// apiErrorCodeAndBody parses the embedded JSON body in an apiCall error
// (format "server returned status N: <body>") and returns its error_code
// field plus the raw body. Empty error_code means the server response is too
// old to carry a typed code so callers should fall back to message inspection.
func apiErrorCodeAndBody(err error) (code, body string) {
	if err == nil {
		return "", ""
	}
	msg := err.Error()
	idx := strings.Index(msg, ": ")
	if idx == -1 {
		body = msg
	} else {
		body = msg[idx+2:]
	}
	var parsed struct {
		ErrorCode string `json:"error_code"`
	}
	if jerr := json.Unmarshal([]byte(body), &parsed); jerr == nil {
		code = parsed.ErrorCode
	}
	return code, body
}

// patchTemplateDisplayName sets target.display_name in the deployment template JSON.
func patchTemplateDisplayName(template json.RawMessage, displayName string) (json.RawMessage, error) {
	var tmpl map[string]any
	if err := json.Unmarshal(template, &tmpl); err != nil {
		return nil, err
	}
	target, _ := tmpl["target"].(map[string]any)
	if target == nil {
		target = make(map[string]any)
	}
	target["display_name"] = displayName
	tmpl["target"] = target
	return json.Marshal(tmpl)
}
