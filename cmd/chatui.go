package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/chatui"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
)

const (
	// chatUIAddr is where the CLI serves astro-client's chat UI during dev.
	chatUIAddr = "127.0.0.1:3100"
	// chatUIURL is the user-facing address (browser/printed link).
	chatUIURL = "http://localhost:3100"

	chatUIPidFile = ".chatui.pid"
	chatUILogFile = ".chatui.log"
)

// chatUIServeCmd is a hidden worker that serves the embedded chat UI and proxies
// the chat/messaging API to the local sidecar. Spawned detached by the dev
// commands; not meant to be invoked directly.
var chatUIServeCmd = &cobra.Command{
	Use:    "chatui-serve",
	Short:  "Internal: serve the local chat UI",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE:   runChatUIServe,
}

func init() {
	rootCmd.AddCommand(chatUIServeCmd)
	chatUIServeCmd.Flags().String("addr", chatUIAddr, "Address to serve the chat UI on")
	chatUIServeCmd.Flags().String("messaging-url", "", "Base URL of the local messaging sidecar HTTP API")
	chatUIServeCmd.Flags().String("agent-name", "", "Agent name for the synthesized local deployment")
	chatUIServeCmd.Flags().String("agent-display", "", "Agent display name for the synthesized local deployment")
	chatUIServeCmd.Flags().Bool("exit-with-parent", false, "Exit when the launching CLI dies (set in foreground mode; off in background mode)")
}

func runChatUIServe(cmd *cobra.Command, _ []string) error {
	srv, err := chatui.New(chatui.Config{
		Addr:         flagString(cmd, "addr"),
		MessagingURL: flagString(cmd, "messaging-url"),
		AgentName:    flagString(cmd, "agent-name"),
		AgentDisplay: flagString(cmd, "agent-display"),
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Foreground mode passes --exit-with-parent so the worker dies with the CLI
	// even on force-quit; background mode omits it so the worker outlives the CLI.
	if flagBool(cmd, "exit-with-parent") {
		ctx = cancelOnParentExit(ctx)
	}
	return srv.ListenAndServe(ctx)
}

// cancelOnParentExit cancels the returned context when the launching CLI dies.
// The worker is a session leader (Setsid) and never sees the terminal's Ctrl+C,
// so we watch for reparenting (ppid change) to follow the CLI's lifetime.
func cancelOnParentExit(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)
	startPPID := os.Getppid()
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if os.Getppid() != startPPID {
					cancel()
					return
				}
			}
		}
	}()
	return ctx
}

// startChatUI spawns the detached chat-UI worker and records its PID under
// astDir. No-op without a web interface; failures warn rather than abort.
func startChatUI(astDir, agentName string, hasWebInterface, exitWithParent bool) {
	if !hasWebInterface {
		return
	}

	// Reclaim the fixed, shared chat-UI port before spawning: stop this project's
	// recorded worker, then reclaim the port from any other chatui-serve holding
	// it (a force-quit leak or another agent's worker the pid file can't track).
	stopChatUI(astDir)
	reclaimChatUIPort(chatUIAddr)

	self, err := os.Executable()
	if err != nil {
		fmt.Printf("%s!%s %sCould not locate CLI binary to start chat UI: %v%s\n", colorYellow, colorReset, colorDim, err, colorReset)
		return
	}

	logPath := filepath.Join(astDir, chatUILogFile)
	logFile, err := os.Create(logPath) //nolint:gosec // path is under the project's .ast dir
	if err != nil {
		fmt.Printf("%s!%s %sCould not open chat UI log: %v%s\n", colorYellow, colorReset, colorDim, err, colorReset)
		return
	}
	defer func() { _ = logFile.Close() }()

	messagingURL := "http://127.0.0.1:" + composeBuilder.MessagingWebHostPort
	args := []string{
		"chatui-serve",
		"--addr", chatUIAddr,
		"--messaging-url", messagingURL,
		"--agent-name", agentName,
		"--agent-display", agentName,
	}
	if exitWithParent {
		args = append(args, "--exit-with-parent")
	}
	proc := exec.Command(self, args...) //nolint:gosec // self path + fixed args
	proc.Stdout = logFile
	proc.Stderr = logFile
	// New session so the worker survives `project start -b` (background mode)
	// and isn't tied to the launching terminal.
	proc.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := proc.Start(); err != nil {
		fmt.Printf("%s!%s %sFailed to start chat UI: %v%s\n", colorYellow, colorReset, colorDim, err, colorReset)
		return
	}

	// Capture the pid before Release(), which invalidates proc.Process.Pid (-1);
	// the readiness probe below needs the real pid to recognise its own worker.
	workerPID := proc.Process.Pid
	pidPath := filepath.Join(astDir, chatUIPidFile)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(workerPID)), 0644); err != nil { //nolint:gosec
		fmt.Printf("%s!%s %sFailed to record chat UI pid: %v%s\n", colorYellow, colorReset, colorDim, err, colorReset)
	}
	// Detach: we manage lifecycle via the pid file, not via Wait.
	_ = proc.Process.Release()

	// Confirm our worker (by pid) is serving :3100 before advertising the chat
	// URL, so we don't point users at a dead or stale worker.
	switch waitForChatUIWorker(chatUIAddr, workerPID, 3*time.Second) {
	case chatUIReady:
		// up and healthy, nothing to say
	case chatUIPortBusy:
		fmt.Printf("%s!%s %sChat UI port %s is held by another process; %s may show a stale agent%s\n",
			colorYellow, colorReset, colorDim, chatUIAddr, chatUIURL, colorReset)
	default: // chatUIUnreachable
		fmt.Printf("%s!%s %sChat UI failed to start on %s, see %s%s\n",
			colorYellow, colorReset, colorDim, chatUIURL, logPath, colorReset)
		if tail := tailFile(logPath, 5); tail != "" {
			fmt.Printf("%s%s%s\n", colorDim, tail, colorReset)
		}
	}
}

