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

func TestGHAuthDistinguishesADevSessionTokenFailureFromNoDevSessionAtAll(t *testing.T) {
	authErr := errors.New("401")
	withFakeGH(t, nil, func(context.Context, string) error {
		return authErr
	}, nil, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	prevURL := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prevURL })
	t.Setenv(sandboxTokenEnvVar, "the-dev-session-bearer")

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	_, err := ghAuth(cmd)

	require.Error(t, err)
	assert.Equal(t, errGHDevSessionTokenUnavailable(authErr).Error(), err.Error())
}

func TestRunGHPassthroughNamesTheDevSessionTokenOnFailure(t *testing.T) {
	// gh's own error text may suggest `gh auth login`, which has no effect
	// once GH_TOKEN overrides it.
	withFakeGH(t, func(context.Context, string, string, ...string) (githubcli.Result, error) {
		return githubcli.Result{}, errors.New("HTTP 401: Bad credentials")
	}, nil, nil, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(issueConnectionTokenResponse{AccessToken: "gho_bad"})) //nolint:errcheck,gosec
	}))
	defer srv.Close()
	prevURL := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prevURL })
	t.Setenv(sandboxTokenEnvVar, "the-dev-session-bearer")

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	err := runGHPassthrough(cmd, "pr", "view")

	require.Error(t, err)
	assert.Equal(t, errGitHubCommandFailedWithDevSessionToken(errors.New("HTTP 401: Bad credentials")).Error(), err.Error())
}

func TestRunGHPassthroughUsesThePlainErrorWithNoDevSession(t *testing.T) {
	ghErr := errors.New("HTTP 404: Not Found")
	withFakeGH(t, func(context.Context, string, string, ...string) (githubcli.Result, error) {
		return githubcli.Result{}, ghErr
	}, func(context.Context, string) error { return nil }, nil, nil)

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	err := runGHPassthrough(cmd, "pr", "view")

	require.Error(t, err)
	assert.Equal(t, errGitHubCommandFailed(ghErr).Error(), err.Error())
}

func TestRunGitHubPushReportsGitNotInstalledDistinctly(t *testing.T) {
	withFakeGH(t, nil, nil, nil, func(context.Context, string) (string, error) {
		return "", githubcli.ErrNotInstalled{Name: "git"}
	})

	cmd := newTestCmd(t, map[string]string{"remote": "origin"}, githubPushCmd)
	err := runGitHubPush(cmd, nil)

	require.Error(t, err)
	assert.Equal(t, errGitNotInstalled().Error(), err.Error())
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
		{name: "--fields selects specific fields", flags: map[string]string{"fields": "number,title"}, want: []string{"pr", "view", "--json", "number,title"}},
		{name: "--json alone uses the default field set", flags: map[string]string{"json": "true"}, want: []string{"pr", "view", "--json", prViewJSONFields}},
		{name: "--fields wins over --json", flags: map[string]string{"json": "true", "fields": "number"}, want: []string{"pr", "view", "--json", "number"}},
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

	cmd := newTestCmd(t, map[string]string{"fields": "name,state", "repo": "astropods/astro-cli"}, githubPRChecksCmd)
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

	cmd := newTestCmd(t, map[string]string{"fields": "number,title", "repo": "astropods/astro-cli"}, githubIssueViewCmd)
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
	// Fakes ghPush the way runCommand actually behaves: the error's own
	// text already carries stderr (see internal/githubcli.runCommand).
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
	// CurrentBranch returns the literal string "HEAD" in a detached
	// checkout, not an error.
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
	// Shortened rather than slept out: the real 10s says nothing extra, and
	// asserting "finished within the constant + 2s" holds for any constant.
	prev := ghAuthCheckTimeout
	ghAuthCheckTimeout = 50 * time.Millisecond
	t.Cleanup(func() { ghAuthCheckTimeout = prev })

	var sawDeadline bool
	withFakeGH(t, nil, func(ctx context.Context, _ string) error {
		<-ctx.Done()
		sawDeadline = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return ctx.Err()
	}, nil, nil)

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	_, err := ghAuth(cmd)

	require.Error(t, err)
	assert.True(t, sawDeadline, "the check must be cancelled by a deadline, not merely return an error")
	// A timeout is not evidence of a bad credential, so it must not be
	// reported as one.
	assert.NotContains(t, err.Error(), "gh auth login",
		"a timed-out check must not tell an authenticated operator to re-login: %s", err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// A missing binary and a cancelled command are true regardless of which
// credential was in play, so they must be named before the credential source
// is. ghAuth returns before its own not-installed check whenever a dev session
// supplies a token, which left this path blaming the connection.
func TestClassifyGitHubErrorNamesTheRealCauseBeforeTheCredential(t *testing.T) {
	t.Run("a missing gh is reported as missing, not as a connection problem", func(t *testing.T) {
		err := classifyGitHubError(githubcli.ErrNotInstalled{Name: "gh"}, "a-dev-session-token")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "https://cli.github.com",
			"the install hint is the point of the not-installed message")
		assert.NotContains(t, err.Error(), "dev session",
			"a missing binary is not the connection's fault: %s", err)
	})

	t.Run("a missing git is reported as git, not gh", func(t *testing.T) {
		err := classifyGitHubError(githubcli.ErrNotInstalled{Name: "git"}, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "git-scm.com")
	})

	t.Run("a cancelled command is not reported as a remote failure", func(t *testing.T) {
		err := classifyGitHubError(fmt.Errorf("gh pr view: %w", context.DeadlineExceeded), "a-dev-session-token")

		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.NotContains(t, err.Error(), "dev session")
	})

	t.Run("a genuine failure still names the credential actually used", func(t *testing.T) {
		withToken := classifyGitHubError(errors.New("exit status 1"), "a-dev-session-token")
		withoutToken := classifyGitHubError(errors.New("exit status 1"), "")

		assert.Contains(t, withToken.Error(), "dev session",
			"gh's own text suggests `gh auth login`, which does nothing once GH_TOKEN is set")
		assert.NotContains(t, withoutToken.Error(), "dev session")
	})
}

func TestRunGHPassthroughReportsAMissingBinaryInsideADevSession(t *testing.T) {
	// The feature's target case: a sandboxed agent with a shared connection
	// and no gh binary.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(issueConnectionTokenResponse{AccessToken: "gho_fromconnection"})) //nolint:errcheck,gosec
	}))
	defer srv.Close()
	withFakeGH(t, func(context.Context, string, string, ...string) (githubcli.Result, error) {
		return githubcli.Result{}, githubcli.ErrNotInstalled{Name: "gh"}
	}, nil, nil, nil)
	prevURL := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prevURL })
	t.Setenv(sandboxTokenEnvVar, "a-dev-session-bearer")

	cmd := newTestCmd(t, nil, githubPRViewCmd)
	err := runGHPassthrough(cmd, "pr", "view")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "https://cli.github.com")
	assert.NotContains(t, err.Error(), "dev session",
		"gh being absent is not the shared connection's fault: %s", err)
}

