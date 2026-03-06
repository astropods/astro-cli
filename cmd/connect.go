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
	"github.com/postman/astro/apps/astro-cli/internal/daemon"
	"github.com/postman/astro/apps/astro-cli/internal/telemetry"
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect this device to Astro",
	Long:  "Establish a persistent connection to the Astro server. The device registers itself and can receive remote commands.",
	RunE:  runConnect,
}

var connectInstallServiceCmd = &cobra.Command{
	Use:   "install-service",
	Short: "Install ast connect as an OS service (launchd/systemd)",
	RunE:  runConnectInstallService,
}

var connectUninstallServiceCmd = &cobra.Command{
	Use:   "uninstall-service",
	Short: "Remove the ast connect OS service",
	RunE:  runConnectUninstallService,
}

func init() {
	rootCmd.AddCommand(connectCmd)
	connectCmd.Hidden = true
	connectCmd.AddCommand(connectInstallServiceCmd)
	connectCmd.AddCommand(connectUninstallServiceCmd)

	connectCmd.Flags().String("server", "", "Server address (host:port)")
	connectCmd.Flags().Bool("daemon", false, "Run in the background")
	connectCmd.Flags().Bool("foreground", false, "Run in foreground (used by daemon)")
	connectCmd.Flags().Bool("status", false, "Check if the daemon is running")
	connectCmd.Flags().Bool("stop", false, "Stop the daemon")
	_ = connectCmd.Flags().MarkHidden("foreground")
}

func runConnect(cmd *cobra.Command, args []string) error {
	// Handle --status
	if status, _ := cmd.Flags().GetBool("status"); status {
		return daemon.Status(binaryName)
	}

	// Handle --stop
	if stop, _ := cmd.Flags().GetBool("stop"); stop {
		return daemon.Stop(binaryName)
	}

	// Handle --daemon: re-exec as background process
	if daemonize, _ := cmd.Flags().GetBool("daemon"); daemonize {
		var extra []string
		if server, _ := cmd.Flags().GetString("server"); server != "" {
			extra = append(extra, "--server", server)
		}
		return daemon.Start(binaryName, extra)
	}

	// Foreground mode (default, or --foreground from daemon re-exec)
	return runConnectForeground(cmd)
}

func runConnectForeground(cmd *cobra.Command) error {
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

	// Clean up PID file on exit if we were started by --daemon
	if fg, _ := cmd.Flags().GetBool("foreground"); fg {
		defer func() {
			pidFile, _, _ := daemon.Paths(binaryName)
			_ = os.Remove(pidFile)
		}()
	}

	return connect.Run(ctx, connect.Config{
		ServerAddr: serverAddr,
		Token:      token,
		DeviceID:   deviceID,
		CLIVersion: version,
	})
}

func runConnectInstallService(cmd *cobra.Command, args []string) error {
	var extra []string
	if server, _ := cmd.Parent().Flags().GetString("server"); server != "" {
		extra = append(extra, "--server", server)
	}
	return daemon.InstallService(binaryName, extra)
}

func runConnectUninstallService(cmd *cobra.Command, args []string) error {
	return daemon.UninstallService()
}

// defaultConnectServer returns the fleet server address set at build time.
func defaultConnectServer() string {
	return auth.FleetServerURL
}
