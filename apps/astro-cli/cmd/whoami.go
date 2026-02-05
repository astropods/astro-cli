package cmd

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/auth"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Display the currently authenticated user",
	Long: `Display information about the currently authenticated user.

This command shows your user details and validates that your credentials
are still valid, refreshing them if necessary.

Example:
  ast whoami`,
	RunE: runWhoami,
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}

func runWhoami(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen)
	cyan := color.New(color.FgCyan)
	dim := color.New(color.Faint)
	bold := color.New(color.Bold)

	// Check for environment token first
	if token := auth.GetEnvAccessToken(); token != "" {
		cyan.Print("→ ")
		fmt.Println("Authenticated via ASTRO_ACCESS_TOKEN environment variable")
		return nil
	}

	storage := auth.NewStorage()

	// Load current profile
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return fmt.Errorf("not logged in. Run 'ast login' to authenticate")
	}

	if profile.AccessToken == "" {
		return fmt.Errorf("not logged in. Run 'ast login' to authenticate")
	}

	// Validate token is still valid (this will refresh if needed)
	tokenManager := auth.NewTokenManager()
	_, err = tokenManager.GetValidAccessToken(context.Background())
	if err != nil {
		return fmt.Errorf("session expired or invalid. Run 'ast login' to re-authenticate")
	}

	// Reload profile in case it was refreshed
	profile, err = storage.GetCurrentProfile()
	if err != nil {
		return fmt.Errorf("failed to load profile: %w", err)
	}

	// Display user info
	green.Print("✓ ")
	bold.Println("Authenticated")
	fmt.Println()

	if profile.User != nil {
		if profile.User.FirstName != "" || profile.User.LastName != "" {
			fmt.Printf("  Name:   %s %s\n", profile.User.FirstName, profile.User.LastName)
		}
		fmt.Printf("  Email:  %s\n", profile.User.Email)
		fmt.Print("  ID:     ")
		dim.Println(profile.User.ID)
	}

	if profile.ServerURL != "" {
		fmt.Print("  Server: ")
		dim.Println(profile.ServerURL)
	}

	if !profile.ExpiresAt.IsZero() {
		fmt.Print("  Expires: ")
		dim.Println(profile.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
	}

	return nil
}
