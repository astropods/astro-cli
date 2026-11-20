package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	gitrepo "github.com/astropods/astro-cli/internal/git"
)

// githubGrantPollInterval and githubGrantTimeout bound the wait while the
// caller authorizes GitHub in a browser. Variables so tests can shrink them.
var (
	githubGrantPollInterval = 2 * time.Second
	githubGrantTimeout      = 5 * time.Minute
)

// githubAuthorizeLandingPath is where the browser lands once GitHub is
// authorized. The server accepts only a web-app path, never a URL, so the CLI
// can't receive the callback itself; it asks the server instead.
const githubAuthorizeLandingPath = "/settings/connectors"

var blueprintConnectCmd = &cobra.Command{
	Use:   "connect [name]",
	Short: "Connect a GitHub repository so pushes build the blueprint",
	Long: `Connect a GitHub repository to a blueprint, then build the branch's latest commit.
After that, every push to the branch builds a new version of the blueprint.

With no arguments this uses the current directory's origin remote and checked
out branch, takes the blueprint name from astropods.yml, and connects the
directory holding astropods.yml when it isn't the repository root.

The first time, your browser opens so you can authorize GitHub. If you've
already connected GitHub in the web app, it doesn't.`,
	Example: "  # Connect the current checkout to the blueprint named in astropods.yml\n" +
		"  ast blueprint connect\n\n" +
		"  # Connect a blueprint to a repository you haven't cloned\n" +
		"  ast blueprint connect my-agent --repo acme/agents --branch main\n\n" +
		"  # Connect an agent that lives in a monorepo subdirectory\n" +
		"  ast blueprint connect --path services/summarizer",
	Args: optionalValidAgentName,
	RunE: runBlueprintConnect,
}

func init() {
	blueprintCmd.AddCommand(blueprintConnectCmd)
	registerConnectFlags(blueprintConnectCmd)
	blueprintConnectCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	blueprintConnectCmd.Flags().Bool("json", false, "Print the connection as JSON on success; progress moves to stderr")
}

// registerConnectFlags adds the flags shared by `blueprint connect` and
// `blueprint create --connect`.
func registerConnectFlags(cmd *cobra.Command) {
	cmd.Flags().String("repo", "", "Repository to connect, as owner/repo or a GitHub URL (default: the origin remote)")
	cmd.Flags().String("branch", "", "Branch to build on push (default: the checked out branch, or the repository's default with --repo)")
	cmd.Flags().String("path", "", "Subdirectory holding the agent (default: the directory holding astropods.yml)")
	cmd.Flags().Bool("no-build", false, "Connect without starting a build")
	cmd.Flags().Bool("no-browser", false, "Print the GitHub authorization link instead of opening it")
}

type connectOptions struct {
	Repo      string
	Branch    string
	Path      string
	PathSet   bool
	NoBuild   bool
	NoBrowser bool
}

func connectOptionsFromFlags(cmd *cobra.Command) connectOptions {
	return connectOptions{
		Repo:      flagString(cmd, "repo"),
		Branch:    flagString(cmd, "branch"),
		Path:      flagString(cmd, "path"),
		PathSet:   cmd.Flags().Changed("path"),
		NoBuild:   flagBool(cmd, "no-build"),
		NoBrowser: flagBool(cmd, "no-browser"),
	}
}

// connectTarget is the repository to connect, as far as the flags and the
// local checkout can say. Branch is empty when the repository's default
// branch should be used.
type connectTarget struct {
	Repo    string
	SubPath string
	Branch  string
}

// preparedConnection is a target the server has confirmed: GitHub's casing of
// the repository, a branch that exists on GitHub, and whether that branch has
// a spec to build.
type preparedConnection struct {
	Base        string
	SubPath     string
	Branch      string
	SpecFound   bool
	GitHubLogin string
}

func (p preparedConnection) repoFullName() string {
	if p.SubPath == "" {
		return p.Base
	}
	return p.Base + "/" + p.SubPath
}

type connectResult struct {
	Blueprint string `json:"blueprint"`
	Repo      string `json:"repo_full_name"`
	Branch    string `json:"branch"`
	BuildID   string `json:"build_id,omitempty"`
}

