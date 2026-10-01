package git

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubRemote(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "scp style", raw: "git@github.com:astropods/astro-cli.git", want: "astropods/astro-cli"},
		{name: "scp style without .git", raw: "git@github.com:astropods/astro-cli", want: "astropods/astro-cli"},
		{name: "https", raw: "https://github.com/astropods/astro-cli.git", want: "astropods/astro-cli"},
		{name: "https without .git", raw: "https://github.com/astropods/astro-cli", want: "astropods/astro-cli"},
		{name: "https with trailing slash", raw: "https://github.com/astropods/astro-cli/", want: "astropods/astro-cli"},
		{name: "https with userinfo", raw: "https://someone@github.com/astropods/astro-cli.git", want: "astropods/astro-cli"},
		{name: "ssh scheme", raw: "ssh://git@github.com/astropods/astro-cli.git", want: "astropods/astro-cli"},
		{name: "ssh scheme with port", raw: "ssh://git@github.com:22/astropods/astro-cli.git", want: "astropods/astro-cli"},
		{name: "host and path only", raw: "github.com/astropods/astro-cli", want: "astropods/astro-cli"},
		{name: "bare owner/repo", raw: "astropods/astro-cli", want: "astropods/astro-cli"},
		{name: "mixed case host", raw: "https://GitHub.com/astropods/astro-cli", want: "astropods/astro-cli"},
		{name: "www host", raw: "https://www.github.com/astropods/astro-cli", want: "astropods/astro-cli"},
		{name: "surrounding whitespace", raw: "  git@github.com:astropods/astro-cli.git\n", want: "astropods/astro-cli"},

		{name: "gitlab https", raw: "https://gitlab.com/astropods/astro-cli.git", wantErr: ErrNotGitHub},
		{name: "gitlab scp", raw: "git@gitlab.com:astropods/astro-cli.git", wantErr: ErrNotGitHub},
		{name: "bitbucket", raw: "https://bitbucket.org/astropods/astro-cli", wantErr: ErrNotGitHub},
		{name: "github enterprise host", raw: "https://github.example.com/astropods/astro-cli", wantErr: ErrNotGitHub},

		{name: "empty", raw: "", wantErr: ErrUnparsableRemote},
		{name: "owner only", raw: "https://github.com/astropods", wantErr: ErrUnparsableRemote},
		{name: "too many segments", raw: "https://github.com/astropods/astro-cli/tree/main", wantErr: ErrUnparsableRemote},
		{name: "no path", raw: "https://github.com", wantErr: ErrUnparsableRemote},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseGitHubRemote(tc.raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// initRepo makes a throwaway repository so the exec-backed helpers run against
// real git rather than a stub.
func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "first"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dir
}

func TestOriginURL(t *testing.T) {
	ctx := context.Background()

	t.Run("reads the origin remote", func(t *testing.T) {
		dir := initRepo(t)
		out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", "git@github.com:astropods/astro-cli.git").CombinedOutput()
		require.NoError(t, err, string(out))

		got, err := OriginURL(ctx, dir)
		require.NoError(t, err)
		assert.Equal(t, "git@github.com:astropods/astro-cli.git", got)
	})

	t.Run("repository without an origin", func(t *testing.T) {
		_, err := OriginURL(ctx, initRepo(t))
		require.ErrorIs(t, err, ErrNoOrigin)
	})

	t.Run("directory that is not a repository", func(t *testing.T) {
		_, err := OriginURL(ctx, t.TempDir())
		require.ErrorIs(t, err, ErrNotARepo)
	})
}

func TestCurrentBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("reads the checked out branch", func(t *testing.T) {
		got, err := CurrentBranch(ctx, initRepo(t))
		require.NoError(t, err)
		assert.Equal(t, "main", got)
	})

	t.Run("detached head", func(t *testing.T) {
		dir := initRepo(t)
		sha, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		require.NoError(t, err)
		out, err := exec.Command("git", "-C", dir, "checkout", "--detach", string(trimNewline(sha))).CombinedOutput()
		require.NoError(t, err, string(out))

		_, err = CurrentBranch(ctx, dir)
		require.ErrorIs(t, err, ErrDetachedHead)
	})

	t.Run("directory that is not a repository", func(t *testing.T) {
		_, err := CurrentBranch(ctx, t.TempDir())
		require.ErrorIs(t, err, ErrNotARepo)
	})
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
