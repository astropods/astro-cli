package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	gitremote "github.com/astropods/astro-cli/internal/git"
)

// githubGrantPollInterval and githubGrantTimeout bound the wait for the browser
// half of the OAuth flow. The grant is recorded by the identity provider during
// the redirect, so polling the server is what tells us it landed — not anything
// the browser reports back. Both are variables so tests can shrink them.
var (
	githubGrantPollInterval = 3 * time.Second
	githubGrantTimeout      = 5 * time.Minute
)

// githubNotConnected is the server's code for an account with no usable GitHub
// grant. It arrives with 422 from every GitHub endpoint.
const githubNotConnected = "github_not_connected"

var blueprintConnectCmd = &cobra.Command{
	Use:   "connect [name]",
	Short: "Connect a GitHub repository so pushes build the blueprint",
	Long: `Connect a GitHub repository to a blueprint.

Astropods installs a push webhook on the repository, so every push to the
connected branch builds a new version of the blueprint.

With no arguments this uses the current directory's origin remote and checked
out branch, and takes the blueprint name from astropods.yml.`,
	Example: `  # Connect the current checkout to the blueprint named in astropods.yml
  ast blueprint connect

  # Connect a specific blueprint to a repository you have not cloned
  ast blueprint connect my-agent --repo https://github.com/acme/agents --branch main

  # An agent that lives in a subdirectory of a monorepo
  ast blueprint connect --path services/summarizer

  # Reinstall a webhook that failed to install the first time
  ast blueprint connect --repair`,
	Args: optionalValidAgentName,
	RunE: runBlueprintConnect,
}

// connectOptions is the whole input to the connect sequence, so that
// `blueprint create --connect` can run it without synthesizing a cobra command.
type connectOptions struct {
	Name      string
	Repo      string
	Branch    string
	Path      string
	Repair    bool
	NoBuild   bool
	NoBrowser bool
	JSON      bool
}

// connectResult is what the sequence achieved, and the --json payload.
type connectResult struct {
	Blueprint string `json:"blueprint"`
	Repo      string `json:"repo_full_name"`
	Branch    string `json:"branch"`
	// WebhookInstalled is nil when the server did not report it. That server is
	// older than the field, not a server that failed to install the webhook, and
	// the two must not read the same.
	WebhookInstalled *bool  `json:"webhook_installed"`
	BuildID          string `json:"build_id,omitempty"`
}

func init() {
	blueprintCmd.AddCommand(blueprintConnectCmd)
	registerConnectFlags(blueprintConnectCmd)
	blueprintConnectCmd.Flags().Bool("repair", false, "Reinstall the push webhook on an existing connection")
	blueprintConnectCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	blueprintConnectCmd.Flags().Bool("json", false, "Print the connection as JSON on success; progress moves to stderr")
}

// registerConnectFlags adds the flags shared by `blueprint connect` and
// `blueprint create --connect`.
func registerConnectFlags(cmd *cobra.Command) {
	cmd.Flags().String("repo", "", "Repository to connect, as a git URL or owner/repo (default: the origin remote)")
	cmd.Flags().String("branch", "", "Branch to build on push (default: the checked out branch)")
	cmd.Flags().String("path", "", "Subdirectory holding the agent, for a monorepo")
	cmd.Flags().Bool("no-build", false, "Link the repository without starting a build")
	cmd.Flags().Bool("no-browser", false, "Print the GitHub authorization URL instead of opening it")
}

