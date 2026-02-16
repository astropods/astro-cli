package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/auth"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the Astro platform",
	Long: `Authenticate with the Astro platform using your browser.

This command initiates the OAuth device authorization flow:
1. A verification code is displayed in your terminal
2. Your browser opens to the authentication page
3. Enter the code and sign in with your account
4. The CLI automatically receives your credentials

Your Astro credentials and Astro server URL are stored in your system's keychain
when available, otherwise in ~/.astro/credentials.json with restricted permissions.

Example:
  ast login
  ast login --no-browser`,
	RunE: runLogin,
}

var noBrowser bool

func init() {
	rootCmd.AddCommand(loginCmd)
	loginCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Don't automatically open browser")
}

func runLogin(cmd *cobra.Command, args []string) error {
	// Color definitions
	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow, color.Bold)
	bold := color.New(color.Bold)
	dim := color.New(color.Faint)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println()
		yellow.Println("Login cancelled.")
		cancel()
		os.Exit(1)
	}()

	// Create auth client
	client := auth.NewClient()

	// Request device authorization
	cyan.Print("→ ")
	fmt.Println("Initiating authentication...")
	authResp, err := client.RequestDeviceAuthorization(ctx)
	if err != nil {
		return fmt.Errorf("failed to initiate authentication: %w", err)
	}

	// Display the user code prominently
	fmt.Println()
	fmt.Println("  ┌────────────────────────────────────────┐")
	fmt.Print("  │  Your verification code is: ")
	yellow.Printf("%-11s", authResp.UserCode)
	fmt.Println("│")
	fmt.Println("  └────────────────────────────────────────┘")
	fmt.Println()

	// Determine verification URL to display/open
	verificationURL := authResp.VerificationURIComplete
	if verificationURL == "" {
		verificationURL = authResp.VerificationURI
	}

	// Open browser if not disabled
	if !noBrowser {
		cyan.Print("→ ")
		fmt.Print("Opening browser to: ")
		dim.Println(verificationURL)
		if err := browser.OpenURL(verificationURL); err != nil {
			yellow.Println("  Could not open browser automatically.")
			fmt.Print("  Please visit: ")
			bold.Println(verificationURL)
		}
	} else {
		fmt.Print("  Please visit: ")
		bold.Println(verificationURL)
	}

	fmt.Println()
	cyan.Print("→ ")
	fmt.Print("Waiting for authentication")

	// Poll for tokens with a simple spinner
	done := make(chan struct{})
	go func() {
		dots := []string{"", ".", "..", "..."}
		i := 0
		for {
			select {
			case <-done:
				return
			case <-time.After(500 * time.Millisecond):
				fmt.Printf("\r")
				cyan.Print("→ ")
				fmt.Printf("Waiting for authentication%-4s", dots[i%len(dots)])
				i++
			}
		}
	}()

	tokenResp, err := client.PollForTokens(ctx, authResp.DeviceCode, authResp.Interval, authResp.ExpiresIn)
	close(done)
	fmt.Println()

	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Store credentials
	storage := auth.NewStorage(binaryName)

	profile := &auth.Profile{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	if tokenResp.User != nil {
		profile.User = &auth.StoredUser{
			ID:        tokenResp.User.ID,
			Email:     tokenResp.User.Email,
			FirstName: tokenResp.User.FirstName,
			LastName:  tokenResp.User.LastName,
		}
	}

	// Fetch user accounts from the server
	serverURL := auth.DefaultServerURL
	accounts, err := fetchUserAccounts(serverURL, profile.AccessToken)
	if err == nil && len(accounts) > 0 {
		profile.Accounts = accounts
		profile.User.AccountName = accounts[0].Name
		profile.User.AccountID = accounts[0].ID
	}

	if err := storage.SaveProfile("default", profile); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	// Success message
	fmt.Println()
	green.Print("✓ ")
	bold.Println("Authentication successful!")
	fmt.Println()

	if profile.User != nil {
		if profile.User.FirstName != "" || profile.User.LastName != "" {
			fmt.Printf("  Logged in as: %s %s ", profile.User.FirstName, profile.User.LastName)
			dim.Printf("(%s)\n", profile.User.Email)
		} else {
			fmt.Printf("  Logged in as: %s\n", profile.User.Email)
		}
		if profile.User.AccountName != "" {
			fmt.Printf("  Account: %s\n", profile.User.AccountName)
		}
	}

	if len(profile.Accounts) == 0 {
		fmt.Println()
		yellow.Println("  Note: No account found. Visit the dashboard to choose your username before pushing.")
	}

	return nil
}

// fetchUserAccounts calls GET /api/v1/me on the server to get the user's accounts.
func fetchUserAccounts(serverURL, accessToken string) ([]auth.StoredAccount, error) {
	reqURL := fmt.Sprintf("%s/api/v1/me", serverURL)
	req, err := http.NewRequestWithContext(context.Background(), "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result struct {
		Accounts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
			Role string `json:"role"`
		} `json:"accounts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var accounts []auth.StoredAccount
	for _, a := range result.Accounts {
		accounts = append(accounts, auth.StoredAccount{
			ID:   a.ID,
			Name: a.Name,
			Type: a.Type,
			Role: a.Role,
		})
	}
	return accounts, nil
}
