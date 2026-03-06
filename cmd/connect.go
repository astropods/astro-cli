package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/auth"
	"github.com/postman/astro/apps/astro-cli/internal/connect"
	"github.com/postman/astro/apps/astro-cli/internal/telemetry"
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect this device to Astro",
	Long:  "Establish a persistent connection to the Astro server. The device registers itself and can receive remote commands.",
	RunE:  runConnect,
}

func init() {
	rootCmd.AddCommand(connectCmd)
	connectCmd.Flags().String("server", "", "Server address (host:port)")
}

func runConnect(cmd *cobra.Command, args []string) error {
	tokenManager := auth.NewTokenManager(binaryName)
	if err := tokenManager.RequireAuth(); err != nil {
		return fmt.Errorf("authentication required: run '%s login' first", binaryName)
	}

	token, err := tokenManager.GetValidAccessToken(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	serverAddr, _ := cmd.Flags().GetString("server")
	if serverAddr == "" {
		serverAddr = defaultConnectServer()
	}

	deviceID := telemetry.GetDeviceID(binaryName)

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Handle signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nDisconnecting...")
		cancel()
	}()

	return connect.Run(ctx, connect.Config{
		ServerAddr: serverAddr,
		Token:      token,
		DeviceID:   deviceID,
		CLIVersion: version,
	})
}

// defaultConnectServer derives the connect server address from the configured API server.
// API server is typically at host:8080, connect server at host:9092.
func defaultConnectServer() string {
	// Use the auth default server URL to derive the host
	// DefaultServerURL is like "http://localhost:8080"
	// We want "localhost:9092"
	return "localhost:9092"
}
