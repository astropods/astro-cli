package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
)

// accountNewStorage is the storage constructor used by account commands. Overridable in tests.
var accountNewStorage = func() *auth.Storage { return auth.NewStorage(buildinfo.BinaryName) }

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage accounts",
	Long:  "List accounts you belong to and switch the active account.",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "List accounts you belong to",
	Args:  cobra.NoArgs,
	RunE:  runAccountList,
}

var accountSwitchCmd = &cobra.Command{
	Use:   "switch [name]",
	Short: "Set the active account",
	Long: `Set the active account. All commands that operate on platform resources
(blueprints, agents, secrets) will be scoped to this account.

If no name is given, an interactive selector is shown. Use - to switch back
to the previously active account.`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runAccountSwitch,
}

var accountTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print an API token scoped to the active account",
	Long: `Print an access token scoped to the active account.

For personal accounts this is your personal access token.
For organization accounts this is an org-scoped access token.`,
	Args: cobra.NoArgs,
	RunE: runAccountToken,
}

func init() {
	accountCmd.AddCommand(accountListCmd)
	accountCmd.AddCommand(accountSwitchCmd)
	if buildinfo.BuildType == buildinfo.BuildTypeDev {
		accountCmd.AddCommand(accountTokenCmd)
	}
	rootCmd.AddCommand(accountCmd)
}

func runAccountList(cmd *cobra.Command, args []string) error {
	storage := accountNewStorage()

	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return fmt.Errorf("not logged in. Run '%s login' to authenticate", buildinfo.BinaryName)
	}

	currentAccount, err := storage.GetCurrentAccount()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	green := color.New(color.FgGreen)
	cyan := color.New(theme.PrimaryFatihAttr)

	for _, a := range profile.Accounts {
		name := a.Name
		if a.Type == "personal" {
			name = fmt.Sprintf("%s (personal)", a.Name)
		}

		if a.Name == currentAccount {
			green.Fprint(w, "✓ ")  //nolint:errcheck,gosec
			cyan.Fprintln(w, name) //nolint:errcheck,gosec
		} else {
			fmt.Fprintf(w, "  %s\n", name) //nolint:errcheck,gosec
		}
	}

	return nil
}

func runAccountSwitch(cmd *cobra.Command, args []string) error {
	storage := accountNewStorage()
	w := cmd.OutOrStdout()
	green := color.New(color.FgGreen)

	name := ""
	if len(args) == 1 {
		name = args[0]
	}

	if name == "" {
		var err error
		name, err = selectAccountInteractive(storage)
		if err != nil {
			return err
		}
	}

	if name == "-" {
		switched, err := storage.SwitchToPreviousAccount()
		if err != nil {
			return err
		}
		green.Fprint(w, "✓ ")                                //nolint:errcheck,gosec
		fmt.Fprintf(w, "Switched to account %q\n", switched) //nolint:errcheck,gosec
		return nil
	}

	if err := storage.SetCurrentAccount(name); err != nil {
		return err
	}

	green.Fprint(w, "✓ ")                            //nolint:errcheck,gosec
	fmt.Fprintf(w, "Switched to account %q\n", name) //nolint:errcheck,gosec
	return nil
}

func runAccountToken(cmd *cobra.Command, args []string) error {
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{
		"account":    at.Account,
		"token":      at.Token,
		"expires_at": at.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// AccountToken holds the resolved account name, its scoped API token, and expiry.
type AccountToken struct {
	Account   string
	Token     string
	ExpiresAt time.Time
}

// getCurrentAccountToken returns the active account name and an API token scoped to it.
// For organization accounts it returns an org-scoped token; for personal accounts it
// returns the personal access token. Other commands use this as the single call to
// obtain both the account and credentials without duplicating resolution logic.
func getCurrentAccountToken(ctx context.Context) (AccountToken, error) {
	account, err := accountNewStorage().GetCurrentAccount()
	if err != nil {
		return AccountToken{}, err
	}
	token, err := getAccountToken(ctx, account)
	if err != nil {
		return AccountToken{}, err
	}
	expiresAt, _ := auth.ParseJWTExpiry(token)
	return AccountToken{Account: account, Token: token, ExpiresAt: expiresAt}, nil
}

// getAccountToken returns an API token scoped to the named account.
// For organization accounts it returns an org-scoped token; for personal
// accounts it returns the personal access token. Other commands use this to
// obtain credentials without duplicating account and token resolution logic.
func getAccountToken(ctx context.Context, account string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	storage := accountNewStorage()
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not logged in. Run '%s login' to authenticate", buildinfo.BinaryName)
	}
	var orgID string
	for _, a := range profile.Accounts {
		if strings.EqualFold(a.Name, account) && a.Type == "organization" {
			orgID = a.WorkOSOrganizationID
			break
		}
	}
	tokenManager := auth.NewTokenManager(buildinfo.BinaryName)
	var token string
	if orgID != "" {
		token, err = tokenManager.GetOrgScopedAccessToken(ctx, orgID)
	} else {
		token, err = tokenManager.GetValidAccessToken(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("authentication failed: %w. Run '%s login' to re-authenticate", err, buildinfo.BinaryName)
	}
	return token, nil
}

func selectAccountInteractive(storage *auth.Storage) (string, error) {
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not logged in. Run 'ast login' to authenticate")
	}

	currentAccount, _ := storage.GetCurrentAccount()

	options := make([]huh.Option[string], 0, len(profile.Accounts))
	for _, a := range profile.Accounts {
		label := a.Name
		if a.Type == "personal" {
			label = fmt.Sprintf("%s (personal)", a.Name)
		}
		if a.Name == currentAccount {
			label = fmt.Sprintf("%s ✓", label)
		}
		options = append(options, huh.NewOption(label, a.Name))
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select account").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return selected, nil
}
