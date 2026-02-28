package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-queen/internal/client"
	"github.com/postman/astro/apps/astro-queen/internal/config"
	"github.com/postman/astro/apps/astro-queen/internal/tui"
)

var (
	cfgFile    string
	serverAddr string
)

var rootCmd = &cobra.Command{
	Use:   "queen",
	Short: "k9s-style TUI for the Astro admin gRPC API",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if serverAddr != "" {
			cfg.Server = serverAddr
		}

		c, err := client.New(cfg)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", cfg.Server, err)
		}
		defer c.Close() //nolint:errcheck

		return tui.Run(c.AdminService(), cfg)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default ~/.astro-queen/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "", "gRPC server address (overrides config)")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
