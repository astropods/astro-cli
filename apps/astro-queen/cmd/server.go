package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-queen/internal/client"
	"github.com/postman/astro/apps/astro-queen/internal/config"
	"github.com/postman/astro/apps/astro-queen/internal/tui"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "TUI for the Astro admin gRPC API",
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
	rootCmd.AddCommand(serverCmd)
}
