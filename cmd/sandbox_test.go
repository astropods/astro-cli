package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/sandbox"
)

func TestTheSandboxWorkerIsSeparateFromTheChatUIWorker(t *testing.T) {
	assert.Equal(t, "sandbox-serve", sandboxWorker.serveCommand,
		"the serve command identifies the worker in a process listing, so it must match the subcommand")
	assert.Equal(t, sandbox.HealthPath, sandboxWorker.healthPath)

	assert.NotEqual(t, chatUIWorker.addr, sandboxWorker.addr,
		"two workers on one port would each reclaim the other's")
	assert.NotEqual(t, chatUIWorker.pidFile, sandboxWorker.pidFile)
	assert.NotEqual(t, chatUIWorker.logFile, sandboxWorker.logFile)
	assert.NotEqual(t, chatUIWorker.serveCommand, sandboxWorker.serveCommand,
		"ownership is matched on this string, so a shared value would let one worker kill the other")
}

func TestTheSandboxServeCommandIsHiddenAndRegistered(t *testing.T) {
	var found *bool
	for _, c := range rootCmd.Commands() {
		if c.Name() == "sandbox-serve" {
			hidden := c.Hidden
			found = &hidden
		}
	}
	require.NotNil(t, found, "the dev commands spawn this subcommand, so it has to be registered")
	assert.True(t, *found, "it is an internal worker, not a command to be typed")
}

func TestTheSigningSecretIsWrittenOwnerOnly(t *testing.T) {
	dir := t.TempDir()

	path, err := writeSandboxSecret(dir, "a-secret")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, sandboxSecretFile), path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"the secret mints tokens for the broker, so it must not be world readable")
}

func TestTheSigningSecretRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path, err := writeSandboxSecret(dir, "a-secret\n")
	require.NoError(t, err)

	secret, err := readSandboxSecret(path)
	require.NoError(t, err)
	assert.Equal(t, "a-secret", secret, "a trailing newline must not change the secret")
}

func TestReadingTheSecretRefusesWhatCannotVerifyAToken(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(empty, []byte("   \n"), 0o600))

	for _, tc := range []struct{ name, path string }{
		{"no path", ""},
		{"a missing file", filepath.Join(dir, "absent")},
		{"an empty file", empty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readSandboxSecret(tc.path)
			assert.Error(t, err,
				"a worker that cannot verify a token would accept any caller, so it must refuse to start")
		})
	}
}

func TestStoppingTheBrokerWithNoWorkerReportsNothingSignaled(t *testing.T) {
	assert.False(t, stopSandboxBroker(t.TempDir()))
}