func runBlueprintConnect(cmd *cobra.Command, args []string) error {
	opts := connectOptions{
		Repo:      flagString(cmd, "repo"),
		Branch:    flagString(cmd, "branch"),
		Path:      flagString(cmd, "path"),
		Repair:    flagBool(cmd, "repair"),
		NoBuild:   flagBool(cmd, "no-build"),
		NoBrowser: flagBool(cmd, "no-browser"),
		JSON:      flagBool(cmd, "json"),
	}

	if len(args) > 0 {
		opts.Name = args[0]
	} else {
		// Only the name is needed, but the spec is where an unnamed invocation
		// finds it, and a broken spec should say so rather than 404 later.
		_, name, _, err := resolveSpecAndName(cmd, args)
		if err != nil {
			return errConnectNeedsBlueprintName(err)
		}
		opts.Name = name
	}

	// Resolve before authenticating. Whether this directory has a GitHub remote
	// is a local, immediate answer, and reporting an expired session first
	// would send the reader to fix the wrong thing.
	repoFullName, branch, err := resolveRepoAndBranch(cmd.Context(), opts)
	if err != nil {
		return err
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	return connectBlueprintRepo(cmd, at, opts, repoFullName, branch, verbose)
}

// connectBlueprintRepo links a repository to a blueprint and reports whether
// pushes will build it. It is the shared body of `blueprint connect` and
// `blueprint create --connect`. Both callers resolve the repository first, so
// that neither acts on a repository it could not read.
func connectBlueprintRepo(cmd *cobra.Command, at AccountToken, opts connectOptions, repoFullName, branch string, verbose bool) error {
	ctx := cmd.Context()
	w := cmd.OutOrStdout()
	// --json owns stdout, so progress has to go somewhere a pipe will not eat.
	progress := w
	if opts.JSON {
		progress = cmd.ErrOrStderr()
	}

	if err := ensureGitHubGrant(ctx, at, opts, verbose, progress); err != nil {
		return err
	}

	result := connectResult{Blueprint: opts.Name, Repo: repoFullName, Branch: branch}

	if opts.Repair {
		installed, err := repairWebhook(ctx, at, opts.Name, verbose)
		if err != nil {
			return err
		}
		result.WebhookInstalled = &installed
	} else {
		fmt.Fprintf(progress, "%s→%s Connecting %s%s%s to %s%s%s on %s%s%s\n", //nolint:errcheck,gosec
			colorCyan, colorReset,
			colorBold, opts.Name, colorReset,
			colorBold, repoFullName, colorReset,
			colorBold, branch, colorReset)

		linked, err := linkRepo(ctx, at, opts.Name, repoFullName, branch, verbose)
		if err != nil {
			return err
		}
		result.WebhookInstalled = linked
	}

	if !opts.NoBuild {
		buildID, err := startBuild(ctx, at, opts.Name, verbose)
		if err != nil {
			// A refused build does not undo the connection, and saying so beats
			// failing a command whose main effect already succeeded.
			fmt.Fprintf(progress, "  %s%s%s\n", colorYellow, msgConnectBuildNotStarted(err), colorReset) //nolint:errcheck,gosec
		} else {
			result.BuildID = buildID
		}
	}

	if opts.JSON {
		return writeJSON(w, result)
	}
	printConnectResult(progress, result, opts.NoBuild)
	return nil
}

// resolveRepoAndBranch turns the flags plus the local checkout into the
// owner/repo[/subpath] and branch the server expects.
func resolveRepoAndBranch(ctx context.Context, opts connectOptions) (repoFullName, branch string, err error) {
	raw := opts.Repo
	if raw == "" {
		raw, err = gitremote.OriginURL(ctx, "")
		if err != nil {
			return "", "", errConnectNoRepo(err)
		}
	}

	base, err := gitremote.ParseGitHubRemote(raw)
	if err != nil {
		if errors.Is(err, gitremote.ErrNotGitHub) {
			return "", "", errConnectNotGitHub(raw)
		}
		return "", "", errConnectUnreadableRepo(raw)
	}

	repoFullName = base
	if sub := strings.Trim(opts.Path, "/"); sub != "" {
		repoFullName = base + "/" + sub
	}

	branch = opts.Branch
	if branch == "" {
		// An explicit --repo may name a repository that was never cloned here,
		// in which case the local branch is not the answer even if one exists.
		if opts.Repo != "" {
			return "", "", errConnectNeedsBranch()
		}
		branch, err = gitremote.CurrentBranch(ctx, "")
		if err != nil {
			return "", "", errConnectNeedsBranch()
		}
	}
	return repoFullName, branch, nil
}

// ensureGitHubGrant makes sure the account has a GitHub authorization the
// server can spend, sending the caller through the browser only when it does
// not. Anyone who has connected GitHub in the web app is already connected
// here: the grant is held against the same identity the CLI logs in as.
func ensureGitHubGrant(ctx context.Context, at AccountToken, opts connectOptions, verbose bool, w io.Writer) error {
	u := apiPath(blueprintBaseURL(), at.Account, "accounts", "github", "connect")
	var resp struct {
		Connected   bool   `json:"connected"`
		GitHubLogin string `json:"github_login"`
		RedirectURL string `json:"redirect_url"`
	}
	// redirect_to is where the browser lands afterwards; it is only ever seen
	// when this flow opened a browser at all.
	if _, err := apiCall(ctx, http.MethodPost, u, map[string]any{"redirect_to": "/settings/connectors"}, at.Token, verbose, &resp); err != nil {
		return errConnectGitHubCheckFailed(err)
	}
	if resp.Connected {
		return nil
	}
	if resp.RedirectURL == "" {
		return errConnectNoAuthorizationURL()
	}

	fmt.Fprintf(w, "%s→%s %s\n", colorCyan, colorReset, msgConnectGitHubAuthorizationNeeded()) //nolint:errcheck,gosec
	if opts.NoBrowser {
		fmt.Fprintf(w, "  %s\n", resp.RedirectURL) //nolint:errcheck,gosec
	} else {
		fmt.Fprintf(w, "  %s%s%s\n", colorDim, resp.RedirectURL, colorReset) //nolint:errcheck,gosec
		if openErr := browser.OpenURL(resp.RedirectURL); openErr != nil {
			fmt.Fprintf(w, "  %s%s%s\n", colorYellow, msgConnectBrowserFailed(), colorReset) //nolint:errcheck,gosec
		}
	}

	return pollGitHubGrant(ctx, at, verbose, w)
}

// pollGitHubGrant waits for the authorization to appear on the server. The
// browser is not the signal: the callback can fail to render a friendly page
// while the grant itself was recorded, so the server's own view decides.
func pollGitHubGrant(ctx context.Context, at AccountToken, verbose bool, w io.Writer) error {
	u := apiPath(blueprintBaseURL(), at.Account, "accounts", "github")
	deadline := time.Now().Add(githubGrantTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(githubGrantPollInterval):
		}

		var status struct {
			Connected   bool   `json:"connected"`
			GitHubLogin string `json:"github_login"`
		}
		if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &status); err != nil {
			continue
		}
		if status.Connected {
			green := color.New(color.FgGreen)
			green.Fprintf(w, "  ✓ %s\n", msgConnectGitHubAuthorized(status.GitHubLogin)) //nolint:errcheck,gosec
			return nil
		}
		fmt.Fprint(w, ".") //nolint:errcheck,gosec
	}
	return errConnectGitHubTimedOut()
}

