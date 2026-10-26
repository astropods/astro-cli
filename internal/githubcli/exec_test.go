package githubcli

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withFakeRun(t *testing.T, fake runFunc) {
	t.Helper()
	original := run
	run = fake
	t.Cleanup(func() { run = original })
}

func TestGHPassesTokenAsEnvOnlyWhenNonEmpty(t *testing.T) {
	t.Run("empty token leaves GH_TOKEN unset, so gh falls back to its own login", func(t *testing.T) {
		var capturedEnv []string
		withFakeRun(t, func(_ context.Context, dir string, env []string, name string, args ...string) (Result, error) {
			capturedEnv = env
			assert.Equal(t, "gh", name)
			assert.Equal(t, []string{"pr", "view", "5"}, args)
			return Result{Stdout: "ok"}, nil
		})

		result, err := GH(context.Background(), "/repo", "", "pr", "view", "5")
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Stdout)
		assert.NotContains(t, capturedEnv, "GH_TOKEN=secret")
		for _, e := range capturedEnv {
			assert.NotContains(t, e, "GH_TOKEN=")
		}
	})

	t.Run("non-empty token is passed as GH_TOKEN", func(t *testing.T) {
		var capturedEnv []string
		withFakeRun(t, func(_ context.Context, _ string, env []string, _ string, _ ...string) (Result, error) {
			capturedEnv = env
			return Result{}, nil
		})

		_, err := GH(context.Background(), "/repo", "secret", "pr", "view")
		require.NoError(t, err)
		assert.Contains(t, capturedEnv, "GH_TOKEN=secret")
	})
}

func TestPushOnlySetsAuthHeaderWhenTokenGiven(t *testing.T) {
	t.Run("empty token runs a plain git push with no extraheader override", func(t *testing.T) {
		var capturedArgs, capturedEnv []string
		withFakeRun(t, func(_ context.Context, dir string, env []string, name string, args ...string) (Result, error) {
			capturedArgs, capturedEnv = args, env
			assert.Equal(t, "git", name)
			assert.Equal(t, "/repo", dir)
			return Result{}, nil
		})

		_, err := Push(context.Background(), "/repo", "", "origin", "my-branch", false)
		require.NoError(t, err)
		assert.Equal(t, []string{"push", "--", "origin", "my-branch"}, capturedArgs)
		assert.Equal(t, []string{"GIT_TERMINAL_PROMPT=0"}, capturedEnv,
			"an empty token must leave git's own credential setup alone apart from prompt suppression")
	})

	t.Run("non-empty token adds a single-request auth header via env, not argv, and --set-upstream when asked", func(t *testing.T) {
		var capturedArgs, capturedEnv []string
		withFakeRun(t, func(_ context.Context, _ string, env []string, _ string, args ...string) (Result, error) {
			capturedArgs, capturedEnv = args, env
			return Result{}, nil
		})

		_, err := Push(context.Background(), "/repo", "gho_abc123", "origin", "my-branch", true)
		require.NoError(t, err)
		assert.Equal(t, []string{"push", "--set-upstream", "--", "origin", "my-branch"}, capturedArgs,
			"the token must never appear in args: runCommand joins args into its error message on failure")
		assert.Contains(t, capturedEnv, "GIT_CONFIG_COUNT=1")
		assert.Contains(t, capturedEnv, "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader")
		assert.Contains(t, capturedEnv, "GIT_CONFIG_VALUE_0=AUTHORIZATION: basic eC1hY2Nlc3MtdG9rZW46Z2hvX2FiYzEyMw==")
	})
}

func TestPushNeverLeaksTheTokenIntoAFailedPushsErrorText(t *testing.T) {
	// Exercises the real exec path, no fake runner.
	dir := t.TempDir() // not a git repository, so `git push` fails immediately
	const token = "SUPERSECRETTOKENVALUE123"

	_, err := Push(context.Background(), dir, token, "origin", "some-branch", false)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), token)
	assert.NotContains(t, err.Error(), basicAuthHeader(token))
}

