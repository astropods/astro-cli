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

	"github.com/astropods/astro/apps/astro-cli/internal/chatui"
	composeBuilder "github.com/astropods/astro/apps/astro-cli/internal/compose"
)

const (
	// chatUIAddr is where the CLI serves astro-client's chat UI during dev.
	chatUIAddr = "127.0.0.1:3100"
	// chatUIURL is the user-facing address (browser/printed link).
	chatUIURL = "http://localhost:3100"

	chatUIPidFile = ".chatui.pid"
	chatUILogFile = ".chatui.log"
)

// chatUIServeCmd is a hidden, long-lived worker that serves the embedded chat
// UI and proxies the deployment-scoped chat/messaging API to the local
// messaging sidecar. `project start` spawns it detached so the chat UI survives
// in background mode; foreground/--local stop it on exit. It is not meant to be
// invoked directly by users.
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
	return srv.ListenAndServe(ctx)
}

// startChatUI spawns the detached chat-UI worker for the current dev session and
// records its PID under astDir. It is a no-op when the agent has no web chat
// interface. Failures are surfaced as warnings, not fatal — the rest of the dev
// environment still works without the chat UI.
func startChatUI(astDir, agentName string, hasWebInterface bool) {
	if !hasWebInterface {
		return
	}

	// Replace any worker orphaned by a previous session (e.g. a force-quit that
	// left the detached worker holding :3100). Otherwise it would keep serving
	// the old agent's chat and our new worker would fail to bind unnoticed. If we
	// did displace one, wait for it to release the port before we rebind.
	if stopChatUI(astDir) {
		waitForPortFree(chatUIAddr, 3*time.Second)
	}

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
	proc := exec.Command(self, "chatui-serve", //nolint:gosec // self path + fixed args
		"--addr", chatUIAddr,
		"--messaging-url", messagingURL,
		"--agent-name", agentName,
		"--agent-display", agentName,
	)
	proc.Stdout = logFile
	proc.Stderr = logFile
	// New session so the worker survives `project start -b` (background mode)
	// and isn't tied to the launching terminal.
	proc.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := proc.Start(); err != nil {
		fmt.Printf("%s!%s %sFailed to start chat UI: %v%s\n", colorYellow, colorReset, colorDim, err, colorReset)
		return
	}

	pidPath := filepath.Join(astDir, chatUIPidFile)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(proc.Process.Pid)), 0644); err != nil { //nolint:gosec
		fmt.Printf("%s!%s %sFailed to record chat UI pid: %v%s\n", colorYellow, colorReset, colorDim, err, colorReset)
	}
	// Detach: we manage lifecycle via the pid file, not via Wait.
	_ = proc.Process.Release()

	// Confirm the worker actually came up on :3100 — and that the responder is
	// *this* worker (pid match), not an orphan we failed to displace — rather
	// than silently advertising a dead or stale chat URL in the ready block.
	switch waitForChatUIWorker(chatUIAddr, proc.Process.Pid, 3*time.Second) {
	case chatUIReady:
		// up and healthy — nothing to say
	case chatUIPortBusy:
		fmt.Printf("%s!%s %sChat UI port %s is held by another process; %s may show a stale agent%s\n",
			colorYellow, colorReset, colorDim, chatUIAddr, chatUIURL, colorReset)
	default: // chatUIUnreachable
		fmt.Printf("%s!%s %sChat UI failed to start on %s — see %s%s\n",
			colorYellow, colorReset, colorDim, chatUIURL, logPath, colorReset)
		if tail := tailFile(logPath, 5); tail != "" {
			fmt.Printf("%s%s%s\n", colorDim, tail, colorReset)
		}
	}
}

// stopChatUI terminates the chat-UI worker recorded under astDir, if any, and
// reports whether it actually signaled one. It verifies the recorded pid still
// belongs to a live chatui-serve process before signaling, so a stale pid file
// (worker already gone, pid possibly recycled by the OS) can't take down an
// unrelated process. The worker is its own session leader (Setsid in
// startChatUI), so we signal the whole process group.
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
		return false // stale/recycled pid — not our worker; don't signal it
	}
	// Negative pid → the whole process group (the worker is a session leader).
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	return true
}

// isChatUIProcess reports whether pid is a live chatui-serve worker we started.
// Liveness (signal 0) rules out a dead pid; the command-line check rules out an
// unrelated process that reused a recycled pid. When the command line can't be
// read we conservatively return false — leaking a dev worker is cheaper than
// signaling a stranger.
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

// processCommandLine returns pid's command line for identity checks. Linux reads
// /proc; macOS shells out to ps. Those are the only dev-supported OSes
// (checkDockerRunning rejects Windows).
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

// waitForPortFree polls addr until nothing accepts a TCP connection there or the
// timeout elapses. Used after displacing an orphaned worker so the port is free
// before the replacement binds.
func waitForPortFree(addr string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
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