func runBlueprintConnect(cmd *cobra.Command, args []string) error {
	opts := connectOptionsFromFlags(cmd)
	name, specDir, err := resolveConnectBlueprint(cmd, args)
	if err != nil {
		return err
	}
	// Resolve before authenticating: a directory with no GitHub remote is a
	// local answer, and an expired session reported first would send the
	// reader to fix the wrong thing.
	target, err := resolveConnectTarget(opts, specDir)
	if err != nil {
		return err
	}
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	jsonOut := flagBool(cmd, "json")
	progress := cmd.OutOrStdout()
	if jsonOut {
		progress = cmd.ErrOrStderr()
	}
	prepared, err := prepareConnection(cmd.Context(), at, target, opts.NoBrowser, verbose, progress)
	if err != nil {
		return err
	}
	result, err := finishConnection(cmd.Context(), at, name, prepared, opts.NoBuild, verbose, progress)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	return nil
}

// resolveConnectBlueprint returns the blueprint name and the directory holding
// its spec. The spec names the blueprint when no argument does, and its
// directory is the agent's place in the repository. specDir is "" (the working
// directory) when there is no spec.
func resolveConnectBlueprint(cmd *cobra.Command, args []string) (name, specDir string, err error) {
	if len(args) > 0 {
		name = args[0]
	}
	specFile := flagString(cmd, "file")
	specPath, err := resolveSpecPathFromCwd(specFile)
	if err != nil {
		if name == "" {
			return "", "", errConnectNeedsBlueprintName()
		}
		if specFile != "" {
			return "", "", err
		}
		return name, "", nil
	}
	if name == "" {
		if name, err = specFileBlueprintName(specPath); err != nil {
			return "", "", err
		}
	}
	return name, filepath.Dir(specPath), nil
}

// resolveConnectTarget turns the flags plus the checkout in dir into the
// repository, subdirectory, and branch to connect.
func resolveConnectTarget(opts connectOptions, dir string) (connectTarget, error) {
	raw := opts.Repo
	fromCheckout := raw == ""
	if fromCheckout {
		var err error
		if raw, err = gitrepo.OriginURL(dir); err != nil {
			return connectTarget{}, errConnectNoRemote(err)
		}
	}
	repo, err := gitrepo.ParseGitHubRemote(raw)
	if errors.Is(err, gitrepo.ErrNotGitHub) {
		return connectTarget{}, errConnectNotGitHub(raw)
	}
	if err != nil {
		return connectTarget{}, errConnectUnreadableRepo(raw)
	}

	target := connectTarget{Repo: repo, SubPath: strings.Trim(opts.Path, "/"), Branch: opts.Branch}
	// The local checkout only describes --repo when --repo is absent; a named
	// repository may never have been cloned here.
	if !fromCheckout {
		return target, nil
	}
	if !opts.PathSet {
		if target.SubPath, err = gitrepo.RepoPrefix(dir); err != nil {
			return connectTarget{}, errConnectNoRemote(err)
		}
	}
	if target.Branch == "" {
		if target.Branch, err = gitrepo.CurrentBranch(dir); err != nil {
			return connectTarget{}, errConnectNeedsBranch(err)
		}
	}
	return target, nil
}

// prepareConnection runs every check that can refuse the connection, so a
// caller that creates the blueprint first has nothing to undo.
func prepareConnection(ctx context.Context, at AccountToken, target connectTarget, noBrowser, verbose bool, w io.Writer) (preparedConnection, error) {
	login, err := ensureGitHubGrant(ctx, at, noBrowser, verbose, w)
	if err != nil {
		return preparedConnection{}, err
	}

	// The server stores the name as given and matches push events by exact
	// string, so a remote with different casing would connect and never build.
	repo, err := getGitHubRepo(ctx, at, target.Repo, verbose)
	if err != nil {
		return preparedConnection{}, err
	}
	prepared := preparedConnection{
		Base:        repo.RepoFullName,
		SubPath:     target.SubPath,
		Branch:      target.Branch,
		GitHubLogin: login,
	}
	if prepared.Branch == "" {
		prepared.Branch = repo.DefaultBranch
	}

	// The server links a branch it can't read and only fails at the first
	// build, typically because the branch hasn't been pushed yet.
	scan, err := scanGitHubRepo(ctx, at, prepared.repoFullName(), prepared.Branch, verbose)
	if err != nil {
		return preparedConnection{}, err
	}
	if !scan.Readable {
		return preparedConnection{}, errConnectBranchNotOnGitHub(prepared.Base, prepared.Branch)
	}
	prepared.SpecFound = scan.Found
	return prepared, nil
}

