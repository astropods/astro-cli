package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
	"github.com/astropods/astro-cli/internal/tui"
)

// accountNewStorage is the storage constructor used by account commands. Overridable in tests.
var accountNewStorage = func() *auth.Storage { return auth.NewStorage(buildinfo.BinaryName) }

// accountServerURLOverride is set in tests to redirect API calls to a test server.
var accountServerURLOverride string

func accountBaseURL() string {
	if accountServerURLOverride != "" {
		return strings.TrimSuffix(accountServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

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

The token is scoped to that account's organization, so it carries the
permissions you hold there.`,
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

	if _, err := storage.GetCurrentProfile(); err != nil {
		return fmt.Errorf("not logged in. Run '%s login' to authenticate", buildinfo.BinaryName)
	}
	accounts, err := accountsForSelection(cmd.Context(), storage)
	if err != nil {
		return err
	}

	currentAccount, err := storage.GetCurrentAccount()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	green := color.New(color.FgGreen)
	cyan := color.New(theme.PrimaryFatihAttr)

	for _, a := range accounts {
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
		name, err = selectAccountInteractive(cmd.Context(), storage)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				printCancelled(w)
				return nil
			}
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

	refreshAccountsIfMissing(cmd.Context(), storage, name)
	if err := storage.SetCurrentAccount(name); err != nil {
		return err
	}

	green.Fprint(w, "✓ ")                            //nolint:errcheck,gosec
	fmt.Fprintf(w, "Switched to account %q\n", name) //nolint:errcheck,gosec
	return nil
}

// refreshAccounts fetches and persists the live account list, refreshing the
// access token first if it's close to expiry.
func refreshAccounts(ctx context.Context, storage *auth.Storage) ([]auth.StoredAccount, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	token, err := auth.NewTokenManager(buildinfo.BinaryName).GetValidAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := fetchUserAccounts(accountBaseURL(), token)
	if err != nil {
		return nil, err
	}
	if err := storage.SetAccounts(accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

// Best-effort: a failed refresh falls through to SetCurrentAccount's own error.
func refreshAccountsIfMissing(ctx context.Context, storage *auth.Storage, name string) {
	profile, err := storage.GetCurrentProfile()
	if err != nil || auth.HasAccount(profile.Accounts, name) {
		return
	}
	_, _ = refreshAccounts(ctx, storage)
}

// Unlike refreshAccountsIfMissing, always refreshes, falling back to the cache on failure.
func accountsForSelection(ctx context.Context, storage *auth.Storage) ([]auth.StoredAccount, error) {
	if accounts, err := refreshAccounts(ctx, storage); err == nil {
		return accounts, nil
	}
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return nil, err
	}
	return profile.Accounts, nil
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

func getAccountToken(ctx context.Context, account string) (string, error) {
	return accountToken(ctx, account, false)
}

func forceAccountToken(ctx context.Context, account string) (string, error) {
	return accountToken(ctx, account, true)
}

func accountOrgID(accounts []auth.StoredAccount, account string) string {
	for _, a := range accounts {
		if strings.EqualFold(a.Name, account) {
			return a.OrganizationID
		}
	}
	return ""
}

func accountToken(ctx context.Context, account string, force bool) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	storage := accountNewStorage()
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not logged in. Run '%s login' to authenticate", buildinfo.BinaryName)
	}
	orgID := accountOrgID(profile.Accounts, account)
	tokenManager := auth.NewTokenManager(buildinfo.BinaryName)
	var token string
	if orgID != "" {
		token, err = tokenManager.GetOrgScopedAccessToken(ctx, orgID)
	} else if force {
		token, err = tokenManager.ForceRefreshAccessToken(ctx)
	} else {
		token, err = tokenManager.GetValidAccessToken(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("authentication failed: %w. Run '%s login' to re-authenticate", err, buildinfo.BinaryName)
	}
	return token, nil
}

func selectAccountInteractive(ctx context.Context, storage *auth.Storage) (string, error) {
	if _, err := storage.GetCurrentProfile(); err != nil {
		return "", fmt.Errorf("not logged in. Run '%s login' to authenticate", buildinfo.BinaryName)
	}

	accounts, err := accountsForSelection(ctx, storage)
	if err != nil {
		return "", err
	}

	currentAccount, _ := storage.GetCurrentAccount()

	options := make([]huh.Option[string], 0, len(accounts))
	for _, a := range accounts {
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
	if err := runForm(form); err != nil {
		return "", err
	}
	return selected, nil
}
