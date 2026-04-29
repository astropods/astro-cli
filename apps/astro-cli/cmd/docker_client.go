package cmd

import (
	"context"
	"fmt"
	"os"
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

			red := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
			dim := lipgloss.NewStyle().Faint(true)
			hint := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

			if _, statErr := os.Lstat("/var/run/docker.sock"); os.IsNotExist(statErr) {
				msg := "Docker is not installed."
				if runtime.GOOS == "darwin" {
					msg += "\n  → Download Docker Desktop for Mac: https://docs.docker.com/desktop/install/mac-install/"
				} else {
					msg += "\n  → Install Docker Engine: https://docs.docker.com/engine/install/"
				}
				dockerClientErr = fmt.Errorf("%s", msg)
				return
			}

			var hint2 string
			if runtime.GOOS == "darwin" {
				hint2 = hint.Render("→ Open Docker Desktop from your Applications folder or system tray")
			} else {
				hint2 = hint.Render("→ Run: sudo systemctl start docker")
			}
			msg := red.Render("🐳 Docker is not running") + "\n" +
				dim.Render("Start Docker and re-run your command.") + "\n\n" +
				hint2
			dockerClientErr = fmt.Errorf("%s", msg)
			return
		}

		dockerClient = cli
	})
	return dockerClient, dockerClientErr
}

// Close releases resources held by the singleton Docker client, if initialised.
// Call it via defer in main.
func CloseDockerClient() error {
	if dockerClient != nil {
		return dockerClient.Close()
	}
	return nil
}
