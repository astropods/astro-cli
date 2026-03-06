package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/auth"
	"github.com/postman/astro/apps/astro-cli/internal/theme"
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
when available, otherwise in ~/.ast/credentials.json with restricted permissions.

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
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	// Color definitions
	cyan := color.New(theme.PrimaryFatihAttr)
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
		yellow.Println("Login cancelled.") //nolint:errcheck,gosec
		cancel()
		os.Exit(1)
	}()

	// Create auth client
	client := auth.NewClient()

	// Request device authorization
	cyan.Print("→ ") //nolint:errcheck,gosec
	fmt.Println("Initiating authentication...")
	authResp, err := client.RequestDeviceAuthorization(ctx)
	if err != nil {
		return fmt.Errorf("failed to initiate authentication: %w", err)
	}

	// Display the user code prominently
	fmt.Println()
	fmt.Println("  ┌────────────────────────────────────────┐")
	fmt.Print("  │  Your verification code is: ")
	yellow.Printf("%-11s", authResp.UserCode) //nolint:errcheck,gosec
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
		cyan.Print("→ ") //nolint:errcheck,gosec
		fmt.Print("Opening browser to: ")
		dim.Println(verificationURL) //nolint:errcheck,gosec
		if err := browser.OpenURL(verificationURL); err != nil {
			yellow.Println("  Could not open browser automatically.") //nolint:errcheck,gosec
			fmt.Print("  Please visit: ")
			bold.Println(verificationURL) //nolint:errcheck,gosec
		}
	} else {
		fmt.Print("  Please visit: ")
		bold.Println(verificationURL) //nolint:errcheck,gosec
	}

	fmt.Println()
	cyan.Print("→ ") //nolint:errcheck,gosec
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
				cyan.Print("→ ") //nolint:errcheck,gosec
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
	if verbose {
		fmt.Printf("  %s GET %s\n", cyan.Sprint("→"), dim.Sprint(serverURL+"/api/v1/me")) //nolint:errcheck,gosec
	}
	accounts, err := fetchUserAccounts(serverURL, profile.AccessToken)
	if err == nil && len(accounts) > 0 {
		if verbose {
			fmt.Printf("  %s accounts: %s\n", cyan.Sprint("→"), dim.Sprintf("%d found (%s)", len(accounts), accounts[0].Name)) //nolint:errcheck,gosec
		}
		profile.Accounts = accounts
		// A personal account is required for login
		personalAcct := findPersonalAccount(accounts)
		if personalAcct != nil {
			profile.User.AccountName = personalAcct.Name
			profile.User.AccountID = personalAcct.ID
		} else {
			return fmt.Errorf("no personal account found. You must have a personal account to log in. Only organization accounts were found")
		}
	} else if verbose && err != nil {
		red := color.New(color.FgRed)
		fmt.Printf("  %s accounts: %s\n", red.Sprint("✗"), dim.Sprintf("fetch failed: %v", err)) //nolint:errcheck,gosec
	}

	// If no account exists, prompt the user to claim a username now
	if len(profile.Accounts) == 0 {
		fmt.Println()
		yellow.Println("  No account found. Choose a username to get started.") //nolint:errcheck,gosec
		account, claimErr := claimUsernameInteractive(serverURL, profile.AccessToken, verbose)
		if claimErr != nil {
			fmt.Println()
			yellow.Println("  Note: Username not set. Visit the dashboard to choose your username before pushing.") //nolint:errcheck,gosec
		} else {
			profile.Accounts = []auth.StoredAccount{account}
			profile.User.AccountName = account.Name
			profile.User.AccountID = account.ID
		}
	}

	if err := storage.SaveProfile("default", profile); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	// Success message
	fmt.Println()
	green.Print("✓ ") //nolint:errcheck,gosec
	bold.Println("Authentication successful!") //nolint:errcheck,gosec
	fmt.Println()

	if profile.User != nil {
		if profile.User.FirstName != "" || profile.User.LastName != "" {
			fmt.Printf("  Logged in as: %s %s ", profile.User.FirstName, profile.User.LastName)
			dim.Printf("(%s)\n", profile.User.Email) //nolint:errcheck,gosec
		} else {
			fmt.Printf("  Logged in as: %s\n", profile.User.Email)
		}
		if profile.User.AccountName != "" {
			fmt.Printf("  Account: %s\n", profile.User.AccountName)
		}
	}

	return nil
}