// git accepts a URL where a remote name goes, and `github push` is reachable
// as an MCP write tool alongside read tools that fetch attacker-authored PR
// and issue text, so the value is not always the operator's own.
func TestRunGitHubPushRequiresARemoteName(t *testing.T) {
	for _, tc := range []struct{ name, remote string }{
		{"https URL", "https://github.com/attacker/x.git"},
		{"scp-style", "git@github.com:attacker/x.git"},
		{"ssh URL", "ssh://git@host/x.git"},
		{"path", "../elsewhere"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pushed bool
			withFakeGH(t, nil, nil,
				func(context.Context, string, string, string, string, bool) (githubcli.Result, error) {
					pushed = true
					return githubcli.Result{}, nil
				},
				func(context.Context, string) (string, error) { return "feature/x", nil },
			)

			cmd := newTestCmd(t, map[string]string{"remote": tc.remote}, githubPushCmd)
			err := runGitHubPush(cmd, nil)

			require.Error(t, err)
			assert.False(t, pushed, "the push must not run: %q is not a configured remote's name", tc.remote)
		})
	}

	t.Run("a plain name is accepted", func(t *testing.T) {
		var gotRemote string
		withFakeGH(t, nil, nil,
			func(_ context.Context, _, _, remote, _ string, _ bool) (githubcli.Result, error) {
				gotRemote = remote
				return githubcli.Result{}, nil
			},
			func(context.Context, string) (string, error) { return "feature/x", nil },
		)

		cmd := newTestCmd(t, map[string]string{"remote": "upstream"}, githubPushCmd)
		require.NoError(t, runGitHubPush(cmd, nil))
		assert.Equal(t, "upstream", gotRemote)
	})
}

// `gh pr checks` exits 8 for "checks pending" as distinct from 1 for "a check
// failed". Flattening both to 1 destroys the signal for an agent polling CI.
func TestExitCodeForPassesAWrappedBinarysStatusThrough(t *testing.T) {
	pendingChecks := githubcli.ErrExit{
		Name: "gh",
		Code: 8,
		Err:  errors.New("gh pr checks -- 7: exit status 8"),
	}

	// Through the same wrapping a real failure takes, so the test covers the
	// classification and message layers rather than the bare error.
	assert.Equal(t, 8, exitCodeFor(classifyGitHubError(pendingChecks, "")),
		"gh's pending-checks code must survive classification and wrapping")
	assert.Equal(t, 8, exitCodeFor(classifyGitHubError(pendingChecks, "a-dev-session-token")),
		"and must not depend on which credential was in play")

	assert.Equal(t, 1, exitCodeFor(classifyGitHubError(errors.New("boom"), "")),
		"an error carrying no child status still exits 1")
}
