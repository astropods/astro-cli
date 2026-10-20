package githubcli

import (
	"context"
	"os"
	"os/exec"
	"testing"

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
		var capturedArgs []string
		withFakeRun(t, func(_ context.Context, dir string, _ []string, name string, args ...string) (Result, error) {
			capturedArgs = args
			assert.Equal(t, "git", name)
			assert.Equal(t, "/repo", dir)
			return Result{}, nil
		})

		_, err := Push(context.Background(), "/repo", "", "origin", "my-branch", false)
		require.NoError(t, err)
		assert.Equal(t, []string{"push", "--", "origin", "my-branch"}, capturedArgs)
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
	// Exercises the real exec path, no fake runner: runCommand's error
	// formatting joins args into the message, so the token must never be
	// placed there.
	dir := t.TempDir() // not a git repository, so `git push` fails immediately
	const token = "SUPERSECRETTOKENVALUE123"

	_, err := Push(context.Background(), dir, token, "origin", "some-branch", false)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), token)
	assert.NotContains(t, err.Error(), basicAuthHeader(token))
}

func TestPushNeverLetsARemoteValueActAsAGitFlag(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...) //nolint:gosec
		cmd.Dir = dir
		require.NoError(t, cmd.Run())
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	require.NoError(t, os.WriteFile(dir+"/f.txt", []byte("x"), 0o600))
	run("add", "f.txt")
	run("commit", "-q", "-m", "init")

	_, err := Push(context.Background(), dir, "", "--force", "main", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked",
		"expected git to refuse the positional --force rather than run it as a real flag; got: %s", err.Error())
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