// claimUsernameInteractive prompts the user to pick a username and creates the account.
func claimUsernameInteractive(serverURL, accessToken string, verbose bool) (auth.StoredAccount, error) {
	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	for {
		var username string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Choose your username").
					Description("2-39 chars · lowercase letters, digits, hyphens · must start with a letter").
					Placeholder("your-username").
					Value(&username).
					Validate(func(v string) error {
						if err := validateUsernameLocal(v); err != nil {
							return err
						}
						if verbose {
							fmt.Printf("  %s GET %s\n", cyan.Sprint("→"), dim.Sprintf("%s/api/v1/accounts/check/%s", serverURL, v)) //nolint:errcheck,gosec
						}
						available, reason, err := checkAccountNameAvailability(serverURL, v)
						if verbose {
							if err != nil {
								red := color.New(color.FgRed)
								fmt.Printf("  %s check: %s\n", red.Sprint("✗"), dim.Sprintf("error: %v", err)) //nolint:errcheck,gosec
							} else {
								fmt.Printf("  %s check: %s\n", cyan.Sprint("→"), dim.Sprintf("available=%v reason=%q", available, reason)) //nolint:errcheck,gosec
							}
						}
						if err != nil {
							return fmt.Errorf("couldn't check availability: %w", err)
						}
						if !available {
							if reason != "" {
								return fmt.Errorf("%s", reason)
							}
							return fmt.Errorf("username is not available")
						}
						return nil
					}),
			),
		)

		if err := form.Run(); err != nil {
			return auth.StoredAccount{}, err
		}

		cyan.Print("→ ") //nolint:errcheck,gosec
		fmt.Println("Creating account...")
		if verbose {
			fmt.Printf("  %s POST %s\n", cyan.Sprint("→"), dim.Sprintf("%s/api/v1/accounts  name=%q", serverURL, username)) //nolint:errcheck,gosec
		}

		account, err := createAccount(serverURL, accessToken, username, verbose)
		if err != nil {
			fmt.Printf("  Failed to create account: %v. Please try a different username.\n", err)
			continue
		}

		return account, nil
	}
}

// validateUsernameLocal mirrors the server-side account name validation rules.
func validateUsernameLocal(name string) error {
	reserved := map[string]bool{
		"admin": true, "api": true, "auth": true, "deploy": true, "health": true,
		"login": true, "logout": true, "new": true, "onboarding": true, "operator": true,
		"register": true, "settings": true, "status": true, "support": true, "system": true,
		"www": true, "hire": true, "dev": true, "agents": true, "deployments": true,
	}

	if len(name) < 2 {
		return fmt.Errorf("must be at least 2 characters")
	}
	if len(name) > 39 {
		return fmt.Errorf("must be at most 39 characters")
	}
	if name != strings.ToLower(name) {
		return fmt.Errorf("must be lowercase")
	}
	if !unicode.IsLetter(rune(name[0])) {
		return fmt.Errorf("must start with a letter")
	}
	if name[len(name)-1] == '-' {
		return fmt.Errorf("must not end with a hyphen")
	}
	prevHyphen := false
	for _, ch := range name {
		if ch == '-' {
			if prevHyphen {
				return fmt.Errorf("must not contain consecutive hyphens")
			}
			prevHyphen = true
			continue
		}
		prevHyphen = false
		if !unicode.IsLower(ch) && !unicode.IsDigit(ch) {
			return fmt.Errorf("only lowercase letters, digits, and hyphens allowed")
		}
	}
	if reserved[name] {
		return fmt.Errorf("%q is a reserved name", name)
	}
	return nil
}

// checkAccountNameAvailability calls GET /api/v1/accounts/check/:name.
// Returns (available, reason, error).
func checkAccountNameAvailability(serverURL, name string) (bool, string, error) {
	reqURL := fmt.Sprintf("%s/api/v1/accounts/check/%s", serverURL, name)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		return false, "", err
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

	var result struct {
		Available bool   `json:"available"`
		Reason    string `json:"reason,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "", err
	}
	return result.Available, result.Reason, nil
}

// createAccount calls POST /api/v1/accounts to create a personal account.
func createAccount(serverURL, accessToken, name string, verbose bool) (auth.StoredAccount, error) {
	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	body, err := json.Marshal(map[string]string{"name": name, "type": "personal"})
	if err != nil {
		return auth.StoredAccount{}, fmt.Errorf("failed to encode request: %w", err)
	}
	reqURL := fmt.Sprintf("%s/api/v1/accounts", serverURL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return auth.StoredAccount{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return auth.StoredAccount{}, err
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

	if verbose {
		fmt.Printf("  %s status: %s\n", cyan.Sprint("→"), dim.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))) //nolint:errcheck,gosec
		if resp.Request.URL.String() != reqURL {
			fmt.Printf("  %s redirected to: %s\n", cyan.Sprint("→"), dim.Sprint(resp.Request.URL.String())) //nolint:errcheck,gosec
		}
	}

	// Accept 200 or 201 as success; anything else is an error.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error   string `json:"error"`
			Details string `json:"details,omitempty"`
		}
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errBody); jsonErr == nil && errBody.Error != "" {
			if errBody.Details != "" {
				return auth.StoredAccount{}, fmt.Errorf("%s: %s", errBody.Error, errBody.Details)
			}
			return auth.StoredAccount{}, fmt.Errorf("%s", errBody.Error)
		}
		return auth.StoredAccount{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return auth.StoredAccount{}, err
	}
	if result.ID == "" {
		return auth.StoredAccount{}, fmt.Errorf("invalid response from server")
	}
	return auth.StoredAccount{ID: result.ID, Name: result.Name, Type: result.Type, Role: "owner"}, nil
}

// fetchUserAccounts calls GET /api/v1/me on the server to get the user's accounts.
func fetchUserAccounts(serverURL, accessToken string) ([]auth.StoredAccount, error) {
	reqURL := fmt.Sprintf("%s/api/v1/me", serverURL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

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

// findPersonalAccount returns the first account with type "personal", or nil.
func findPersonalAccount(accounts []auth.StoredAccount) *auth.StoredAccount {
	for i := range accounts {
		if accounts[i].Type == "personal" {
			return &accounts[i]
		}
	}
	return nil
}