// linkRepo saves the connection and installs the push webhook. The returned
// pointer is nil when the server does not report webhook state.
func linkRepo(ctx context.Context, at AccountToken, name, repoFullName, branch string, verbose bool) (*bool, error) {
	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "github", "link")
	var resp struct {
		RepoFullName     string `json:"repo_full_name"`
		Branch           string `json:"branch"`
		WebhookInstalled *bool  `json:"webhook_installed"`
	}
	status, err := apiCall(ctx, http.MethodPost, u, map[string]string{
		"repo_full_name": repoFullName,
		"branch":         branch,
	}, at.Token, verbose, &resp)

	switch status {
	case http.StatusConflict:
		return nil, errConnectRepoTakenByAnotherBlueprint(repoFullName, apiErrorMessage(err))
	case http.StatusForbidden:
		return nil, errConnectOwnedByAnotherMember()
	case http.StatusNotFound:
		return nil, errConnectBlueprintNotFound(name, at.Account)
	case http.StatusUnprocessableEntity:
		if apiErrorMessage(err) == githubNotConnected {
			return nil, errConnectGitHubDisconnected()
		}
		return nil, errConnectRejected(apiErrorMessage(err))
	}
	if err != nil {
		return nil, err
	}
	return resp.WebhookInstalled, nil
}

// repairWebhook reinstalls a push webhook for a connection that already exists.
func repairWebhook(ctx context.Context, at AccountToken, name string, verbose bool) (bool, error) {
	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "github", "webhook", "repair")
	var resp struct {
		WebhookInstalled bool `json:"webhook_installed"`
	}
	status, err := apiCall(ctx, http.MethodPost, u, nil, at.Token, verbose, &resp)
	if status == http.StatusNotFound {
		// A server without the repair endpoint 404s the route itself, which
		// arrives as plain text rather than the JSON error the handler sends
		// for a blueprint that has nothing to repair. Reporting "no connection"
		// for an absent endpoint would send the reader to fix the wrong thing.
		if apiErrorMessage(err) == "" {
			return false, errConnectRepairUnsupported()
		}
		return false, errConnectNothingToRepair(name)
	}
	if err != nil {
		return false, err
	}
	return resp.WebhookInstalled, nil
}

// startBuild enqueues a build of the connected branch's current commit, so the
// blueprint has a version without waiting for the next push.
func startBuild(ctx context.Context, at AccountToken, name string, verbose bool) (string, error) {
	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "github", "rebuild")
	var resp struct {
		BuildID string `json:"build_id"`
	}
	if _, err := apiCall(ctx, http.MethodPost, u, nil, at.Token, verbose, &resp); err != nil {
		return "", err
	}
	return resp.BuildID, nil
}

// apiErrorMessage returns the server's own `error` field, which the GitHub
// endpoints use to distinguish causes that share a status code.
func apiErrorMessage(err error) string {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	return ""
}

func printConnectResult(w io.Writer, result connectResult, noBuild bool) {
	green := color.New(color.FgGreen)

	switch {
	case result.WebhookInstalled == nil:
		// An older server cannot say, and claiming either way would be a guess.
		green.Fprintf(w, "  ✓ %s\n", msgConnectLinked(result.Repo, result.Branch))     //nolint:errcheck,gosec
		fmt.Fprintf(w, "  %s%s%s\n", colorDim, msgConnectWebhookUnknown(), colorReset) //nolint:errcheck,gosec
	case *result.WebhookInstalled:
		green.Fprintf(w, "  ✓ %s\n", msgConnectLinkedAndBuilding(result.Repo, result.Branch)) //nolint:errcheck,gosec
	default:
		green.Fprintf(w, "  ✓ %s\n", msgConnectLinked(result.Repo, result.Branch))           //nolint:errcheck,gosec
		fmt.Fprintf(w, "  %s%s%s\n", colorYellow, msgConnectWebhookMissing(), colorReset)    //nolint:errcheck,gosec
		fmt.Fprintf(w, "  %s%s%s\n", colorDim, msgConnectWebhookMissingRemedy(), colorReset) //nolint:errcheck,gosec
	}

	if result.BuildID != "" {
		fmt.Fprintf(w, "  %s%s%s\n", colorDim, msgConnectBuildStarted(result.BuildID), colorReset) //nolint:errcheck,gosec
	} else if noBuild {
		fmt.Fprintf(w, "  %s%s%s\n", colorDim, msgConnectNoBuildRequested(), colorReset) //nolint:errcheck,gosec
	}
}
