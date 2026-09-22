package cmd

import (
	"net"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The seam has a per-platform implementation, so these run against whichever
// one the build selected rather than asserting a particular OS's behaviour.

func TestProcessAlive(t *testing.T) {
	tests := []struct {
		name string
		pid  int
		want bool
	}{
		{"this process", os.Getpid(), true},
		// Above the default pid ceiling on every supported platform, so
		// nothing can legitimately hold it.
		{"a pid nothing can hold", 1 << 30, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, processAlive(tc.pid))
		})
	}
}

func TestProcessCommandLine_NamesTheTestBinary(t *testing.T) {
	out, err := processCommandLine(os.Getpid())
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(out),
		"an empty command line leaves identity checks unable to spot a recycled pid")
}

func TestListenerPID_FindsOurOwnListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)

	pid, ok := listenerPID(port)
	if !ok {
		t.Skipf("no listener reported for port %s; the platform probe (lsof/netstat) is unavailable here", port)
	}
	assert.Equal(t, os.Getpid(), pid)
}

func TestDevWorkerListenerPID_RejectsAMalformedAddress(t *testing.T) {
	_, ok := devWorker{addr: "not-an-address"}.listenerPID()
	assert.False(t, ok)
}

func TestDetachProcAttr_IsSet(t *testing.T) {
	// A nil attr would tie the worker to the launching terminal, which is the
	// bug the detach exists to prevent.
	assert.NotNil(t, detachProcAttr())
}
