package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/auth"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure Astro CLI settings",
	Long: `Configure Astro CLI settings interactively.

This command prompts for configuration values and saves them to ~/.astro/config.yaml.
Configuration values can still be overridden by environment variables or command flags.

Priority order (highest to lowest):
  1. Command flags (--server, --registry)
  2. Environment variables (ASTRO_SERVER_URL, ASTRO_REGISTRY_URL)
  3. Config file (~/.astro/config.yaml)

Example:
  ast configure`,
	RunE: runConfigure,
}

func init() {
	rootCmd.AddCommand(configureCmd)
}

func runConfigure(cmd *cobra.Command, args []string) error {
	// Load existing config
	config, err := auth.LoadConfig()
	if err != nil {
		config = &auth.Config{}
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Astro CLI Configuration")
	fmt.Println("========================")
	fmt.Println("Press Enter to keep the current value shown in brackets.")
	fmt.Println()

	// Server URL
	serverPrompt := "Astro Server URL"
	if config.ServerURL != "" {
		serverPrompt = fmt.Sprintf("%s [%s]", serverPrompt, config.ServerURL)
	}
	fmt.Printf("%s: ", serverPrompt)
	serverInput, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	serverInput = strings.TrimSpace(serverInput)
	if serverInput != "" {
		config.ServerURL = serverInput
	}

	// Registry URL
	registryPrompt := "Astro Registry URL"
	if config.RegistryURL != "" {
		registryPrompt = fmt.Sprintf("%s [%s]", registryPrompt, config.RegistryURL)
	}
	fmt.Printf("%s: ", registryPrompt)
	registryInput, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	registryInput = strings.TrimSpace(registryInput)
	if registryInput != "" {
		config.RegistryURL = registryInput
	}

	// Save config
	if err := auth.SaveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	configPath, _ := auth.ConfigPath()
	fmt.Printf("\nConfiguration saved to %s\n", configPath)

	// Show current config
	fmt.Println("\nCurrent configuration:")
	if config.ServerURL != "" {
		fmt.Printf("  Server URL:   %s\n", config.ServerURL)
	} else {
		fmt.Printf("  Server URL:   (not set)\n")
	}
	if config.RegistryURL != "" {
		fmt.Printf("  Registry URL: %s\n", config.RegistryURL)
	} else {
		fmt.Printf("  Registry URL: (not set)\n")
	}

	return nil
}
