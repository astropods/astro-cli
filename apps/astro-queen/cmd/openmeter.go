package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-queen/internal/config"
	"github.com/postman/astro/apps/astro-queen/internal/openmeter"
	"github.com/postman/astro/apps/astro-queen/internal/openmeter_tui"
)

var openmeterCmd = &cobra.Command{
	Use:   "openmeter",
	Short: "TUI for managing OpenMeter (meters, events, customers, subjects)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		client := openmeter.New(cfg.OpenMeterServer, cfg.OpenMeterAPIKey)
		return openmeter_tui.Run(client, cfg)
	},
}

func init() {
	rootCmd.AddCommand(openmeterCmd)
}
