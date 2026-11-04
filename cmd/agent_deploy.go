package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/config"
	"github.com/astropods/astro-cli/internal/tui"
)

// deployTemplateRequest is the POST body for /agents/:account/:name/deployment-template.
type deployTemplateRequest struct {
	Build        string `json:"build,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty"`
	// The URL account is the blueprint's publisher; this is where the agent runs.
	TargetAccount string                    `json:"target_account,omitempty"`
	Interfaces    *deployTemplateInterfaces `json:"interfaces,omitempty"`
	Variables     map[string]deployVarInput `json:"variables,omitempty"`
	Schedules     map[string]string         `json:"schedules,omitempty"`
	Finalize      bool                      `json:"finalize,omitempty"`
	ClusterID     string                    `json:"cluster_id,omitempty"`
}

type deployTemplateInterfaces struct {
	Adapters []string `json:"adapters"`
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
	Signature  string                   `json:"signature,omitempty"`
	Schedules  map[string]string        `json:"schedules,omitempty"`
}

type agentDeployResult struct {
	Revision        int    `json:"revision"`
	CommitSHA       string `json:"commit_sha,omitempty"`
	Status          string `json:"status"`
	DeploymentID    string `json:"deployment_id"`
	Name            string `json:"name"`
	BuildID         string `json:"build_id"`
	EnvironmentID   string `json:"environment_id,omitempty"`
	EnvironmentName string `json:"environment_name,omitempty"`
}

var blueprintDeployCmd = &cobra.Command{
	Use:   "deploy [<account>/]<name>",
	Short: "Deploy a blueprint",
	Long: `Deploy a blueprint as an agent in the active account.

Name a blueprint that another account publishes as <account>/<name>. A bare
<name> is a blueprint in the active account. With no argument, deploy the
blueprint in the current directory's astropods.yml.`,
	Args: optionalBlueprintRef,
	RunE: runBlueprintDeploy,
}

// registerDeployCommonFlags registers adapter/var/build/dry-run/json flags shared by deploy and redeploy.
func registerDeployCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("adapter", nil, "Adapter to enable: web or slack (default: web; repeatable). insecure-web is accepted as web")
	cmd.Flags().StringArray("var", nil, "Variable: KEY=VALUE, KEY=@SECRET_NAME, or KEY=@ (secret named KEY); escape literal @ with \\@ (repeatable)")
	cmd.Flags().String("vars-file", "", "Load variables from a .env file")
	cmd.Flags().Bool("from-local", false, "Load variables saved by 'project configure' in the current directory as plaintext values, not secret references; --var and --vars-file take precedence")
	cmd.Flags().StringArray("schedule", nil, "Job schedule: <job>=<cron expression> (repeatable)")
	cmd.Flags().String("build", "", "Pin to a specific build ID")
	cmd.Flags().String("cluster", "", "Cluster to deploy to (default: the account default, or the agent's current cluster on redeploy)")
	cmd.Flags().Bool("dry-run", false, "Validate inputs without deploying")
	cmd.Flags().Bool("json", false, "Print JSON output on success")
	cmd.Flags().Bool("wait", false, "Wait for the submitted revision to become ready")
	cmd.Flags().String("commit", "", "Wait for and deploy an exact full Git commit SHA")
	cmd.Flags().Duration("timeout", orchestrationTimeout, "Maximum time for build and deployment")
}

func registerDeployFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("name", "n", "", "Display name for the deployment")
	cmd.Flags().String("env", "", "Environment to deploy into; it must have no agent (default: a free environment, or a new one)")
	registerDeployCommonFlags(cmd)
	registerAgentCoreDeployFlags(cmd)
}

