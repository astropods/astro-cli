package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// devWorker manages a detached local server that `ast dev` starts: a hidden
// serve subcommand, spawned as a session leader so it can outlive the CLI in
// background mode, tracked by a pid file under the project's .ast directory.
//
// The subcommand name identifies the worker in a process listing, which is how
// a stale pid is told apart from a recycled one and how a held port is told
// apart from a port held by something else. Killing a stranger is worse than
// leaking a worker, so every check that cannot confirm ownership declines.
type devWorker struct {
	// serveCommand is the hidden subcommand the worker runs, and the string a
	// process listing is matched against.
	serveCommand string
	// addr is the fixed loopback address the worker binds.
	addr string
	// healthPath answers with the serving worker's pid, so a readiness probe
	// can tell our worker from another holding the port.
	healthPath string
	pidFile    string
	logFile    string
}

// workerStartStatus is the outcome of the post-spawn readiness probe.
type workerStartStatus int

const (
	workerReady workerStartStatus = iota
	workerPortBusy
	workerUnreachable
)

// spawn starts the worker detached and records its pid. It returns the pid and
// the log path, so the caller can probe readiness and report a failure against
// the log.
func (w devWorker) spawn(astDir string, args []string) (pid int, logPath string, err error) {
	self, err := os.Executable()
	if err != nil {
		return 0, "", fmt.Errorf("locate the CLI binary: %w", err)
	}

	logPath = filepath.Join(astDir, w.logFile)
	logFile, err := os.Create(logPath) //nolint:gosec // path is under the project's .ast dir
	if err != nil {
		return 0, logPath, fmt.Errorf("open the worker log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	proc := exec.Command(self, append([]string{w.serveCommand}, args...)...) //nolint:gosec // self path + fixed args
	proc.Stdout = logFile
	proc.Stderr = logFile
	proc.SysProcAttr = detachProcAttr()
	if err := proc.Start(); err != nil {
		return 0, logPath, fmt.Errorf("start the worker: %w", err)
	}

	// Capture the pid before Release, which invalidates proc.Process.Pid (-1);
	// the readiness probe needs the real pid to recognize its own worker.
	pid = proc.Process.Pid
	if err := os.WriteFile(filepath.Join(astDir, w.pidFile), []byte(strconv.Itoa(pid)), 0644); err != nil { //nolint:gosec
		// The worker is already running, so a pid file that cannot be written
		// costs lifecycle management rather than the worker itself.
		_ = proc.Process.Release()
		return pid, logPath, fmt.Errorf("record the worker pid: %w", err)
	}
	// Detach: lifecycle is managed through the pid file, not through Wait.
	_ = proc.Process.Release()
	return pid, logPath, nil
}

// stop terminates the worker recorded under astDir and reports whether it
// signaled one. It confirms the pid is still this worker first, so a stale or
// recycled pid cannot kill an unrelated process.
func (w devWorker) stop(astDir string) bool {
	pidPath := filepath.Join(astDir, w.pidFile)
	data, err := os.ReadFile(pidPath) //nolint:gosec // path is under the project's .ast dir
	if err != nil {
		return false
	}
	defer func() { _ = os.Remove(pidPath) }()

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	if !w.owns(pid) {
		return false
	}
	signalProcessGroup(pid, false)
	return true
}

// owns reports whether pid is a live worker of this kind: signal 0 checks
// liveness, and the command line rules out a recycled pid. An unreadable
// command line reports false, because leaking beats killing a stranger.
func (w devWorker) owns(pid int) bool {
	if !processAlive(pid) {
		return false
	}
	cmdline, err := processCommandLine(pid)
	if err != nil {
		return false
	}
	return strings.Contains(cmdline, w.serveCommand)
}

// reclaimPort frees the address from a worker of this kind bound to it,
// whichever session started it. The address is fixed and shared, so a
// per-project pid file cannot track a leak. A listener of any other kind is
// left alone.
func (w devWorker) reclaimPort() {
	pid, ok := w.listenerPID()
	if !ok || !w.owns(pid) {
		return
	}
	signalProcessGroup(pid, true)
	portFreeWithin(w.addr, 2*time.Second)
}

// listenerPID returns the pid listening on the worker's address, or no listener
// when the platform probe fails or nothing binds.
func (w devWorker) listenerPID() (int, bool) {
	_, port, err := net.SplitHostPort(w.addr)
	if err != nil {
		return 0, false
	}
	return listenerPID(port)
}

// waitFor polls the worker's health endpoint until it reports the expected pid
// (ready), consistently answers with a different pid (the port is held by
// another or orphaned worker), or never responds within timeout (failed to
// bind).
func (w devWorker) waitFor(wantPID int, timeout time.Duration) workerStartStatus {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	url := "http://" + w.addr + w.healthPath
	deadline := time.Now().Add(timeout)
	sawOther := false
	for {
		if pid, ok := workerHealthPID(client, url); ok {
			if pid == wantPID {
				return workerReady
			}
			sawOther = true
		}
		if time.Now().After(deadline) {
			if sawOther {
				return workerPortBusy
			}
			return workerUnreachable
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// workerHealthPID fetches a worker's health endpoint and returns the
// responder's pid. ok is false when the endpoint is unreachable or the body is
// unparseable.
func workerHealthPID(client *http.Client, url string) (pid int, ok bool) {
	resp, err := client.Get(url) //nolint:gosec,noctx // fixed localhost URL, short client timeout
	if err != nil {
		return 0, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	var body struct {
		PID int `json:"pid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, false
	}
	return body.PID, true
}

// portFreeWithin reports whether addr has no listener within timeout.
func portFreeWithin(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return true
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// tailFile returns the last n non-empty lines of the file at path
// (best-effort).
func tailFile(path string, n int) string {
	data, err := os.ReadFile(path) //nolint:gosec // path is under the project's .ast dir
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	kept := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, l)
		}
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	return strings.Join(kept, "\n")
}
