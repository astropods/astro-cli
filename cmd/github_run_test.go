package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/githubcli"
)

// withFakeGH fakes every gh/git seam for the duration of the test.
func withFakeGH(t *testing.T,
	run func(ctx context.Context, dir, token string, args ...string) (githubcli.Result, error),
	authenticated func(ctx context.Context, dir string) error,
	push func(ctx context.Context, dir, token, remote, branch string, setUpstream bool) (githubcli.Result, error),
	currentBranch func(ctx context.Context, dir string) (string, error),
) {
	t.Helper()
	prevRun, prevAuth, prevPush, prevBranch := ghRun, ghAuthenticated, ghPush, ghCurrentBranch
	if run != nil {
		ghRun = run
	}
	if authenticated != nil {
		ghAuthenticated = authenticated
	}
	if push != nil {
		ghPush = push
	}
	if currentBranch != nil {
		ghCurrentBranch = currentBranch
	}
	t.Setenv(sandboxTokenEnvVar, "")
	t.Cleanup(func() {
		ghRun, ghAuthenticated, ghPush, ghCurrentBranch = prevRun, prevAuth, prevPush, prevBranch
	})
}

// newTestCmd wraps source's own flag set (not a copy: AddFlagSet shares the
// underlying *Flag pointers), so every value set here is reset to its
// default on cleanup, since source's flags outlive this one test.
func newTestCmd(t *testing.T, flags map[string]string, source *cobra.Command) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: source.Use}
	cmd.Flags().AddFlagSet(source.Flags())
	for name, value := range flags {
		f := cmd.Flags().Lookup(name)
		require.NotNil(t, f, name)
		def := f.DefValue
		require.NoError(t, cmd.Flags().Set(name, value))
		t.Cleanup(func() {
			cmd.Flags().Set(name, def) //nolint:errcheck,gosec
			f.Changed = false
		})
	}
	cmd.SetContext(context.Background())
	root := &cobra.Command{Use: "ast"}
	root.PersistentFlags().Bool("verbose", false, "")
	root.AddCommand(cmd)
	cmd.SetArgs(nil)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	return cmd
}

func outputOf(cmd *cobra.Command) string {
	buf, ok := cmd.OutOrStdout().(*bytes.Buffer)
	if !ok {
		return ""
	}
	return buf.String()
}

func TestGHAuthPrefersTheDevSessionConnectionToken(t *testing.T) {
	withFakeGH(t, nil, func(context.Context, string) error {
		t.Fatal("should not check local gh auth when a dev session token resolves")
		return nil
	}, nil, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(issueConnectionTokenResponse{AccessToken: "gho_fromconnection"})) //nolint:errcheck,gosec
	}))
	defer srv.Close()
	prevURL := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prevURL })
	t.Setenv(sandboxTokenEnvVar, "a-dev-session-bearer")

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	token, err := ghAuth(cmd)

	require.NoError(t, err)
	assert.Equal(t, "gho_fromconnection", token)
}

func TestGHAuthFallsBackToLocalLoginWithNoDevSession(t *testing.T) {
	var checked bool
	withFakeGH(t, nil, func(context.Context, string) error {
		checked = true
		return nil
	}, nil, nil)

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	token, err := ghAuth(cmd)

	require.NoError(t, err)
	assert.Empty(t, token)
	assert.True(t, checked, "with no dev session, ghAuth must check the operator's own gh login")
}

func TestGHAuthReportsMissingBinaryDistinctlyFromUnauthenticated(t *testing.T) {
	withFakeGH(t, nil, func(context.Context, string) error {
		return githubcli.ErrNotInstalled{Name: "gh"}
	}, nil, nil)

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	_, err := ghAuth(cmd)

	require.Error(t, err)
	assert.Equal(t, errGHNotInstalled().Error(), err.Error())
}

func TestGHAuthWrapsAGenericAuthFailure(t *testing.T) {
	authErr := errors.New("401")
	withFakeGH(t, nil, func(context.Context, string) error {
		return authErr
	}, nil, nil)

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	_, err := ghAuth(cmd)

	require.Error(t, err)
	assert.Equal(t, errGHNotAuthenticated(authErr).Error(), err.Error())
}

