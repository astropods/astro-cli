package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from the Astropods platform",
	Long: `Log out from the Astropods platform and clear stored credentials.

This command removes your stored authentication credentials from your system.
If you used the system keychain, credentials are removed from there as well.

Example:
  ast logout
  ast logout --all`,
	Args: cobra.NoArgs,
	RunE: runLogout,
}

var logoutAll bool

func init() {
	rootCmd.AddCommand(logoutCmd)
	logoutCmd.Flags().BoolVar(&logoutAll, "all", false, "Log out from all profiles")
}

func runLogout(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen)
	dim := color.New(color.Faint)

	storage := auth.NewStorage(binaryName)

	if logoutAll {
		if err := storage.DeleteAllProfiles(); err != nil {
			return fmt.Errorf("failed to clear credentials: %w", err)
		}
		green.Print("✓ ") //nolint:errcheck,gosec
		fmt.Println("Logged out from all profiles")
	} else {
		creds, err := storage.LoadCredentials()
		if err != nil {
			return fmt.Errorf("failed to load credentials: %w", err)
		}

		if _, ok := creds.Profiles[creds.CurrentProfile]; !ok {
			dim.Println("Not currently logged in.") //nolint:errcheck,gosec
			return nil
		}

		if err := storage.DeleteProfile(creds.CurrentProfile); err != nil {
			return fmt.Errorf("failed to clear credentials: %w", err)
		}
		green.Print("✓ ") //nolint:errcheck,gosec
		fmt.Printf("Logged out from profile %q\n", creds.CurrentProfile)
	}

	return nil
}
