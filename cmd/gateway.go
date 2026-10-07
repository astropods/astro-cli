package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/claudesettings"
	"github.com/astropods/astro-cli/internal/deviceid"
	"github.com/astropods/astro-cli/internal/tui"
)

var gatewayServerURLOverride string

func gatewayBaseURL() string {
	if gatewayServerURLOverride != "" {
		return strings.TrimSuffix(gatewayServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

var (
	gatewayConfirm = func(title, description string) (bool, error) {
		var ok bool
		form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(title).Description(description).Value(&ok)))
		if err := runForm(form); err != nil {
			return false, err
		}
		return ok, nil
	}
	gatewayPick = func(title, description string, options []string) (string, error) {
		var picked string
		opts := make([]huh.Option[string], 0, len(options))
		for _, o := range options {
			opts = append(opts, huh.NewOption(o, o))
		}
		form := huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Title(title).Description(description).Options(opts...).Value(&picked)))
		if err := runForm(form); err != nil {
			return "", err
		}
		return picked, nil
	}
)

type gatewayKeyMeta struct {
	KeyID         string     `json:"key_id"`
	KeyPrefix     string     `json:"key_prefix"`
	UserID        string     `json:"user_id"`
	Email         string     `json:"email"`
	DeviceID      string     `json:"device_id"`
	DeviceName    string     `json:"device_name"`
	OS            string     `json:"os"`
	Arch          string     `json:"arch"`
	ClientVersion string     `json:"client_version"`
	CreatedIP     string     `json:"created_ip"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

type gatewaySettings struct {
	Env map[string]string `json:"env"`
}

type gatewayMintResponse struct {
	Key     gatewayKeyMeta `json:"key"`
	APIKey  string         `json:"api_key"`
	Profile struct {
		BaseURL         string          `json:"base_url"`
		ManagedSettings gatewaySettings `json:"managed_settings"`
		UserSettings    gatewaySettings `json:"user_settings"`
	} `json:"profile"`
}

type gatewayKeyList struct {
	Keys []gatewayKeyMeta `json:"keys"`
}

type gatewayMine struct {
	BaseURL  string `json:"base_url"`
	Accounts []struct {
		Account            string `json:"account"`
		DisplayName        string `json:"display_name"`
		Type               string `json:"type"`
		AutoConnectOnLogin bool   `json:"auto_connect_on_login"`
	} `json:"accounts"`
}

// gatewayState is ~/.<binary>/gateway.json.
type gatewayState struct {
	Account   string                 `json:"account,omitempty"`
	KeyID     string                 `json:"key_id,omitempty"`
	KeyPrefix string                 `json:"key_prefix,omitempty"`
	DeviceID  string                 `json:"device_id,omitempty"`
	Change    *claudesettings.Change `json:"change,omitempty"`
	// Login's auto-connect skips Disconnected accounts.
	Disconnected []string `json:"disconnected_accounts,omitempty"`
}

func gatewayConfigFile(name string) (string, error) {
	dir, err := auth.ConfigDir(buildinfo.BinaryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func loadGatewayState() (*gatewayState, error) {
	path, err := gatewayConfigFile("gateway.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec
	if errors.Is(err, fs.ErrNotExist) {
		return &gatewayState{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s gatewayState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	return &s, nil
}

func saveGatewayState(s *gatewayState) error {
	path, err := gatewayConfigFile("gateway.json")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func (s *gatewayState) connected() bool { return s.Account != "" && s.KeyID != "" }

func (s *gatewayState) disconnectedFrom(account string) bool {
	for _, a := range s.Disconnected {
		if strings.EqualFold(a, account) {
			return true
		}
	}
	return false
}

func (s *gatewayState) forget(account string) {
	kept := s.Disconnected[:0]
	for _, a := range s.Disconnected {
		if !strings.EqualFold(a, account) {
			kept = append(kept, a)
		}
	}
	s.Disconnected = kept
}

var errGatewayServerUnavailable = errGatewayUnavailable()

const (
	gatewayDisabledCode        = "DEVTOOL_GATEWAY_DISABLED"
	gatewaySetupInProgressCode = "DEVTOOL_DEVICE_SETUP_IN_PROGRESS"
)

func gatewayAPIError(action, account string, status int, err error) error {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case gatewayDisabledCode:
			return errGatewayNotEnabled(account)
		case gatewaySetupInProgressCode:
			return errGatewaySetupInProgress()
		}
	}
	if status == http.StatusServiceUnavailable {
		return errGatewayServerUnavailable
	}
	return errGatewayRequestFailed(action, err)
}

func fetchMyGateways(ctx context.Context, account string, verbose bool) (*gatewayMine, error) {
	var mine gatewayMine
	status, err := apiCallForAccount(ctx, http.MethodGet, gatewayBaseURL()+"/api/v1/me/devtool-gateway", nil, account, verbose, &mine)
	if err != nil {
		return nil, gatewayAPIError("list your AI Gateway accounts", account, status, err)
	}
	return &mine, nil
}

func gatewayKeysRoute(all bool) string {
	if all {
		return "keys"
	}
	return "my-keys"
}

func fetchMyDeviceKeys(ctx context.Context, account string, all, verbose bool) ([]gatewayKeyMeta, error) {
	var list gatewayKeyList
	url := apiPath(gatewayBaseURL(), account, "accounts", "devtool-gateway", gatewayKeysRoute(all))
	status, err := apiCallForAccount(ctx, http.MethodGet, url, nil, account, verbose, &list)
	if err != nil {
		return nil, gatewayAPIError("list device keys", account, status, err)
	}
	return list.Keys, nil
}

func revokeDeviceKey(ctx context.Context, account, keyID string, all, verbose bool) error {
	url := apiPath(gatewayBaseURL(), account, "accounts", "devtool-gateway", gatewayKeysRoute(all), keyID)
	status, err := apiCallForAccount(ctx, http.MethodDelete, url, nil, account, verbose, nil)
	if status == http.StatusNotFound {
		return errGatewayKeyNotFound(keyID)
	}
	if err != nil {
		return gatewayAPIError("revoke the device key", account, status, err)
	}
	return nil
}

type gatewayConnectOptions struct {
	account         string
	replaceExisting bool
	interactive     bool
	verbose         bool
}

func connectGateway(ctx context.Context, w io.Writer, opts gatewayConnectOptions) error {
	state, err := loadGatewayState()
	if err != nil {
		return err
	}
	mine, err := fetchMyGateways(ctx, opts.account, opts.verbose)
	if err != nil {
		return err
	}
	if !gatewayEnabledFor(mine, opts.account) {
		return errGatewayNotEnabled(opts.account)
	}

	settingsPath, err := claudesettings.UserSettingsPath()
	if err != nil {
		return err
	}
	settings, err := claudesettings.Load(settingsPath)
	if err != nil {
		return err
	}
	prior := state.Change
	if prior != nil && prior.Path != settingsPath {
		prior = nil
	}

	switching := state.connected() && !strings.EqualFold(state.Account, opts.account)
	if switching {
		if err := gatewayResolve(opts, errGatewayOtherAccount(state.Account, opts.account), func() (string, string) {
			return msgGatewayConfirmOtherAccount(state.Account, opts.account)
		}); err != nil {
			return err
		}
	}
	conflicts, err := claudesettings.Conflicts(settings, map[string]string{claudesettings.EnvBaseURL: mine.BaseURL}, prior)
	if err != nil {
		return err
	}
	for _, c := range conflicts {
		if err := gatewayResolve(opts, errGatewayForeignBaseURL(settingsPath, c.Current), func() (string, string) {
			return msgGatewayConfirmForeignBaseURL(settingsPath, c.Current)
		}); err != nil {
			return err
		}
	}

	idPath, err := gatewayConfigFile("device-id")
	if err != nil {
		return err
	}
	device, err := deviceid.ID(idPath)
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()

	fmt.Fprintln(w, msgGatewayConnecting(opts.account))                                            //nolint:errcheck,gosec
	fmt.Fprintln(w)                                                                                //nolint:errcheck,gosec
	fmt.Fprintln(w, msgGatewayConnectTarget(hostname, runtime.GOOS, runtime.GOARCH, settingsPath)) //nolint:errcheck,gosec

	var minted gatewayMintResponse
	url := apiPath(gatewayBaseURL(), opts.account, "accounts", "devtool-gateway", "my-keys")
	status, err := apiCallForAccount(ctx, http.MethodPost, url, map[string]string{
		"device_id":      device,
		"device_name":    hostname,
		"os":             runtime.GOOS,
		"arch":           runtime.GOARCH,
		"client_version": buildinfo.BinaryName + " " + buildinfo.Version,
	}, opts.account, opts.verbose, &minted)
	if err != nil {
		return gatewayAPIError("issue a key for this device", opts.account, status, err)
	}
	fmt.Fprintln(w, msgGatewayKeyIssued(minted.Key.KeyPrefix)) //nolint:errcheck,gosec

	abandon := func(err error) error {
		_ = revokeDeviceKey(ctx, opts.account, minted.Key.KeyID, false, opts.verbose)
		return errGatewayConnectIncomplete(buildinfo.BinaryName, err)
	}
	change, err := claudesettings.Apply(settings, minted.Profile.UserSettings.Env, prior)
	if err != nil {
		return abandon(err)
	}
	previous := *state
	state.Account = opts.account
	state.KeyID = minted.Key.KeyID
	state.KeyPrefix = minted.Key.KeyPrefix
	state.DeviceID = device
	state.Change = change
	state.forget(opts.account)
	// Write the record before the settings: undo skips any value that isn't ours.
	if err := saveGatewayState(state); err != nil {
		return abandon(err)
	}
	if err := settings.Save(); err != nil {
		_ = saveGatewayState(&previous)
		return abandon(err)
	}
	fmt.Fprintln(w, msgGatewaySettingsUpdated(settingsPath)) //nolint:errcheck,gosec
	for _, key := range sortedEnvKeys(minted.Profile.UserSettings.Env) {
		fmt.Fprintln(w, msgGatewaySettingWritten(key, gatewaySettingLabel(key, minted.Profile.UserSettings.Env[key]))) //nolint:errcheck,gosec
	}
	if switching {
		if err := revokeDeviceKey(ctx, previous.Account, previous.KeyID, false, opts.verbose); err != nil {
			fmt.Fprintln(w, msgGatewaySwitchRevokeFailed(previous.Account, previous.KeyPrefix, buildinfo.BinaryName, err)) //nolint:errcheck,gosec
		}
	}
	fmt.Fprintf(w, "\n%s\n", msgGatewayConnected(opts.account, buildinfo.BinaryName)) //nolint:errcheck,gosec
	return nil
}

func gatewayResolve(opts gatewayConnectOptions, refusal error, prompt func() (string, string)) error {
	if opts.replaceExisting {
		return nil
	}
	if !opts.interactive {
		return refusal
	}
	title, description := prompt()
	ok, err := gatewayConfirm(title, description)
	if err != nil {
		return err
	}
	if !ok {
		return tui.ErrCanceled
	}
	return nil
}

func gatewayEnabledFor(mine *gatewayMine, account string) bool {
	for _, a := range mine.Accounts {
		if strings.EqualFold(a.Account, account) {
			return true
		}
	}
	return false
}

// Any other key can hold a credential, so its value is never printed.
func gatewaySettingLabel(key, value string) string {
	switch key {
	case claudesettings.EnvBaseURL:
		return value
	case claudesettings.EnvCustomHeaders:
		return strings.Join(claudesettings.HeaderNames(value), ", ")
	}
	return msgGatewaySettingHidden()
}

func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func disconnectGateway(ctx context.Context, w io.Writer, verbose bool) error {
	state, err := loadGatewayState()
	if err != nil {
		return err
	}
	if !state.connected() {
		fmt.Fprintln(w, msgGatewayNotConnected(buildinfo.BinaryName)) //nolint:errcheck,gosec
		return nil
	}
	if state.Change != nil {
		settings, err := claudesettings.Load(state.Change.Path)
		if err != nil {
			return err
		}
		kept, err := claudesettings.Undo(settings, state.Change)
		if err != nil {
			return err
		}
		if err := settings.Save(); err != nil {
			return err
		}
		fmt.Fprintln(w, msgGatewaySettingsRestored(state.Change.Path)) //nolint:errcheck,gosec
		for _, key := range kept {
			fmt.Fprintln(w, msgGatewaySettingKept(key)) //nolint:errcheck,gosec
		}
	}

	account, keyID, prefix := state.Account, state.KeyID, state.KeyPrefix
	state.forget(account)
	state.Disconnected = append(state.Disconnected, account)
	state.Account, state.KeyID, state.KeyPrefix, state.Change = "", "", "", nil
	if err := saveGatewayState(state); err != nil {
		return err
	}
	if err := revokeDeviceKey(ctx, account, keyID, false, verbose); err != nil {
		fmt.Fprintln(w, msgGatewayDisconnectRevokeFailed(prefix, buildinfo.BinaryName, err)) //nolint:errcheck,gosec
		return nil
	}
	fmt.Fprintln(w, msgGatewayThisDeviceKeyRevoked(prefix)) //nolint:errcheck,gosec
	return nil
}

func lastUsedLabel(t *time.Time, now time.Time) string {
	if t == nil {
		return "never"
	}
	d := now.Sub(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	}
	return plural(int(d.Hours()/24), "day") + " ago"
}

func statusGateway(ctx context.Context, w io.Writer, verbose bool, now time.Time) error {
	state, err := loadGatewayState()
	if err != nil {
		return err
	}
	if !state.connected() {
		fmt.Fprintln(w, msgGatewayNotConnected(buildinfo.BinaryName)) //nolint:errcheck,gosec
		return nil
	}
	reportGatewayKey(ctx, w, state, verbose, now)
	if mine, err := fetchMyGateways(ctx, state.Account, verbose); err == nil && !gatewayEnabledFor(mine, state.Account) {
		fmt.Fprintln(w, msgGatewayStatusCollectionOff(state.Account, buildinfo.BinaryName)) //nolint:errcheck,gosec
	}

	userPath, err := claudesettings.UserSettingsPath()
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	reportGatewayRouting(w, state, userPath, cwd)

	if f, err := claudesettings.Load(userPath); err == nil && !gatewaySettingsIntact(f, state.Change) {
		fmt.Fprintln(w, msgGatewayStatusSettingsMissing(buildinfo.BinaryName)) //nolint:errcheck,gosec
	}
	return nil
}

func reportGatewayKey(ctx context.Context, w io.Writer, state *gatewayState, verbose bool, now time.Time) {
	keys, err := fetchMyDeviceKeys(ctx, state.Account, false, verbose)
	var mine *gatewayKeyMeta
	for i := range keys {
		if keys[i].KeyID == state.KeyID {
			mine = &keys[i]
		}
	}
	switch {
	case err != nil:
		fmt.Fprintln(w, msgGatewayStatusUnreachable(state.Account, err)) //nolint:errcheck,gosec
	case mine == nil:
		fmt.Fprintln(w, msgGatewayStatusKeyGone(state.Account, buildinfo.BinaryName)) //nolint:errcheck,gosec
	case mine.RevokedAt != nil:
		fmt.Fprintln(w, msgGatewayStatusRevoked(lastUsedLabel(mine.RevokedAt, now), buildinfo.BinaryName)) //nolint:errcheck,gosec
	default:
		fmt.Fprintln(w, msgGatewayStatusConnected(state.Account, mine.DeviceName, mine.KeyPrefix, lastUsedLabel(mine.LastUsedAt, now))) //nolint:errcheck,gosec
	}
}

func reportGatewayRouting(w io.Writer, state *gatewayState, userPath, cwd string) {
	var winner *claudesettings.Layer
	var winningURL string
	var also []string
	for _, layer := range claudesettings.Layers(userPath, cwd) {
		f, err := claudesettings.Load(layer.Path)
		if err != nil {
			continue
		}
		v, ok, _ := f.Env(claudesettings.EnvBaseURL)
		if !ok || v == "" {
			continue
		}
		if winner == nil {
			l := layer
			winner, winningURL = &l, v
			continue
		}
		also = append(also, layer.Path)
	}
	if winner == nil {
		fmt.Fprintln(w, msgGatewayStatusNoRouting(buildinfo.BinaryName)) //nolint:errcheck,gosec
	} else {
		fmt.Fprintln(w, msgGatewayStatusRouting(winningURL, msgGatewaySettingsScope(winner.Scope), also)) //nolint:errcheck,gosec
		if winner.Scope != claudesettings.ScopeUser && state.Change != nil {
			if want, ok := state.Change.Env[claudesettings.EnvBaseURL]; ok && want.Wrote != winningURL {
				fmt.Fprintln(w, msgGatewayStatusOverridden(msgGatewaySettingsScope(winner.Scope))) //nolint:errcheck,gosec
			}
		}
	}
	if v := os.Getenv(claudesettings.EnvBaseURL); v != "" && v != winningURL {
		fmt.Fprintln(w, msgGatewayStatusShellExport(claudesettings.EnvBaseURL, v)) //nolint:errcheck,gosec
	}
}

func gatewaySettingsIntact(f *claudesettings.File, change *claudesettings.Change) bool {
	if change == nil {
		return true
	}
	for key, edit := range change.Env {
		current, present, err := f.Env(key)
		if err != nil || !present {
			return false
		}
		if key != claudesettings.EnvCustomHeaders {
			if current != edit.Wrote {
				return false
			}
			continue
		}
		for _, name := range edit.HeaderNames {
			wrote, _ := claudesettings.HeaderValue(edit.Wrote, name)
			if got, ok := claudesettings.HeaderValue(current, name); !ok || got != wrote {
				return false
			}
		}
	}
	return true
}

func listGatewayDevices(ctx context.Context, w io.Writer, all, verbose bool, now time.Time) error {
	account, err := gatewayAccount(ctx)
	if err != nil {
		return err
	}
	state, err := loadGatewayState()
	if err != nil {
		return err
	}
	keys, err := fetchMyDeviceKeys(ctx, account, all, verbose)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		fmt.Fprintln(w, msgGatewayNoDevices(account)) //nolint:errcheck,gosec
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := "DEVICE\tOS\tKEY\tCONNECTED\tLAST USED\tSTATUS"
	if all {
		header = "MEMBER\t" + header
	}
	fmt.Fprintln(tw, header) //nolint:errcheck,gosec
	for _, k := range keys {
		status := "active"
		if k.RevokedAt != nil {
			status = "revoked"
		}
		name := k.DeviceName
		if k.KeyID == state.KeyID {
			name += " (this device)"
		}
		row := fmt.Sprintf("%s\t%s %s\t%s…\t%s\t%s\t%s", name, k.OS, k.Arch, k.KeyPrefix,
			k.CreatedAt.Local().Format("2006-01-02"), lastUsedLabel(k.LastUsedAt, now), status)
		if all {
			row = k.Email + "\t" + row
		}
		fmt.Fprintln(tw, row) //nolint:errcheck,gosec
	}
	return tw.Flush()
}

func matchDeviceKey(keys []gatewayKeyMeta, query string) (*gatewayKeyMeta, error) {
	q := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(query), "…"), "...")
	var matches []*gatewayKeyMeta
	for i := range keys {
		k := &keys[i]
		if k.KeyID == q {
			return k, nil
		}
		if k.RevokedAt == nil && q != "" && (strings.HasPrefix(k.KeyPrefix, q) || strings.HasPrefix(q, k.KeyPrefix)) {
			matches = append(matches, k)
		}
	}
	switch len(matches) {
	case 0:
		return nil, errGatewayKeyNotFound(query)
	case 1:
		return matches[0], nil
	}
	return nil, errGatewayKeyAmbiguous(query, len(matches))
}

func revokeGatewayDevice(ctx context.Context, w io.Writer, query string, all, verbose bool) error {
	account, err := gatewayAccount(ctx)
	if err != nil {
		return err
	}
	keys, err := fetchMyDeviceKeys(ctx, account, all, verbose)
	if err != nil {
		return err
	}
	key, err := matchDeviceKey(keys, query)
	if err != nil {
		return err
	}
	if err := revokeDeviceKey(ctx, account, key.KeyID, all, verbose); err != nil {
		return err
	}
	fmt.Fprintln(w, msgGatewayDeviceKeyRevoked(key.DeviceName, key.KeyPrefix)) //nolint:errcheck,gosec
	if state, err := loadGatewayState(); err == nil && state.KeyID == key.KeyID {
		fmt.Fprintln(w, msgGatewayRevokedThisDevice(buildinfo.BinaryName)) //nolint:errcheck,gosec
	}
	return nil
}

func gatewayAccount(ctx context.Context) (string, error) {
	if state, err := loadGatewayState(); err == nil && state.connected() {
		return state.Account, nil
	}
	at, err := getCurrentAccountToken(ctx)
	if err != nil {
		return "", err
	}
	return at.Account, nil
}

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Route this machine's Claude Code through your organization's AI Gateway",
	Long: `Route this machine's Claude Code through your organization's AI Gateway, so its
usage appears in your organization's Insights.

Claude Code keeps your existing login and billing. Connect writes two settings to
your Claude Code user settings and records exactly what it changed, so disconnect
can put them back.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var gatewayConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect this machine to the AI Gateway",
	Long: `Connect this machine to the AI Gateway for an account. Running it again replaces
this machine's key. A machine reports to one account at a time.

Without a terminal, connect never replaces another gateway or another account's
connection unless --replace-existing is given.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
		flag, _ := cmd.Flags().GetString("account")
		replace, _ := cmd.Flags().GetBool("replace-existing")
		account, err := gatewayAccountForConnect(cmd.Context(), flag)
		if err != nil {
			return err
		}
		err = connectGateway(cmd.Context(), cmd.OutOrStdout(), gatewayConnectOptions{
			account:         account,
			replaceExisting: replace,
			interactive:     interactiveTerminal(),
			verbose:         verbose,
		})
		if errors.Is(err, tui.ErrCanceled) {
			printCanceled(cmd.OutOrStdout())
			return nil
		}
		return err
	},
}

// Unlike gatewayAccount, this defaults to the active account: connect is how a machine switches accounts.
func gatewayAccountForConnect(ctx context.Context, flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	at, err := getCurrentAccountToken(ctx)
	if err != nil {
		return "", err
	}
	return at.Account, nil
}

var gatewayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether this machine routes through the AI Gateway",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
		return statusGateway(cmd.Context(), cmd.OutOrStdout(), verbose, time.Now())
	},
}

var gatewayDevicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List your devices connected to the AI Gateway",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
		all, _ := cmd.Flags().GetBool("all")
		return listGatewayDevices(cmd.Context(), cmd.OutOrStdout(), all, verbose, time.Now())
	},
}

var gatewayRevokeCmd = &cobra.Command{
	Use:   "revoke <key>",
	Short: "Revoke one of your devices' keys, by key prefix or id",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
		all, _ := cmd.Flags().GetBool("all")
		return revokeGatewayDevice(cmd.Context(), cmd.OutOrStdout(), args[0], all, verbose)
	},
}

var gatewayDisconnectCmd = &cobra.Command{
	Use:   "disconnect",
	Short: "Disconnect this machine and undo what connect changed",
	Long: `Restore your Claude Code settings to how they were before connect, then revoke
this machine's key. The settings are restored first, so disconnect works even
when the server can't be reached.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
		return disconnectGateway(cmd.Context(), cmd.OutOrStdout(), verbose)
	},
}

func init() {
	gatewayConnectCmd.Flags().String("account", "", "Account to connect to (default: the active account)")
	gatewayConnectCmd.Flags().Bool("replace-existing", false, "Replace another gateway or account's connection without asking")
	gatewayDevicesCmd.Flags().Bool("all", false, "List every member's devices (requires permission to manage data sources)")
	gatewayRevokeCmd.Flags().Bool("all", false, "Revoke any member's device key (requires permission to manage data sources)")
	gatewayCmd.AddCommand(gatewayConnectCmd, gatewayStatusCmd, gatewayDevicesCmd, gatewayRevokeCmd, gatewayDisconnectCmd)
	rootCmd.AddCommand(gatewayCmd)
}
