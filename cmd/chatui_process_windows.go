//go:build windows

package cmd

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const (
	// Detach from the launching console and start a new process group, so the
	// worker survives `project start -b` and taskkill can reach its children.
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

func detachProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}

// signalProcessGroup ends the worker and its children. Windows has no signals,
// so both the graceful and forced paths are a terminate; /T covers the tree.
func signalProcessGroup(pid int, kill bool) {
	args := []string{"/PID", strconv.Itoa(pid), "/T"}
	if kill {
		args = append(args, "/F")
	}
	_ = exec.Command("taskkill", args...).Run() //nolint:gosec // pid is our own recorded worker pid
}

// processAlive reports whether pid is live. tasklist exits 0 whether or not it
// matched, so the answer has to be read out of the output.
func processAlive(pid int) bool {
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH", "/FO", "CSV").Output() //nolint:gosec // pid is our own recorded worker pid
	if err != nil {
		return false
	}
	// Compare the pid column rather than searching the row: the session and
	// memory columns are also quoted numbers.
	want := `"` + strconv.Itoa(pid) + `"`
	for line := range strings.SplitSeq(string(out), "\n") {
		if fields := strings.Split(strings.TrimSpace(line), ","); len(fields) > 1 && fields[1] == want {
			return true
		}
	}
	return false
}

// processCommandLine reads the command line from CIM: Windows has neither
// /proc nor ps.
func processCommandLine(pid int) (string, error) {
	query := `(Get-CimInstance Win32_Process -Filter "ProcessId=` + strconv.Itoa(pid) + `").CommandLine`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", query).Output() //nolint:gosec // pid is our own recorded worker pid
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// listenerPID returns the pid listening on port, read from netstat: it ships
// with Windows, where Get-NetTCPConnection needs a module that may not.
func listenerPID(port string) (int, bool) {
	out, err := exec.Command("netstat", "-ano", "-p", "TCP").Output()
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		// Proto  Local            Foreign  State       PID
		if len(fields) < 5 || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		if !strings.HasSuffix(fields[1], ":"+port) {
			continue
		}
		if pid, convErr := strconv.Atoi(fields[4]); convErr == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}
