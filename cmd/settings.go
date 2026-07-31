package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/telemetry"
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage CLI settings and shell completions",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var settingsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update CLI settings",
	Long: `Update CLI settings.

Use --telemetry on|off to enable or disable anonymous usage data collection.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		telemetryVal := strings.ToLower(flagString(cmd, "telemetry"))
		if telemetryVal != "on" && telemetryVal != "off" {
			return fmt.Errorf("--telemetry must be on or off")
		}
		if err := telemetry.SetEnabled(buildinfo.BinaryName, telemetryVal == "on"); err != nil {
			return fmt.Errorf("failed to update telemetry setting: %w", err)
		}
		if telemetryVal == "on" {
			fmt.Println("Telemetry enabled.")
		} else {
			fmt.Println("Telemetry disabled. No usage data will be sent.")
		}
		return nil
	},
}

func writeCompletionFile(cmd *cobra.Command, shell string, gen func(w io.Writer) error) error {
	dir, err := auth.ConfigDir(buildinfo.BinaryName)
	if err != nil {
		return fmt.Errorf("failed to resolve config directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	absPath := filepath.Join(dir, buildinfo.BinaryName+"-completion."+shell)
	f, err := os.Create(absPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", absPath, err)
	}
	defer f.Close() //nolint:errcheck

	if err := gen(f); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s✓%s Written %s\n", colorGreen, colorReset, absPath) //nolint:errcheck

	switch shell {
	case "bash":
		fmt.Fprintf(w, "\nTo enable completions, add to ~/.bashrc or ~/.bash_profile:\n\n") //nolint:errcheck
		fmt.Fprintf(w, "  source %s\n\n", absPath)                                          //nolint:errcheck
		fmt.Fprintf(w, "Then reload: source ~/.bashrc\n")                                   //nolint:errcheck
	case "zsh":
		fmt.Fprintf(w, "\nTo enable completions, add to ~/.zshrc:\n\n") //nolint:errcheck
		fmt.Fprintf(w, "  source %s\n\n", absPath)                      //nolint:errcheck
		fmt.Fprintf(w, "Then reload: source ~/.zshrc\n")                //nolint:errcheck
	case "fish":
		fmt.Fprintf(w, "\nTo enable completions, copy to the fish completions directory:\n\n")        //nolint:errcheck
		fmt.Fprintf(w, "  cp %s ~/.config/fish/completions/%s.fish\n", absPath, buildinfo.BinaryName) //nolint:errcheck
	case "powershell":
		fmt.Fprintf(w, "\nTo enable completions, add to your PowerShell profile ($PROFILE):\n\n") //nolint:errcheck
		fmt.Fprintf(w, "  . %s\n", absPath)                                                       //nolint:errcheck
	}
	return nil
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(settingsCmd)

	settingsUpdateCmd.Flags().String("telemetry", "", "Enable or disable anonymous telemetry (on|off)")
	settingsCmd.AddCommand(settingsUpdateCmd)

	settingsCmd.AddCommand(
		&cobra.Command{
			Use:   "bash",
			Short: "Generate bash completion script",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return writeCompletionFile(cmd, "bash", rootCmd.GenBashCompletion)
			},
		},
		&cobra.Command{
			Use:   "zsh",
			Short: "Generate zsh completion script",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return writeCompletionFile(cmd, "zsh", rootCmd.GenZshCompletion)
			},
		},
		&cobra.Command{
			Use:   "fish",
			Short: "Generate fish completion script",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return writeCompletionFile(cmd, "fish", func(w io.Writer) error {
					return rootCmd.GenFishCompletion(w, true)
				})
			},
		},
		&cobra.Command{
			Use:   "powershell",
			Short: "Generate PowerShell completion script",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return writeCompletionFile(cmd, "powershell", rootCmd.GenPowerShellCompletionWithDesc)
			},
		},
	)
}