func TestRunGitHubPRViewBuildsExpectedArgs(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		flags map[string]string
		want  []string
	}{
		{name: "no number, no json", want: []string{"pr", "view"}},
		{name: "with number", args: []string{"42"}, want: []string{"pr", "view", "--", "42"}},
		{name: "with json fields", flags: map[string]string{"json": "number,title"}, want: []string{"pr", "view", "--json", "number,title"}},
		{name: "with repo", args: []string{"42"}, flags: map[string]string{"repo": "astropods/astro-cli"}, want: []string{"pr", "view", "-R", "astropods/astro-cli", "--", "42"}},
		{name: "number starting with a dash is still passed as positional, not a flag", args: []string{"--force"}, want: []string{"pr", "view", "--", "--force"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotArgs []string
			withFakeGH(t, func(_ context.Context, _, _ string, args ...string) (githubcli.Result, error) {
				gotArgs = args
				return githubcli.Result{Stdout: "ok"}, nil
			}, func(context.Context, string) error { return nil }, nil, nil)

			cmd := newTestCmd(t, tc.flags, githubPRViewCmd)
			err := runGitHubPRView(cmd, tc.args)

			require.NoError(t, err)
			assert.Equal(t, tc.want, gotArgs)
			assert.Equal(t, "ok\n", outputOf(cmd))
		})
	}
}

func TestRunGitHubPRCreateBuildsExpectedArgs(t *testing.T) {
	var gotArgs []string
	withFakeGH(t, func(_ context.Context, _, _ string, args ...string) (githubcli.Result, error) {
		gotArgs = args
		return githubcli.Result{Stdout: "https://github.com/o/r/pull/1"}, nil
	}, func(context.Context, string) error { return nil }, nil, nil)

	cmd := newTestCmd(t, map[string]string{
		"title": "Fix the thing",
		"body":  "Fixes the thing.",
		"base":  "main",
		"head":  "fix/thing",
		"draft": "true",
		"repo":  "astropods/astro-cli",
	}, githubPRCreateCmd)
	err := runGitHubPRCreate(cmd, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"pr", "create", "--title", "Fix the thing", "--body", "Fixes the thing.",
		"--base", "main", "--head", "fix/thing", "--draft", "-R", "astropods/astro-cli",
	}, gotArgs)
}

func TestRunGitHubPRCommentPassesBodyAndNumber(t *testing.T) {
	var gotArgs []string
	withFakeGH(t, func(_ context.Context, _, _ string, args ...string) (githubcli.Result, error) {
		gotArgs = args
		return githubcli.Result{}, nil
	}, func(context.Context, string) error { return nil }, nil, nil)

	cmd := newTestCmd(t, map[string]string{"body": "Looks good.", "repo": "astropods/astro-cli"}, githubPRCommentCmd)
	err := runGitHubPRComment(cmd, []string{"7"})

	require.NoError(t, err)
	assert.Equal(t, []string{"pr", "comment", "--body", "Looks good.", "-R", "astropods/astro-cli", "--", "7"}, gotArgs)
}

func TestRunGitHubPRChecksBuildsExpectedArgs(t *testing.T) {
	var gotArgs []string
	withFakeGH(t, func(_ context.Context, _, _ string, args ...string) (githubcli.Result, error) {
		gotArgs = args
		return githubcli.Result{}, nil
	}, func(context.Context, string) error { return nil }, nil, nil)

	cmd := newTestCmd(t, map[string]string{"json": "name,state", "repo": "astropods/astro-cli"}, githubPRChecksCmd)
	err := runGitHubPRChecks(cmd, []string{"12"})

	require.NoError(t, err)
	assert.Equal(t, []string{"pr", "checks", "--json", "name,state", "-R", "astropods/astro-cli", "--", "12"}, gotArgs)
}

func TestRunGitHubIssueViewBuildsExpectedArgs(t *testing.T) {
	var gotArgs []string
	withFakeGH(t, func(_ context.Context, _, _ string, args ...string) (githubcli.Result, error) {
		gotArgs = args
		return githubcli.Result{}, nil
	}, func(context.Context, string) error { return nil }, nil, nil)

	cmd := newTestCmd(t, map[string]string{"json": "number,title", "repo": "astropods/astro-cli"}, githubIssueViewCmd)
	err := runGitHubIssueView(cmd, []string{"9"})

	require.NoError(t, err)
	assert.Equal(t, []string{"issue", "view", "--json", "number,title", "-R", "astropods/astro-cli", "--", "9"}, gotArgs)
}

