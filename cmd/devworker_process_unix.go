//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// detachProcAttr puts the worker in its own session so it survives
// `project start -b` and is not tied to the launching terminal.
func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// signalProcessGroup signals the worker's whole group. The worker is a session
// leader, so a negative pid reaches its children too.
func signalProcessGroup(pid int, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pid, sig)
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// processCommandLine returns pid's command line: /proc on Linux, ps elsewhere.
func processCommandLine(pid int) (string, error) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)) //nolint:gosec // pid is our own recorded worker pid
		if err != nil {
			return "", err
		}
		return strings.ReplaceAll(string(data), "\x00", " "), nil
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output() //nolint:gosec // pid is our own recorded worker pid
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func listenerPID(port string) (int, bool) {
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN", "-t").Output() //nolint:gosec // port is our fixed chat-UI port
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if pid, convErr := strconv.Atoi(strings.TrimSpace(line)); convErr == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}
