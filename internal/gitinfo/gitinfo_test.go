package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowsableURL(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{name: "https with suffix", remote: "https://github.com/astropods/astro.git", want: "https://github.com/astropods/astro"},
		{name: "https without suffix", remote: "https://github.com/astropods/astro", want: "https://github.com/astropods/astro"},
		{name: "trailing slash", remote: "https://github.com/astropods/astro/", want: "https://github.com/astropods/astro"},
		{name: "scp form", remote: "git@github.com:astropods/astro.git", want: "https://github.com/astropods/astro"},
		{name: "ssh scheme", remote: "ssh://git@github.com/astropods/astro.git", want: "https://github.com/astropods/astro"},
		{name: "git scheme", remote: "git://github.com/astropods/astro.git", want: "https://github.com/astropods/astro"},
		{name: "gitlab host", remote: "git@gitlab.com:group/agent.git", want: "https://gitlab.com/group/agent"},
		{name: "local path", remote: "/Users/me/repos/astro", want: ""},
		{name: "relative path", remote: "../astro", want: ""},
		{name: "empty", remote: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BrowsableURL(tt.remote))
		})
	}
}

// initRepo creates a git repository with an origin remote and returns its path.
func initRepo(t *testing.T, remote string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", dir, "init", "--quiet").Run())
	if remote != "" {
		require.NoError(t, exec.Command("git", "-C", dir, "remote", "add", "origin", remote).Run())
	}
	return dir
}

func TestFor(t *testing.T) {
	t.Run("project at the repository root", func(t *testing.T) {
		repo := initRepo(t, "git@github.com:astropods/agents.git")

		got := For(filepath.Join(repo, "scout"))

		assert.Equal(t, "https://github.com/astropods/agents", got.URL)
		assert.Equal(t, "scout", got.Directory)
	})

	t.Run("project nested below the repository root", func(t *testing.T) {
		repo := initRepo(t, "https://github.com/astropods/agents.git")
		require.NoError(t, os.MkdirAll(filepath.Join(repo, "internal", "team"), 0o755))

		got := For(filepath.Join(repo, "internal", "team", "scout"))

		assert.Equal(t, "https://github.com/astropods/agents", got.URL)
		assert.Equal(t, "internal/team/scout", got.Directory)
	})

	t.Run("repository without an origin", func(t *testing.T) {
		repo := initRepo(t, "")

		assert.Equal(t, Repository{}, For(filepath.Join(repo, "scout")))
	})

	t.Run("directory outside any repository", func(t *testing.T) {
		assert.Equal(t, Repository{}, For(filepath.Join(t.TempDir(), "scout")))
	})
}

func TestUserName(t *testing.T) {
	repo := initRepo(t, "")
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.name", "Jane Doe").Run())

	assert.Equal(t, "Jane Doe", UserName(repo))
}
