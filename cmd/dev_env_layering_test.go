package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/config"
	spec "github.com/astropods/astro-spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// devEnvWithBothSources writes key to the env file and the project store with
// distinct values, and returns the working directory.
func devEnvWithBothSources(t *testing.T, key, fileValue, storeValue string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(workingDir, ".env"), []byte(key+"="+fileValue+"\n"), 0o600,
	))
	require.NoError(t, config.MergeProjectVars(
		buildinfo.BinaryName, workingDir, "ag", map[string]string{key: storeValue},
	))
	return workingDir
}

func TestAssembleDevEnv_ProcessEnvMatchesTheMap(t *testing.T) {
	const key = "ASTRO_TEST_LAYERED"
	require.NoError(t, os.Unsetenv(key))
	workingDir := devEnvWithBothSources(t, key, "from-file", "from-store")

	var out strings.Builder
	envVars, counts, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
		WorkingDir: workingDir, EnvFile: ".env", Export: true,
	})
	require.NoError(t, err)

	assert.Equal(t, envVars[key], os.Getenv(key),
		"the process env and the container env must resolve a key the same way")
	assert.Zero(t, counts.AlreadySet,
		"nothing was set before the run, so the run must not report a key as already set")
}

func TestAssembleDevEnv_CountsAKeyLeftAloneOnce(t *testing.T) {
	const key = "ASTRO_TEST_LAYERED_TWICE"
	workingDir := devEnvWithBothSources(t, key, "from-file", "from-store")
	t.Setenv(key, "from-shell")

	var out strings.Builder
	_, counts, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
		WorkingDir: workingDir, EnvFile: ".env", Export: true,
	})
	require.NoError(t, err)

	assert.Equal(t, "from-shell", os.Getenv(key), "a value set for this run outranks both sources")
	assert.Equal(t, 1, counts.AlreadySet,
		"one key was left alone, so it counts once however many sources supplied it")
}
