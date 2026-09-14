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

// newDockerClient returns the shared Docker client, initializing it on first call.
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

// Close releases resources held by the singleton Docker client, if initialized.
// Call it via defer in main.
func CloseDockerClient() error {
	if dockerClient != nil {
		return dockerClient.Close()
	}
	return nil
}