func init() {
	blueprintCmd.AddCommand(blueprintDeployCmd)
	registerDeployFlags(blueprintDeployCmd)

	topLevelDeployCmd := &cobra.Command{
		Use:   blueprintDeployCmd.Use,
		Short: blueprintDeployCmd.Short,
		Long:  blueprintDeployCmd.Long,
		Args:  optionalBlueprintRef,
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
// Valid names: web, insecure-web, slack. insecure-web is the same as web.
// When no adapters are given, defaults to web.
func buildDeployInterfaces(adapters []string) (*deployTemplateInterfaces, error) {
	if len(adapters) == 0 {
		return &deployTemplateInterfaces{Adapters: []string{"web"}}, nil
	}

	serverAdapters := make([]string, 0, len(adapters))
	webVariants := 0
	for _, a := range adapters {
		switch a {
		case "web", "insecure-web":
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
	return &deployTemplateInterfaces{Adapters: serverAdapters}, nil
}

// parseDeployVarsFromCmd reads --var, --vars-file, and --from-local and merges
// them. For the same key, --var wins over --vars-file, which wins over --from-local.
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

	if fromLocal, _ := cmd.Flags().GetBool("from-local"); fromLocal {
		workingDir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		localVars := config.GetProjectVars(buildinfo.BinaryName, workingDir)
		if len(localVars) == 0 {
			return nil, errNoLocalProjectVars()
		}
		for k, v := range localVars {
			if _, exists := vars[k]; !exists {
				vars[k] = deployVarInput{Value: v}
			}
		}
	}

	return vars, nil
}

// scheduleCronParser matches the server's parser, so a cron the CLI accepts is
// one the server accepts.
var scheduleCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func parseDeploySchedules(values []string) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for _, raw := range values {
		name, cron, ok := strings.Cut(raw, "=")
		name = strings.TrimSpace(name)
		cron = strings.TrimSpace(cron)
		if !ok || name == "" || cron == "" {
			return nil, errInvalidSchedule(raw)
		}
		if _, exists := result[name]; exists {
			return nil, errDuplicateSchedule(name)
		}
		if _, err := scheduleCronParser.Parse(cron); err != nil {
			return nil, errInvalidCronExpression(name, cron)
		}
		result[name] = cron
	}
	return result, nil
}

func parseDeploySchedulesFromCmd(cmd *cobra.Command) (map[string]string, error) {
	values, _ := cmd.Flags().GetStringArray("schedule")
	return parseDeploySchedules(values)
}

// The server ignores a schedule aimed at an unknown job silently.
func checkScheduleTargets(requested, available map[string]string) error {
	if len(requested) == 0 {
		return nil
	}
	unknown := make([]string, 0, len(requested))
	for name := range requested {
		if _, ok := available[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	names := make([]string, 0, len(available))
	for name := range available {
		names = append(names, name)
	}
	slices.Sort(unknown)
	slices.Sort(names)
	return errUnknownJobSchedule(unknown, names)
}

// deployValidationSubject splits a template validation field into the label to
// print and the subject to emphasize. Only variables.* names read as a
// variable; every other field is an API path, so it stays intact.
func deployValidationSubject(field string) (label, subject string) {
	if name, ok := strings.CutPrefix(field, "variables."); ok {
		return "variable ", strings.ReplaceAll(name, ".", " ")
	}
	return "", field
}

func runBlueprintDeploy(cmd *cobra.Command, args []string) error {
	// The spec picks the runtime: a local astropods.yml with
	// agent.annotations.runtime: agentcore deploys to AWS Bedrock AgentCore
	// instead of taking the server-mediated path. There is no target flag.
	if handled, err := maybeAgentCoreDeploy(cmd, args); handled || err != nil {
		return err
	}
	if err := rejectAgentCoreOnlyFlags(cmd); err != nil {
		return err
	}
	_, cancel, err := orchestrationContext(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	var arg string
	if len(args) > 0 {
		arg = args[0]
	}
	ref, err := resolveBlueprintName(arg)
	if err != nil {
		return err
	}

	schedules, err := parseDeploySchedulesFromCmd(cmd)
	if err != nil {
		return err
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	publisher, name := splitBlueprintRef(ref)
	if publisher == "" {
		publisher = at.Account
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

	var environment *blueprintEnvironment
	if env := flagString(cmd, "env"); env != "" {
		environment, err = findBlueprintEnvironment(cmd.Context(), at, publisher, name, env, verbose)
		if err != nil {
			return err
		}
		if environment.DeploymentID != "" {
			return environmentOccupiedErr(at, env, publisher, name)
		}
	}

	clusterID, err := resolveDeployCluster(cmd, at, verbose)
	if err != nil {
		if errors.Is(err, tui.ErrCanceled) {
			printCanceled(cmd.OutOrStdout())
			return nil
		}
		return err
	}

	req := deployTemplateRequest{Build: build, TargetAccount: at.Account, Interfaces: iface, ClusterID: clusterID}
	if len(vars) > 0 {
		req.Variables = vars
	}
	if len(schedules) > 0 {
		req.Schedules = schedules
	}

	return runDeployWithRequest(cmd, at, verbose, deployOptions{
		TemplateAccount: publisher, Blueprint: name, DisplayName: displayName,
		Environment: environment, Request: req, DryRun: dryRun,
	})
}

type deployOptions struct {
	TemplateAccount  string
	Blueprint        string
	DisplayName      string
	BuildPublisher   string
	ExpectedRevision int
	Environment      *blueprintEnvironment
	Request          deployTemplateRequest
	DryRun           bool
}

func runDeployWithRequest(cmd *cobra.Command, at AccountToken, verbose bool, opts deployOptions) error {
	req := opts.Request
	commit, err := buildCommit(cmd, req.Build)
	if err != nil {
		return err
	}
	ctx, cancel, err := orchestrationContext(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	w := cmd.OutOrStdout()
	jsonOut, _ := cmd.Flags().GetBool("json")
	wait, _ := cmd.Flags().GetBool("wait")
	progress := w
	if jsonOut {
		progress = cmd.ErrOrStderr()
	}
	if commit != "" || (wait && req.Build != "") {
		buildPublisher := opts.TemplateAccount
		if opts.BuildPublisher != "" {
			buildPublisher = opts.BuildPublisher
		}
		built, err := waitForBuild(ctx, at, buildPublisher, opts.Blueprint, req.Build, commit, verbose, progress)
		if err != nil {
			return err
		}
		req.Build = built.BuildID
		commit = built.CommitSHA
	}
	at, err = refreshPollingToken(ctx, at)
	if err != nil {
		return err
	}
	verb := "Deploying"
	if req.DeploymentID != "" {
		verb = "Redeploying"
	}
	into := ""
	if opts.Environment != nil {
		into = fmt.Sprintf(" into environment %s%s%s", colorBold, opts.Environment.Name, colorReset)
	}
	label := opts.Blueprint
	if opts.TemplateAccount != at.Account {
		label = opts.TemplateAccount + "/" + opts.Blueprint
	}
	fmt.Fprintf(progress, "%s→%s %s blueprint %s%s%s as agent %s%s%s%s\n", colorCyan, colorReset, verb, colorBold, label, colorReset, colorBold, opts.DisplayName, colorReset, into) //nolint:errcheck,gosec

	// Finalized templates include the signature required by the deploy endpoint.
	req.Finalize = true

	u := apiPath(blueprintBaseURL(), opts.TemplateAccount, "agents", opts.Blueprint, "deployment-template")
	var tmplResp deployTemplateResponse
	if status, err := apiCall(ctx, http.MethodPost, u, req, at.Token, verbose, &tmplResp); err != nil {
		if isNotFound(status, err) {
			return notFoundFromTemplateErr(err, opts.TemplateAccount, opts.Blueprint, req.Build, opts.TemplateAccount == at.Account)
		}
		if status == http.StatusForbidden {
			if clusterErr := clusterNotAvailableFromErr(err); clusterErr != nil {
				return clusterErr
			}
		}
		return err
	}

	if err := checkScheduleTargets(req.Schedules, tmplResp.Schedules); err != nil {
		return err
	}

	if !tmplResp.Validation.Valid {
		fmt.Fprintf(progress, "  %s✗%s validation failed:\n", colorRed, colorReset) //nolint:errcheck,gosec
		for _, e := range tmplResp.Validation.Errors {
			label, subject := deployValidationSubject(e.Field)
			fmt.Fprintf(progress, "    %s%s%s%s%s%s:%s %s\n", colorDim, label, colorReset, colorBold, subject, colorDim, colorReset, e.Message) //nolint:errcheck,gosec
		}
		return errDeploymentValidation()
	}

	if opts.DryRun {
		if jsonOut {
			return writeJSON(w, tmplResp)
		}
		fmt.Fprintf(progress, "  %s✓%s template valid\n", colorGreen, colorReset) //nolint:errcheck,gosec
		return nil
	}

	template := tmplResp.Template
	target := map[string]string{}
	if opts.DisplayName != "" && req.DeploymentID == "" {
		target["display_name"] = opts.DisplayName
	}
	if opts.Environment != nil {
		target["environment_id"] = opts.Environment.ID
	}
	if len(target) > 0 {
		patched, err := patchTemplateTarget(template, target)
		if err != nil {
			return errPatchDeployTarget(err)
		}
		template = patched
	}

	deployURL := blueprintBaseURL() + "/api/v1/deploy"
	deployHeaders := http.Header{"X-Template-Signature": []string{tmplResp.Signature}}
	if opts.ExpectedRevision > 0 {
		deployHeaders.Set("If-Match", strconv.Quote(strconv.Itoa(opts.ExpectedRevision)))
	}
	var result agentDeployResult
	if status, err := apiCallWithHeaders(ctx, http.MethodPost, deployURL, template, at.Token, deployHeaders, verbose, &result); err != nil {
		if isNotFound(status, err) {
			return errDeployUnavailable(opts.DisplayName, err)
		}
		if status == http.StatusPreconditionFailed {
			return errDeploymentRevisionConflict(opts.ExpectedRevision)
		}
		if status == http.StatusConflict {
			if code, _ := apiErrorCodeAndBody(err); code == errCodeEnvironmentTaken && opts.Environment != nil {
				return environmentOccupiedErr(at, opts.Environment.Name, opts.TemplateAccount, opts.Blueprint)
			}
			if code, _ := apiErrorCodeAndBody(err); code == "rollout_in_progress" {
				return errDeploymentRolloutInProgress()
			}
			return errDeployNameConflict(opts.DisplayName)
		}
		return err
	}

	result.CommitSHA = commit
	if wait {
		// The deployment is already submitted, so a server that reports no
		// revision downgrades the wait instead of failing a created deployment.
		if result.DeploymentID == "" || result.Revision < 1 {
			fmt.Fprintln(progress, msgDeploymentWaitUnsupported(result.DeploymentID)) //nolint:errcheck
		} else {
			fmt.Fprintln(progress, msgDeploymentQueued(result)) //nolint:errcheck
			if err := waitForDeployment(ctx, result, at, verbose, progress); err != nil {
				return err
			}
			result.Status = "active"
		}
	}
	if jsonOut {
		return writeJSON(w, result)
	}

	dim := color.New(color.Faint)
	green := color.New(color.FgGreen)
	green.Fprintf(w, "  ✓ %s", msgDeployed(result.EnvironmentName)) //nolint:errcheck,gosec
	if result.DeploymentID != "" {
		dim.Fprintf(w, "  %s\n", result.DeploymentID) //nolint:errcheck,gosec
	} else {
		fmt.Fprintln(w) //nolint:errcheck,gosec
	}

	return nil
}

// Server-side error_code values returned by /deployment-template on 404.
// Kept in sync with the astro-server deploy handler.
const (
	errCodeAccountNotFound   = "account_not_found"
	errCodeBlueprintNotFound = "blueprint_not_found"
	errCodeBuildNotFound     = "build_not_found"
	errCodeEnvironmentTaken  = "environment_taken"
)

// notFoundFromTemplateErr maps the deployment-template 404 body to a more
// actionable CLI error. The endpoint returns 404 for three distinct reasons
// (unknown account, unknown blueprint or private+non-member, blueprint exists
// but the requested build is missing), each tagged with a typed error_code on
// the response. The legacy text-based fallback runs only if the server is too
// old to populate error_code so older deploy clients still get a useful
// message.
func notFoundFromTemplateErr(err error, account, name, buildID string, own bool) error {
	code, body := apiErrorCodeAndBody(err)
	switch code {
	case errCodeBuildNotFound:
		return missingBuildErr(account, name, buildID, own)
	case errCodeAccountNotFound:
		return fmt.Errorf("account %q not found", account)
	case errCodeBlueprintNotFound:
		return fmt.Errorf("blueprint %q not found in account %q", name, account)
	}
	switch {
	case strings.Contains(body, "no builds found for agent"):
		return missingBuildErr(account, name, buildID, own)
	case strings.Contains(body, "account not found"):
		return fmt.Errorf("account %q not found", account)
	case strings.Contains(body, "agent not found"):
		return fmt.Errorf("blueprint %q not found in account %q", name, account)
	default:
		return fmt.Errorf("blueprint %q not found", name)
	}
}

// Only the publisher can push, so only it is told to run ast push.
func missingBuildErr(account, name, buildID string, own bool) error {
	switch {
	case !own && buildID != "":
		return errPublishedBuildNotFound(buildID, account+"/"+name)
	case !own:
		return errBlueprintNoPublishedBuild(account + "/" + name)
	case buildID != "":
		return fmt.Errorf("build %q not found for blueprint %q (run `ast push %s` or pick an existing build with `ast blueprint builds %s`)", buildID, name, name, name)
	default:
		return fmt.Errorf("blueprint %q has no builds yet (run `ast push %s` first)", name, name)
	}
}

// agent redeploy --blueprint looks only in the active account, so name --id.
func environmentOccupiedErr(at AccountToken, env, publisher, name string) error {
	if publisher != at.Account {
		return errPublishedEnvironmentOccupied(env)
	}
	return errEnvironmentOccupied(env, name)
}

// apiErrorCodeAndBody parses the embedded JSON body in an apiCall error
// (format "server returned status N: <body>") and returns its error_code
// field plus the raw body. Empty error_code means the server response is too
// old to carry a typed code so callers should fall back to message inspection.
func apiErrorCodeAndBody(err error) (code, body string) {
	if err == nil {
		return "", ""
	}
	// A typed error carries the body already parsed, so the text split below is
	// only for an error that did not come from the API helpers.
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return errorCodeFromBody(apiErr.Body), apiErr.Body
	}
	msg := err.Error()
	idx := strings.Index(msg, ": ")
	if idx == -1 {
		body = msg
	} else {
		body = msg[idx+2:]
	}
	return errorCodeFromBody(body), body
}

// errorCodeFromBody reads the typed error_code the deployment-template endpoint
// tags its 404s with. An empty result means the server is too old to send one.
func errorCodeFromBody(body string) string {
	var parsed struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return ""
	}
	return parsed.ErrorCode
}

// patchTemplateTarget sets fields of target in the deployment template JSON.
// The server leaves them out of the template signature.
func patchTemplateTarget(template json.RawMessage, fields map[string]string) (json.RawMessage, error) {
	var tmpl map[string]any
	if err := json.Unmarshal(template, &tmpl); err != nil {
		return nil, err
	}
	target, _ := tmpl["target"].(map[string]any)
	if target == nil {
		target = make(map[string]any)
	}
	for k, v := range fields {
		target[k] = v
	}
	tmpl["target"] = target
	return json.Marshal(tmpl)
}