func TestWithPositional(t *testing.T) {
	assert.Equal(t, []string{"pr", "view"}, WithPositional([]string{"pr", "view"}))
	assert.Equal(t, []string{"pr", "view", "--", "42"}, WithPositional([]string{"pr", "view"}, "42"))
	assert.Equal(t, []string{"push", "--", "origin", "main"}, WithPositional([]string{"push"}, "origin", "main"))
}

func TestPushNeverLetsARemoteValueActAsAGitFlag(t *testing.T) {
	// git's error text here depends on push.default, and commit.gpgsign or
	// core.hooksPath would fail the setup commands outright.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...) //nolint:gosec
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init", "-q", "-b", "main") // explicit branch name: the runner's git default may not be "main"
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	require.NoError(t, os.WriteFile(dir+"/f.txt", []byte("x"), 0o600))
	run("add", "f.txt")
	run("commit", "-q", "-m", "init")

	_, err := Push(context.Background(), dir, "", "--force", "main", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "strange pathname '--force' blocked",
		"-- did not protect %q as a positional, so git parsed it as a flag: %s", "--force", err.Error())
	assert.NotContains(t, err.Error(), "has no upstream branch",
		"--force reached git as a real flag instead of a positional: %s", err.Error())
}

func TestAppendGitConfigEnvExtendsRatherThanReplacesExistingConfig(t *testing.T) {
	t.Run("no pre-existing config starts at index 0", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_COUNT", "")
		env := appendGitConfigEnv("http.extraheader", "AUTHORIZATION: basic X")
		assert.Equal(t, []string{
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraheader",
			"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic X",
		}, env)
	})

	t.Run("pre-existing config is extended, not replaced", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_COUNT", "2")
		env := appendGitConfigEnv("http.extraheader", "AUTHORIZATION: basic X")
		assert.Equal(t, []string{
			"GIT_CONFIG_COUNT=3",
			"GIT_CONFIG_KEY_2=http.extraheader",
			"GIT_CONFIG_VALUE_2=AUTHORIZATION: basic X",
		}, env, "must raise the count and use the next free index, never touching KEY_0/KEY_1")
	})
}

func TestCurrentBranchTrimsOutput(t *testing.T) {
	withFakeRun(t, func(_ context.Context, _ string, _ []string, name string, args ...string) (Result, error) {
		assert.Equal(t, "git", name)
		assert.Equal(t, []string{"rev-parse", "--abbrev-ref", "HEAD"}, args)
		return Result{Stdout: "feature/x\n"}, nil
	})

	branch, err := CurrentBranch(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, "feature/x", branch)
}

func TestRunCommandWrapsMissingBinaryDistinctly(t *testing.T) {
	_, err := runCommand(context.Background(), t.TempDir(), nil, "definitely-not-a-real-binary-xyz")
	require.Error(t, err)
	assert.ErrorAs(t, err, &ErrNotInstalled{})
}

func TestRunCommandReportsStderrOnFailure(t *testing.T) {
	_, err := runCommand(context.Background(), t.TempDir(), nil, "sh", "-c", "echo boom 1>&2; exit 3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestRunCommandDoesNotHangOnAnOrphanedGrandchild(t *testing.T) {
	// "sleep 30 &" backgrounds a grandchild that inherits the pipes; sh
	// itself exits immediately via `true`.
	start := time.Now()
	_, err := runCommand(context.Background(), t.TempDir(), nil, "sh", "-c", "sleep 30 & true")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, waitDelay+2*time.Second,
		"took %s; must give up at roughly waitDelay, not wait for the orphan's full lifetime", elapsed)
}

func TestRunCommandReportsACancelledContextAsCancellation(t *testing.T) {
	t.Run("deadline exceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := run(ctx, t.TempDir(), nil, "sleep", "10")

		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"a timed-out command must be distinguishable from one the remote rejected: %s", err)
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() { time.Sleep(20 * time.Millisecond); cancel() }()

		_, err := run(ctx, t.TempDir(), nil, "sleep", "10")

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestRunCommandOmitsATrailingSeparatorWhenStderrIsEmpty(t *testing.T) {
	_, err := run(context.Background(), t.TempDir(), nil, "sh", "-c", "exit 8")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), ": ,", "stray separator")
	assert.False(t, strings.HasSuffix(err.Error(), ": "),
		"error should not end in a dangling separator: %q", err.Error())
}
