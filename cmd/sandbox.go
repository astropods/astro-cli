package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/sandbox"
)

const (
	// sandboxBrokerAddr is where the CLI serves the sandbox control plane
	// during dev. The agent reaches it through host.docker.internal, so the
	// port is fixed rather than ephemeral.
	sandboxBrokerAddr = "127.0.0.1:3120"

	sandboxPidFile = ".sandbox.pid"
	sandboxLogFile = ".sandbox.log"
	// sandboxSecretFile holds the signing secret the worker verifies tokens
	// with. It is a file rather than a flag, because a flag is visible in a
	// process listing.
	sandboxSecretFile = ".sandbox.secret"
)

// sandboxWorker describes the detached worker that serves the sandbox control
// plane. It is separate from the chat UI's worker: a sandbox is available
// whether or not the agent serves a web interface.
var sandboxWorker = devWorker{
	serveCommand: "sandbox-serve",
	addr:         sandboxBrokerAddr,
	healthPath:   sandbox.HealthPath,
	pidFile:      sandboxPidFile,
	logFile:      sandboxLogFile,
}

// sandboxServeCmd is a hidden worker that serves the sandbox control plane and
// runs one container per attach name. Spawned detached by the dev commands; not
// meant to be invoked directly.
var sandboxServeCmd = &cobra.Command{
	Use:    "sandbox-serve",
	Short:  "Internal: serve the local sandbox control plane",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE:   runSandboxServe,
}

func init() {
	rootCmd.AddCommand(sandboxServeCmd)
	sandboxServeCmd.Flags().String("addr", sandboxBrokerAddr, "Address to serve the sandbox control plane on")
	sandboxServeCmd.Flags().String("image", "", "Sandbox image to run")
	sandboxServeCmd.Flags().String("network", "", "Compose network the sandbox joins so the agent can reach it")
	sandboxServeCmd.Flags().String("project", "", "Compose project name, which scopes the sandbox labels")
	sandboxServeCmd.Flags().String("secret-file", "", "File holding the token signing secret")
	sandboxServeCmd.Flags().Bool("exit-with-parent", false,
		"Exit when the launching CLI dies (set in foreground mode; off in background mode)")
}

func runSandboxServe(cmd *cobra.Command, _ []string) error {
	secret, err := readSandboxSecret(flagString(cmd, "secret-file"))
	if err != nil {
		return err
	}

	docker, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("connect to Docker: %w", err)
	}

	runner := sandbox.NewRunner(docker, sandbox.Config{
		Image:   flagString(cmd, "image"),
		Network: flagString(cmd, "network"),
		Project: flagString(cmd, "project"),
	})

	log := slog.New(slog.NewTextHandler(cmd.OutOrStdout(), &slog.HandlerOptions{Level: slog.LevelInfo}))
	srv := sandbox.NewServer(runner, secret, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Foreground mode passes --exit-with-parent so the worker dies with the
	// CLI even on force-quit; background mode omits it so the worker outlives
	// the CLI and a sandbox stays attachable.
	if flagBool(cmd, "exit-with-parent") {
		ctx = cancelOnParentExit(ctx)
	}

	// Sandboxes outlive the serving process only as containers, and a container
	// with no broker is unreachable and still billed to the developer's
	// machine, so the worker clears them on the way out.
	defer func() {
		removeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runner.RemoveAll(removeCtx); err != nil {
			log.Warn("sandbox broker: remove sandboxes on shutdown failed", "error", err)
		}
	}()

	log.Info("sandbox broker: listening", "addr", flagString(cmd, "addr"))
	return srv.ListenAndServe(ctx, flagString(cmd, "addr"))
}

// writeSandboxSecret records the signing secret for the worker to read, with
// owner-only permissions. The CLI mints the agent's token from the same secret.
func writeSandboxSecret(astDir, secret string) (string, error) {
	path := filepath.Join(astDir, sandboxSecretFile)
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return "", fmt.Errorf("record the sandbox signing secret: %w", err)
	}
	return path, nil
}

func readSandboxSecret(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no --secret-file given, so no token could be verified")
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is under the project's .ast dir
	if err != nil {
		return "", fmt.Errorf("read the sandbox signing secret: %w", err)
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("the sandbox signing secret is empty")
	}
	return secret, nil
}

// stopSandboxBroker terminates the sandbox worker recorded under astDir and
// reports whether it signaled one.
func stopSandboxBroker(astDir string) bool {
	return sandboxWorker.stop(astDir)
}
