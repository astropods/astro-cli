package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

func TestReadPushLink(t *testing.T) {
	t.Run("no file means no link", func(t *testing.T) {
		t.Chdir(t.TempDir())
		assert.Nil(t, readPushLink())
	})

	t.Run("malformed file means no link", func(t *testing.T) {
		t.Chdir(t.TempDir())
		path, err := pushLinkPath()
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))
		assert.Nil(t, readPushLink())
	})

	t.Run("round trips what writePushLink wrote", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writePushLink("acme", "my-agent")

		link := readPushLink()
		require.NotNil(t, link)
		assert.Equal(t, pushLink{Account: "acme", Name: "my-agent"}, *link)
	})

	t.Run("writePushLink overwrites a prior link", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writePushLink("acme", "my-agent")
		writePushLink("acme", "renamed-agent")

		link := readPushLink()
		require.NotNil(t, link)
		assert.Equal(t, pushLink{Account: "acme", Name: "renamed-agent"}, *link)
	})
}

func TestPushLinkPath(t *testing.T) {
	t.Chdir(t.TempDir())
	path, err := pushLinkPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(buildinfo.AppDirName, "push.json"), mustRel(t, path))
}

// mustRel resolves path relative to the current working directory, so the
// assertion doesn't depend on the temp dir's absolute location.
func mustRel(t *testing.T, path string) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	rel, err := filepath.Rel(wd, path)
	require.NoError(t, err)
	return rel
}
