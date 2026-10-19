package githubcli

import (
	"context"
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
		assert.Equal(t, []string{"push", "origin", "my-branch"}, capturedArgs)
	})

	t.Run("non-empty token adds a single-request auth header, and --set-upstream when asked", func(t *testing.T) {
		var capturedArgs []string
		withFakeRun(t, func(_ context.Context, _ string, _ []string, _ string, args ...string) (Result, error) {
			capturedArgs = args
			return Result{}, nil
		})

		_, err := Push(context.Background(), "/repo", "gho_abc123", "origin", "my-branch", true)
		require.NoError(t, err)
		require.Len(t, capturedArgs, 6)
		assert.Equal(t, "-c", capturedArgs[0])
		assert.Equal(t, "http.https://github.com/.extraheader=AUTHORIZATION: basic eC1hY2Nlc3MtdG9rZW46Z2hvX2FiYzEyMw==", capturedArgs[1])
		assert.Equal(t, []string{"push", "--set-upstream", "origin", "my-branch"}, capturedArgs[2:])
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
