package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
)

const testSpecPath = "/project/astropods.yml"

func TestReadPushLink(t *testing.T) {
	t.Run("no file means no link", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		assert.Nil(t, readPushLink(testSpecPath))
	})

	t.Run("malformed file means no link", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		path, err := pushLinkPath(testSpecPath)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))
		assert.Nil(t, readPushLink(testSpecPath))
	})

	t.Run("round trips what writePushLink wrote", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		writePushLink(testSpecPath, "acme", "my-agent")

		link := readPushLink(testSpecPath)
		require.NotNil(t, link)
		assert.Equal(t, pushLink{Account: "acme", Name: "my-agent"}, *link)
	})

	t.Run("writePushLink overwrites a prior link", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		writePushLink(testSpecPath, "acme", "my-agent")
		writePushLink(testSpecPath, "acme", "renamed-agent")

		link := readPushLink(testSpecPath)
		require.NotNil(t, link)
		assert.Equal(t, pushLink{Account: "acme", Name: "renamed-agent"}, *link)
	})

	t.Run("relative spec paths from different working directories resolve to the same link", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		projectDir := t.TempDir()
		t.Chdir(projectDir)
		writePushLink("astropods.yml", "acme", "my-agent")

		t.Chdir(filepath.Dir(projectDir))
		link := readPushLink(filepath.Join(filepath.Base(projectDir), "astropods.yml"))
		require.NotNil(t, link)
		assert.Equal(t, pushLink{Account: "acme", Name: "my-agent"}, *link)
	})

	t.Run("two different spec files never collide", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		writePushLink("/project-a/astropods.yml", "acme", "agent-a")
		writePushLink("/project-b/astropods.yml", "acme", "agent-b")

		linkA := readPushLink("/project-a/astropods.yml")
		require.NotNil(t, linkA)
		assert.Equal(t, "agent-a", linkA.Name)

		linkB := readPushLink("/project-b/astropods.yml")
		require.NotNil(t, linkB)
		assert.Equal(t, "agent-b", linkB.Name)
	})
}

func TestPushLinkPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := pushLinkPath(testSpecPath)
	require.NoError(t, err)

	wantDir, err := auth.ConfigDir(buildinfo.BinaryName)
	require.NoError(t, err)
	assert.Equal(t, wantDir, filepath.Dir(filepath.Dir(path)))
	assert.Equal(t, "push-links", filepath.Base(filepath.Dir(path)))
	assert.True(t, filepath.IsAbs(path))

	// Same spec path always resolves to the same file.
	again, err := pushLinkPath(testSpecPath)
	require.NoError(t, err)
	assert.Equal(t, path, again)

	// A different spec path resolves elsewhere.
	other, err := pushLinkPath("/some/other/astropods.yml")
	require.NoError(t, err)
	assert.NotEqual(t, path, other)
}
