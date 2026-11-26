package git

import (
	"os"
	"path/filepath"
	"strings"
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
		{name: "owner casing is kept", raw: "git@github.com:Astropods/Astro-CLI.git", want: "Astropods/Astro-CLI"},
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
		{name: "userinfo colon without a host colon", raw: "user:pass@github.com/astropods/astro-cli", wantErr: ErrUnparsableRemote},
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

func TestOriginURL(t *testing.T) {
	t.Run("reads the origin remote", func(t *testing.T) {
		dir, _ := committedRepository(t)
		gitCommand(t, dir, "remote", "add", "origin", "git@github.com:astropods/astro-cli.git")

		got, err := OriginURL(dir)
		require.NoError(t, err)
		assert.Equal(t, "git@github.com:astropods/astro-cli.git", got)
	})

	t.Run("repository without an origin", func(t *testing.T) {
		dir, _ := committedRepository(t)
		_, err := OriginURL(dir)
		require.ErrorIs(t, err, ErrNoOrigin)
	})

	t.Run("directory that is not a repository", func(t *testing.T) {
		_, err := OriginURL(t.TempDir())
		require.ErrorIs(t, err, ErrNotARepo)
	})
}

func TestCurrentBranch(t *testing.T) {
	t.Run("reads the checked out branch", func(t *testing.T) {
		dir, _ := committedRepository(t)
		gitCommand(t, dir, "checkout", "-q", "-b", "feature/connect")

		got, err := CurrentBranch(dir)
		require.NoError(t, err)
		assert.Equal(t, "feature/connect", got)
	})

	t.Run("detached head", func(t *testing.T) {
		dir, sha := committedRepository(t)
		gitCommand(t, dir, "checkout", "-q", "--detach", strings.TrimSpace(sha))

		_, err := CurrentBranch(dir)
		require.ErrorIs(t, err, ErrDetachedHead)
	})

	t.Run("directory that is not a repository", func(t *testing.T) {
		_, err := CurrentBranch(t.TempDir())
		require.ErrorIs(t, err, ErrNotARepo)
	})
}

func TestRepoPrefix(t *testing.T) {
	dir, _ := committedRepository(t)
	nested := filepath.Join(dir, "services", "summarizer")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	t.Run("repository root", func(t *testing.T) {
		got, err := RepoPrefix(dir)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("subdirectory", func(t *testing.T) {
		got, err := RepoPrefix(nested)
		require.NoError(t, err)
		assert.Equal(t, "services/summarizer", got)
	})

	t.Run("directory that is not a repository", func(t *testing.T) {
		_, err := RepoPrefix(t.TempDir())
		require.ErrorIs(t, err, ErrNotARepo)
	})
}