// finishConnection links the prepared repository to the blueprint and starts
// its first build.
func finishConnection(ctx context.Context, at AccountToken, name string, p preparedConnection, noBuild, verbose bool, w io.Writer) (connectResult, error) {
	repo := p.repoFullName()
	fmt.Fprintf(w, "%s→%s %s\n", colorCyan, colorReset, msgConnecting(name, repo, p.Branch)) //nolint:errcheck,gosec
	if err := linkGitHubRepo(ctx, at, name, repo, p.Branch, verbose); err != nil {
		return connectResult{}, err
	}
	fmt.Fprintf(w, "%s✓%s %s\n", colorGreen, colorReset, msgConnected(name, repo, p.Branch)) //nolint:errcheck,gosec
	if p.GitHubLogin != "" {
		fmt.Fprintln(w, colorDim+msgConnectBuildsUseLogin(p.GitHubLogin)+colorReset) //nolint:errcheck,gosec
	}

	result := connectResult{Blueprint: name, Repo: repo, Branch: p.Branch}
	switch {
	case noBuild:
	case !p.SpecFound:
		fmt.Fprintf(w, "%s%s%s\n", colorYellow, msgConnectNoSpecOnBranch(repo, p.Branch), colorReset) //nolint:errcheck,gosec
	default:
		started, err := startServerBuild(ctx, at, name, verbose)
		if err != nil {
			// The connection is the command's main effect and it stands, so a
			// refused build is a warning, not a failure.
			fmt.Fprintf(w, "%s%s%s\n", colorYellow, msgConnectBuildNotStarted(name, explainError(err)), colorReset) //nolint:errcheck,gosec
			break
		}
		result.BuildID = started.BuildID
		fmt.Fprintf(w, "%s✓%s %s\n", colorGreen, colorReset, msgConnectBuildStarted(started.BuildID, started.CommitSHA, started.CommitMessage)) //nolint:errcheck,gosec
		fmt.Fprintln(w, colorDim+msgTailBuildLogs(name, started.BuildID)+colorReset)                                                            //nolint:errcheck,gosec
	}
	return result, nil
}

// ensureGitHubGrant makes sure the caller has a GitHub authorization the server
// can use, and returns its GitHub login when the server reports one. Anyone who
// connected GitHub in the web app already has one: it belongs to the same
// identity the CLI signs in as.
func ensureGitHubGrant(ctx context.Context, at AccountToken, noBrowser, verbose bool, w io.Writer) (string, error) {
	u := apiPath(blueprintBaseURL(), at.Account, "accounts", "github", "connect")
	var resp struct {
		Connected   bool   `json:"connected"`
		GitHubLogin string `json:"github_login"`
		RedirectURL string `json:"redirect_url"`
	}
	body := map[string]any{"redirect_to": githubAuthorizeLandingPath, "force": false}
	if _, err := apiCall(ctx, http.MethodPost, u, body, at.Token, verbose, &resp); err != nil {
		return "", errConnectGitHubCheckFailed(err)
	}
	if resp.Connected {
		return resp.GitHubLogin, nil
	}
	if nonInteractive() {
		return "", errConnectGitHubAuthorizationRequired(resp.RedirectURL)
	}

	fmt.Fprintf(w, "%s→%s %s\n", colorCyan, colorReset, msgConnectAuthorizeGitHub()) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %s\n", resp.RedirectURL)                                       //nolint:errcheck,gosec
	if !noBrowser {
		if err := browser.OpenURL(resp.RedirectURL); err != nil {
			fmt.Fprintf(w, "  %s%s%s\n", colorYellow, msgConnectBrowserFailed(), colorReset) //nolint:errcheck,gosec
		}
	}
	return pollGitHubGrant(ctx, at, resp.RedirectURL, verbose, w)
}

// pollGitHubGrant waits for the server to see the authorization. The server is
// the signal, not the browser: the grant is recorded during GitHub's redirect,
// even when the page the browser lands on can't show it.
func pollGitHubGrant(ctx context.Context, at AccountToken, authorizeURL string, verbose bool, w io.Writer) (string, error) {
	fmt.Fprintf(w, "  %s%s%s\n", colorDim, msgConnectWaitingForGitHub(), colorReset) //nolint:errcheck,gosec
	u := apiPath(blueprintBaseURL(), at.Account, "accounts", "github")
	deadline := time.Now().Add(githubGrantTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(githubGrantPollInterval):
		}
		var status struct {
			Connected   bool   `json:"connected"`
			GitHubLogin string `json:"github_login"`
		}
		if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &status); err != nil {
			return "", errConnectGitHubCheckFailed(err)
		}
		if status.Connected {
			fmt.Fprintf(w, "%s✓%s %s\n", colorGreen, colorReset, msgConnectGitHubAuthorized(status.GitHubLogin)) //nolint:errcheck,gosec
			return status.GitHubLogin, nil
		}
	}
	return "", errConnectGitHubAuthorizationTimedOut(authorizeURL)
}