func TestRunGitHubPushResolvesBranchAndForwardsToken(t *testing.T) {
	var gotRemote, gotBranch, gotToken string
	var gotSetUpstream bool
	withFakeGH(t, nil,
		func(context.Context, string) error {
			t.Fatal("push must never check gh auth: it only runs git, which has its own credential resolution")
			return nil
		},
		func(_ context.Context, _, token, remote, branch string, setUpstream bool) (githubcli.Result, error) {
			gotToken, gotRemote, gotBranch, gotSetUpstream = token, remote, branch, setUpstream
			return githubcli.Result{Stdout: "pushed"}, nil
		},
		func(context.Context, string) (string, error) { return "feature/x", nil },
	)

	cmd := newTestCmd(t, map[string]string{"remote": "origin", "set-upstream": "true"}, githubPushCmd)
	err := runGitHubPush(cmd, nil)

	require.NoError(t, err)
	assert.Equal(t, "origin", gotRemote)
	assert.Equal(t, "feature/x", gotBranch)
	assert.True(t, gotSetUpstream)
	assert.Empty(t, gotToken, "with no dev session open, push should use the operator's own git credentials")
	assert.Contains(t, outputOf(cmd), "pushed")
}

func TestRunGitHubPushNeverPrintsStderrTwiceOnFailure(t *testing.T) {
	// Regression test: runCommand's real error already embeds stderr (see
	// internal/githubcli.runCommand), so writing result.Stderr separately on
	// a failed push showed the same git error text twice. Fakes ghPush the
	// way runCommand actually behaves: the error's own text carries stderr.
	stderrText := "remote: permission denied"
	pushErr := fmt.Errorf("git push origin feature/x: exit status 128: %s", stderrText)
	withFakeGH(t, nil, nil,
		func(context.Context, string, string, string, string, bool) (githubcli.Result, error) {
			return githubcli.Result{Stderr: stderrText}, pushErr
		},
		func(context.Context, string) (string, error) { return "feature/x", nil },
	)

	cmd := newTestCmd(t, map[string]string{"remote": "origin"}, githubPushCmd)
	err := runGitHubPush(cmd, nil)

	require.Error(t, err)
	combined := outputOf(cmd) + err.Error()
	assert.Equal(t, 1, strings.Count(combined, stderrText),
		"stderr text appeared %d times in %q; it must appear exactly once between stdout and the error", strings.Count(combined, stderrText), combined)
}

func TestRunGitHubPushRefusesDetachedHead(t *testing.T) {
	// Regression test: CurrentBranch returns the literal string "HEAD" in a
	// detached checkout (not an error), and forwarding that straight to
	// `git push` as the branch name produced a cryptic git-internals error
	// instead of a clear one.
	var pushCalled bool
	withFakeGH(t, nil, nil,
		func(context.Context, string, string, string, string, bool) (githubcli.Result, error) {
			pushCalled = true
			return githubcli.Result{}, nil
		},
		func(context.Context, string) (string, error) { return "HEAD", nil },
	)

	cmd := newTestCmd(t, map[string]string{"remote": "origin"}, githubPushCmd)
	err := runGitHubPush(cmd, nil)

	require.Error(t, err)
	assert.Equal(t, errGitHubDetachedHead().Error(), err.Error())
	assert.False(t, pushCalled, "must not attempt a push when not on a real branch")
}

func TestGHAuthAppliesATimeoutToTheAuthCheck(t *testing.T) {
	// Regression test: gh auth status validates the token against GitHub's
	// API, so with no dev session open, a slow or blocked network used to
	// hang every gh-based command indefinitely, same as the already-fixed
	// connections-token fetch, one call deeper.
	withFakeGH(t, nil, func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}, nil, nil)

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	start := time.Now()
	_, err := ghAuth(cmd)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, ghAuthCheckTimeout+2*time.Second,
		"took %s; ghAuth must give up at roughly ghAuthCheckTimeout, not hang forever", elapsed)
}
