package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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

// chatUIWorker describes the detached worker that serves the chat UI.
var chatUIWorker = devWorker{
	serveCommand: "chatui-serve",
	addr:         chatUIAddr,
	healthPath:   chatui.HealthPath,
	pidFile:      chatUIPidFile,
	logFile:      chatUILogFile,
}

// startChatUI spawns the detached chat-UI worker and records its PID under
// astDir. No-op without a web interface; failures warn rather than abort.
func startChatUI(astDir, agentName string, hasWebInterface, exitWithParent bool) {
	if !hasWebInterface {
		return
	}

	// Reclaim the fixed, shared chat-UI port before spawning: stop this
	// project's recorded worker, then reclaim the port from any other
	// chatui-serve holding it (a force-quit leak or another agent's worker the
	// pid file cannot track).
	stopChatUI(astDir)
	chatUIWorker.reclaimPort()

	messagingURL := "http://127.0.0.1:" + composeBuilder.MessagingWebHostPort
	args := []string{
		"--addr", chatUIAddr,
		"--messaging-url", messagingURL,
		"--agent-name", agentName,
		"--agent-display", agentName,
	}
	if exitWithParent {
		args = append(args, "--exit-with-parent")
	}

	workerPID, logPath, err := chatUIWorker.spawn(astDir, args)
	if err != nil {
		fmt.Printf("%s!%s %sFailed to start chat UI: %v%s\n", colorYellow, colorReset, colorDim, err, colorReset)
		if workerPID == 0 {
			return
		}
	}

	// Confirm our worker (by pid) is serving :3100 before advertising the chat
	// URL, so we do not point users at a dead or stale worker.
	switch chatUIWorker.waitFor(workerPID, 3*time.Second) {
	case workerReady:
		// up and healthy, nothing to say
	case workerPortBusy:
		fmt.Printf("%s!%s %sChat UI port %s is held by another process; %s may show a stale agent%s\n",
			colorYellow, colorReset, colorDim, chatUIAddr, chatUIURL, colorReset)
	default: // workerUnreachable
		fmt.Printf("%s!%s %sChat UI failed to start on %s, see %s%s\n",
			colorYellow, colorReset, colorDim, chatUIURL, logPath, colorReset)
		if tail := tailFile(logPath, 5); tail != "" {
			fmt.Printf("%s%s%s\n", colorDim, tail, colorReset)
		}
	}
}

// stopChatUI terminates the chat-UI worker recorded under astDir and reports
// whether it signaled one.
func stopChatUI(astDir string) bool {
	return chatUIWorker.stop(astDir)
}
