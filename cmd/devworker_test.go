package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A worker whose serve command cannot appear in any real process listing, so
// ownership checks against this test binary always decline.
func strangerWorker() devWorker {
	return devWorker{
		serveCommand: "not-a-real-serve-command",
		addr:         "127.0.0.1:0",
		pidFile:      ".stranger.pid",
		logFile:      ".stranger.log",
	}
}

func TestOwnsDeclinesAProcessThatIsNotThisWorker(t *testing.T) {
	w := strangerWorker()

	assert.False(t, w.owns(os.Getpid()),
		"a live process whose command line does not name this worker must not be claimed")
	assert.False(t, w.owns(-1), "a pid that cannot be alive must not be claimed")
}

func TestStopWithNoPidFileReportsNothingSignaled(t *testing.T) {
	w := strangerWorker()

	assert.False(t, w.stop(t.TempDir()))
}

func TestStopDiscardsAnUnreadablePidFile(t *testing.T) {
	dir := t.TempDir()
	w := strangerWorker()
	path := filepath.Join(dir, w.pidFile)
	require.NoError(t, os.WriteFile(path, []byte("not-a-pid"), 0o600))

	assert.False(t, w.stop(dir))
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "a pid file that cannot be parsed is of no further use")
}

func TestStopDeclinesAPidThatIsNotThisWorker(t *testing.T) {
	dir := t.TempDir()
	w := strangerWorker()
	// This test's own pid: alive, but not a worker of this kind. Signaling it
	// would kill the test binary, so declining is the behaviour under test.
	require.NoError(t, os.WriteFile(filepath.Join(dir, w.pidFile),
		[]byte(strconv.Itoa(os.Getpid())), 0o600))

	assert.False(t, w.stop(dir),
		"a recycled pid must not be signaled: killing a stranger is worse than leaking a worker")
}

func TestReclaimPortLeavesAListenerItDoesNotOwn(t *testing.T) {
	// The address has no listener of this worker's kind, so reclaim must be a
	// no-op rather than signaling whatever holds the port.
	strangerWorker().reclaimPort()
}

func TestWaitForReportsUnreachableWhenNothingBinds(t *testing.T) {
	w := devWorker{addr: "127.0.0.1:1", healthPath: "/__health"}

	assert.Equal(t, workerUnreachable, w.waitFor(os.Getpid(), 0),
		"a worker that never bound is unreachable, not merely busy")
}

func TestTailFileReturnsTheLastNonEmptyLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.log")
	require.NoError(t, os.WriteFile(path, []byte("one\n\ntwo\nthree\n\n"), 0o600))

	assert.Equal(t, "two\nthree", tailFile(path, 2))
	assert.Empty(t, tailFile(filepath.Join(t.TempDir(), "missing.log"), 5))
}

func TestTheChatUIWorkerIsDescribedByItsServeCommand(t *testing.T) {
	assert.Equal(t, "chatui-serve", chatUIWorker.serveCommand,
		"the serve command is what tells a stale pid from a recycled one, so it must match the subcommand")
	assert.Equal(t, chatUIAddr, chatUIWorker.addr)
	assert.NotEmpty(t, chatUIWorker.healthPath)
}
