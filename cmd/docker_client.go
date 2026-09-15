package cmd

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/moby/moby/client"
)

var (
	dockerClientOnce sync.Once
	dockerClient     *client.Client
	dockerClientErr  error
)

// newDockerClient returns the shared Docker client, initialising it on first call.
// Verifies daemon reachability and returns a styled, actionable error if Docker
// is not installed or not running. Callers must not close the client — it is a
// singleton; use Close() at process exit to release it.
func newDockerClient() (*client.Client, error) {
	dockerClientOnce.Do(func() {
		cli, err := client.New(client.FromEnv)
		if err != nil {
			dockerClientErr = fmt.Errorf("failed to create Docker client: %w", err)
			return
		}

		if _, err := cli.Ping(context.Background(), client.PingOptions{}); err != nil {
			_ = cli.Close()
			dockerClientErr = dockerUnreachableError(runtime.GOOS, dockerEndpointMissing())
			return
		}

		dockerClient = cli
	})
	return dockerClient, dockerClientErr
}

// dockerUnreachableError explains an unreachable daemon: Docker is either
// absent or installed and not started. goos is a parameter rather than read
// from runtime so every platform's wording is testable from whichever platform
// runs the tests.
func dockerUnreachableError(goos string, endpointMissing bool) error {
	if endpointMissing {
		msg := "Docker is not installed."
		switch goos {
		case "darwin":
			msg += "\n  → Download Docker Desktop for Mac: https://docs.docker.com/desktop/install/mac-install/"
		case "windows":
			msg += "\n  → Download Docker Desktop for Windows: https://docs.docker.com/desktop/install/windows-install/"
		default:
			msg += "\n  → Install Docker Engine: https://docs.docker.com/engine/install/"
		}
		return fmt.Errorf("%s", msg)
	}

	var start string
	switch goos {
	case "darwin":
		start = "→ Open Docker Desktop from your Applications folder or system tray"
	case "windows":
		start = "→ Start Docker Desktop from the Start menu or system tray"
	default:
		start = "→ Run: sudo systemctl start docker"
	}
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	return fmt.Errorf("%s", red.Render("🐳 Docker is not running")+"\n"+
		dim.Render("Start Docker and re-run your command.")+"\n\n"+
		hint.Render(start))
}

// Close releases resources held by the singleton Docker client, if initialised.
// Call it via defer in main.
func CloseDockerClient() error {
	if dockerClient != nil {
		return dockerClient.Close()
	}
	return nil
}