type githubRepo struct {
	RepoFullName  string `json:"repo_full_name"`
	DefaultBranch string `json:"default_branch"`
}

// getGitHubRepo returns repo as the caller's GitHub authorization sees it, in
// GitHub's own casing.
func getGitHubRepo(ctx context.Context, at AccountToken, repo string, verbose bool) (githubRepo, error) {
	u := apiPath(blueprintBaseURL(), at.Account, "accounts", "github", "repo") + "?" + url.Values{"repo": {repo}}.Encode()
	var found githubRepo
	if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &found); err != nil {
		return githubRepo{}, explainGitHubError(err, repo)
	}
	return found, nil
}

type githubScan struct {
	Found    bool `json:"found"`
	Readable bool `json:"readable"`
}

// scanGitHubRepo reports whether branch exists on GitHub and has an
// astropods.yml at repoFullName's subdirectory.
func scanGitHubRepo(ctx context.Context, at AccountToken, repoFullName, branch string, verbose bool) (githubScan, error) {
	q := url.Values{"repo": {repoFullName}, "branch": {branch}}
	u := apiPath(blueprintBaseURL(), at.Account, "accounts", "github", "scan") + "?" + q.Encode()
	var scan githubScan
	if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &scan); err != nil {
		return githubScan{}, explainGitHubError(err, repoFullName)
	}
	return scan, nil
}

// checkRepoNotConnected refuses a repository and subdirectory already connected
// to another blueprint in the account. Linking checks this too, but only after
// `create --connect` has already created the blueprint.
func checkRepoNotConnected(ctx context.Context, at AccountToken, p preparedConnection, verbose bool) error {
	u := apiPath(blueprintBaseURL(), at.Account, "accounts", "github", "connections")
	var resp struct {
		Connections []struct {
			AgentName    string `json:"agent_name"`
			RepoFullName string `json:"repo_full_name"`
		} `json:"connections"`
	}
	if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &resp); err != nil {
		return err
	}
	for _, c := range resp.Connections {
		if sameGitHubRepository(c.RepoFullName, p.Base, p.SubPath) {
			return errConnectRepoConnectedTo(p.repoFullName(), c.AgentName)
		}
	}
	return nil
}

// sameGitHubRepository matches the server's rule: owner and repository ignore
// case, the subdirectory doesn't.
func sameGitHubRepository(repoFullName, base, subPath string) bool {
	parts := strings.SplitN(strings.Trim(repoFullName, "/"), "/", 3)
	if len(parts) < 2 {
		return false
	}
	gotSub := ""
	if len(parts) == 3 {
		gotSub = parts[2]
	}
	return strings.EqualFold(parts[0]+"/"+parts[1], base) && gotSub == subPath
}

// linkGitHubRepo saves the connection. The server installs the push webhook
// first and refuses the link if it can't.
func linkGitHubRepo(ctx context.Context, at AccountToken, name, repoFullName, branch string, verbose bool) error {
	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "github", "link")
	body := map[string]string{"repo_full_name": repoFullName, "branch": branch}
	status, err := apiCall(ctx, http.MethodPost, u, body, at.Token, verbose, nil)
	if status == http.StatusConflict {
		return errConnectRepoAlreadyConnected(repoFullName, serverSentence(apiErrorMessage(err)))
	}
	if err != nil {
		return explainGitHubError(err, repoFullName)
	}
	return nil
}

// explainGitHubError explains the error codes the GitHub endpoints share.
// Anything else keeps the server's own explanation.
func explainGitHubError(err error, repo string) error {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Message {
	case "github_not_connected":
		return errConnectGitHubNotConnected()
	case "repository_not_found":
		return errConnectRepoNotFound(repo)
	case "repository_access_denied":
		return errConnectRepoAccessDenied(repo)
	case "repository_push_required":
		return errConnectRepoPushRequired(repo)
	case "github_webhook_permission_required":
		return errConnectWebhookPermission(repo)
	case "webhook_setup_unavailable":
		return errConnectWebhookUnavailable()
	case "rate_limit_exceeded":
		return errConnectGitHubRateLimited()
	}
	return err
}
