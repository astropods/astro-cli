package cmd

import (
	"net"
	"os"
	"strings"
	"testing"
)

// The seam has a per-platform implementation, so these run against whichever
// one the build selected rather than asserting a particular OS's behaviour.

func TestProcessAlive_TrueForSelf(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("processAlive(self) = false, want true")
	}
}

func TestProcessAlive_FalseForUnusedPID(t *testing.T) {
	// Above the default pid ceiling on every supported platform, so nothing
	// can legitimately hold it.
	if processAlive(1 << 30) {
		t.Fatal("processAlive(unused pid) = true, want false")
	}
}

func TestProcessCommandLine_NamesTheTestBinary(t *testing.T) {
	out, err := processCommandLine(os.Getpid())
	if err != nil {
		t.Fatalf("processCommandLine(self): %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("processCommandLine(self) is empty, so identity checks cannot distinguish a recycled pid")
	}
}

func TestListenerPID_FindsOurOwnListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	pid, ok := listenerPID(port)
	if !ok {
		t.Skipf("no listener reported for port %s; the platform probe (lsof/netstat) is unavailable here", port)
	}
	if pid != os.Getpid() {
		t.Errorf("listenerPID(%s) = %d, want this process %d", port, pid, os.Getpid())
	}
}

func TestChatUIListenerPID_RejectsAMalformedAddress(t *testing.T) {
	if _, ok := chatUIListenerPID("not-an-address"); ok {
		t.Fatal("chatUIListenerPID accepted a malformed address")
	}
}

func TestDetachProcAttr_IsSet(t *testing.T) {
	// A nil attr would tie the worker to the launching terminal, which is the
	// bug the detach exists to prevent.
	if attr := detachProcAttr(); attr == nil {
		t.Fatal("detachProcAttr() = nil, want platform detach flags")
	}
}