// stopChatUI terminates the chat-UI worker recorded under astDir and reports
// whether it signaled one. It confirms the pid is still a live chatui-serve
// process first, so a stale/recycled pid can't kill an unrelated process.
func stopChatUI(astDir string) bool {
	pidPath := filepath.Join(astDir, chatUIPidFile)
	data, err := os.ReadFile(pidPath) //nolint:gosec // path is under the project's .ast dir
	if err != nil {
		return false
	}
	defer func() { _ = os.Remove(pidPath) }()
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	if !isChatUIProcess(pid) {
		return false // stale/recycled pid, not our worker
	}
	// Negative pid → the whole process group (the worker is a session leader).
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	return true
}

// isChatUIProcess reports whether pid is a live chatui-serve worker: signal 0
// checks liveness, the command-line check rules out a recycled pid. Returns
// false when the command line can't be read (leaking beats killing a stranger).
func isChatUIProcess(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	cmdline, err := processCommandLine(pid)
	if err != nil {
		return false
	}
	return strings.Contains(cmdline, "chatui-serve")
}

// reclaimChatUIPort frees addr from any chatui-serve worker bound to it,
// whichever session started it (the port is shared and the per-project pid file
// can't track leaks). Only a chatui-serve listener is terminated (escalating to
// SIGKILL); a non-chatui listener is left alone.
func reclaimChatUIPort(addr string) {
	pid, ok := chatUIListenerPID(addr)
	if !ok || !isChatUIProcess(pid) {
		return
	}
	// Negative pid → the whole process group (the worker is a session leader).
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	if portFreeWithin(addr, 2*time.Second) {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	portFreeWithin(addr, 2*time.Second)
}

// chatUIListenerPID returns the pid listening on addr via lsof (present on the
// dev-supported macOS/Linux); reports no listener when lsof fails or none binds.
func chatUIListenerPID(addr string) (int, bool) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, false
	}
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN", "-t").Output() //nolint:gosec // port is our fixed chat-UI port
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false
	}
	return pid, true
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

// processCommandLine returns pid's command line for identity checks: /proc on
// Linux, ps on macOS (the only dev-supported OSes).
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
	return string(out), nil
}

// chatUIStartStatus is the outcome of the post-spawn readiness probe.
type chatUIStartStatus int

const (
	chatUIReady chatUIStartStatus = iota
	chatUIPortBusy
	chatUIUnreachable
)

// waitForChatUIWorker polls the worker's health endpoint until it reports the
// expected pid (ready), consistently answers with a different pid (port held by
// another/orphaned worker), or never responds within timeout (failed to bind).
func waitForChatUIWorker(addr string, wantPID int, timeout time.Duration) chatUIStartStatus {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	url := "http://" + addr + chatui.HealthPath
	deadline := time.Now().Add(timeout)
	sawOther := false
	for {
		if pid, ok := chatUIHealthPID(client, url); ok {
			if pid == wantPID {
				return chatUIReady
			}
			sawOther = true
		}
		if time.Now().After(deadline) {
			if sawOther {
				return chatUIPortBusy
			}
			return chatUIUnreachable
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// chatUIHealthPID fetches the worker health endpoint and returns the responder's
// pid. ok is false when the endpoint is unreachable or the body is unparseable.
func chatUIHealthPID(client *http.Client, url string) (pid int, ok bool) {
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

// tailFile returns the last n non-empty lines of the file at path (best-effort).
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
