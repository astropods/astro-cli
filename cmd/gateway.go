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

// gatewayServerURLOverride is set in tests to redirect API calls to a test server.
var gatewayServerURLOverride string

func gatewayBaseURL() string {
	if gatewayServerURLOverride != "" {
		return strings.TrimSuffix(gatewayServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

// gatewayConfirm and gatewayPick are replaced in tests.
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

// Wire types for astro-server's dev-tool gateway routes.

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

// gatewayState is ~/.<binary>/gateway.json: what connect did to this machine,
// so status can report it and disconnect can undo exactly that.
type gatewayState struct {
	Account   string                 `json:"account,omitempty"`
	KeyID     string                 `json:"key_id,omitempty"`
	KeyPrefix string                 `json:"key_prefix,omitempty"`
	DeviceID  string                 `json:"device_id,omitempty"`
	Change    *claudesettings.Change `json:"change,omitempty"`
	// Disconnected lists accounts the user disconnected from on purpose, so
	// login's auto-connect does not reconnect them.
	Disconnected []string `json:"disconnected_accounts,omitempty"`
}

func gatewayStatePath() (string, error) {
	dir, err := auth.ConfigDir(buildinfo.BinaryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gateway.json"), nil
}

func gatewayDeviceIDPath() (string, error) {
	dir, err := auth.ConfigDir(buildinfo.BinaryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "device-id"), nil
}

func loadGatewayState() (*gatewayState, error) {
	path, err := gatewayStatePath()
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
	path, err := gatewayStatePath()
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

// errGatewayServerUnavailable is the one value for an environment without a
// gateway, so callers can tell it apart with errors.Is.
var errGatewayServerUnavailable = errGatewayUnavailable()

// gatewayAPIError maps the server's answers for these routes onto messages a
// developer can act on.
func gatewayAPIError(action, account string, status int, err error) error {
	switch status {
	case http.StatusConflict:
		if strings.Contains(err.Error(), "not enabled") {
			return errGatewayNotEnabled(account)
		}
		return errGatewaySetupInProgress()
	case http.StatusServiceUnavailable:
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

func fetchMyDeviceKeys(ctx context.Context, account string, all, verbose bool) ([]gatewayKeyMeta, error) {
	route := "my-keys"
	if all {
		route = "keys"
	}
	var list gatewayKeyList
	url := apiPath(gatewayBaseURL(), account, "accounts", "devtool-gateway", route)
	status, err := apiCallForAccount(ctx, http.MethodGet, url, nil, account, verbose, &list)
	if err != nil {
		return nil, gatewayAPIError("list device keys", account, status, err)
	}
	return list.Keys, nil
}

func revokeDeviceKey(ctx context.Context, account, keyID string, all, verbose bool) error {
	route := "my-keys"
	if all {
		route = "keys"
	}
	url := apiPath(gatewayBaseURL(), account, "accounts", "devtool-gateway", route, keyID)
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

// connectGateway sets this machine up for account. It checks every reason to
// stop before minting a key, so a refusal leaves nothing behind.
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

	idPath, err := gatewayDeviceIDPath()
	if err != nil {
		return err
	}
	device, err := deviceid.ID(idPath)
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()

	fmt.Fprintln(w, msgGatewayConnecting(opts.account))                                 //nolint:errcheck,gosec
	fmt.Fprintln(w)                                                                     //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Device     %s (%s %s)\n", hostname, runtime.GOOS, runtime.GOARCH) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Settings   %s\n\n", settingsPath)                                 //nolint:errcheck,gosec

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
	fmt.Fprintf(w, "✓ Issued a key for this device (%s…)\n", minted.Key.KeyPrefix) //nolint:errcheck,gosec

	change, err := claudesettings.Apply(settings, minted.Profile.UserSettings.Env, prior)
	if err != nil {
		_ = revokeDeviceKey(ctx, opts.account, minted.Key.KeyID, false, opts.verbose)
		return err
	}
	previous := *state
	state.Account = opts.account
	state.KeyID = minted.Key.KeyID
	state.KeyPrefix = minted.Key.KeyPrefix
	state.DeviceID = device
	state.Change = change
	state.forget(opts.account)
	// The record is written first: if saving the settings then fails, undo
	// skips any value that is not ours, so a stale record is harmless.
	if err := saveGatewayState(state); err != nil {
		_ = revokeDeviceKey(ctx, opts.account, minted.Key.KeyID, false, opts.verbose)
		return err
	}
	if err := settings.Save(); err != nil {
		_ = revokeDeviceKey(ctx, opts.account, minted.Key.KeyID, false, opts.verbose)
		_ = saveGatewayState(&previous)
		return err
	}
	fmt.Fprintf(w, "✓ Updated %s\n", settingsPath) //nolint:errcheck,gosec
	for _, key := range sortedEnvKeys(minted.Profile.UserSettings.Env) {
		label := minted.Profile.UserSettings.Env[key]
		if key == claudesettings.EnvCustomHeaders {
			label = strings.Join(claudesettings.HeaderNames(label), ", ")
		}
		fmt.Fprintf(w, "    %-26s %s\n", key, label) //nolint:errcheck,gosec
	}
	if switching {
		_ = revokeDeviceKey(ctx, previous.Account, previous.KeyID, false, opts.verbose)
	}
	fmt.Fprintf(w, "\n%s\n", msgGatewayConnected(opts.account, buildinfo.BinaryName)) //nolint:errcheck,gosec
	return nil
}

// gatewayResolve asks to proceed past a blocking condition. A non-interactive
// run never proceeds without --replace-existing, so a script cannot take over
// another gateway silently.
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
		fmt.Fprintf(w, "✓ Restored %s to how it was before connect\n", state.Change.Path) //nolint:errcheck,gosec
		for _, key := range kept {
			fmt.Fprintf(w, "  %s changed after connect, so it was left as it is\n", key) //nolint:errcheck,gosec
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
	fmt.Fprintf(w, "✓ Revoked this device's key (%s…)\n", prefix) //nolint:errcheck,gosec
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

// routingLayer is one place Claude Code can read ANTHROPIC_BASE_URL from,
// highest precedence first.
type routingLayer struct {
	name, path string
}

func managedSettingsPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode/managed-settings.json"
	case "windows":
		return `C:\ProgramData\ClaudeCode\managed-settings.json`
	}
	return "/etc/claude-code/managed-settings.json"
}

func routingLayers(userPath, cwd string) []routingLayer {
	return []routingLayer{
		{"managed settings (your organization)", managedSettingsPath()},
		{"this project's local settings", filepath.Join(cwd, ".claude", "settings.local.json")},
		{"this project's shared settings", filepath.Join(cwd, ".claude", "settings.json")},
		{"your user settings", userPath},
	}
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

	keys, keysErr := fetchMyDeviceKeys(ctx, state.Account, false, verbose)
	var mine *gatewayKeyMeta
	for i := range keys {
		if keys[i].KeyID == state.KeyID {
			mine = &keys[i]
		}
	}
	switch {
	case keysErr != nil:
		fmt.Fprintln(w, msgGatewayStatusUnreachable(state.Account, keysErr)) //nolint:errcheck,gosec
	case mine == nil:
		fmt.Fprintln(w, msgGatewayStatusKeyGone(state.Account, buildinfo.BinaryName)) //nolint:errcheck,gosec
	case mine.RevokedAt != nil:
		fmt.Fprintln(w, msgGatewayStatusRevoked(lastUsedLabel(mine.RevokedAt, now), buildinfo.BinaryName)) //nolint:errcheck,gosec
	default:
		fmt.Fprintln(w, msgGatewayStatusConnected(state.Account, mine.DeviceName, mine.KeyPrefix, lastUsedLabel(mine.LastUsedAt, now))) //nolint:errcheck,gosec
	}

	userPath, err := claudesettings.UserSettingsPath()
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	var winner *routingLayer
	var winningURL string
	var also []string
	for _, layer := range routingLayers(userPath, cwd) {
		f, err := claudesettings.Load(layer.path)
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
		also = append(also, layer.path)
	}
	if winner == nil {
		fmt.Fprintln(w, msgGatewayStatusNoRouting(buildinfo.BinaryName)) //nolint:errcheck,gosec
	} else {
		fmt.Fprintf(w, "  Routing       %s\n", winningURL) //nolint:errcheck,gosec
		fmt.Fprintf(w, "  Set by        %s", winner.name)  //nolint:errcheck,gosec
		if len(also) > 0 {
			fmt.Fprintf(w, ", also in %s", strings.Join(also, ", ")) //nolint:errcheck,gosec
		}
		fmt.Fprintln(w) //nolint:errcheck,gosec
		if winner.path != userPath && state.Change != nil {
			if want, ok := state.Change.Env[claudesettings.EnvBaseURL]; ok && want.Wrote != winningURL {
				fmt.Fprintln(w, msgGatewayStatusOverridden(winner.name)) //nolint:errcheck,gosec
			}
		}
	}
	if v := os.Getenv(claudesettings.EnvBaseURL); v != "" && v != winningURL {
		fmt.Fprintln(w, msgGatewayStatusShellExport(claudesettings.EnvBaseURL, v)) //nolint:errcheck,gosec
	}

	if f, err := claudesettings.Load(userPath); err == nil {
		headers, _, _ := f.Env(claudesettings.EnvCustomHeaders)
		vk, hasVK := claudesettings.HeaderValue(headers, "x-bf-vk")
		direct, hasDirect := claudesettings.HeaderValue(headers, "x-bf-direct-key")
		if !hasVK || !hasDirect || !strings.EqualFold(direct, "true") || !strings.HasPrefix(vk, state.KeyPrefix) {
			fmt.Fprintln(w, msgGatewayStatusHeadersMissing(buildinfo.BinaryName)) //nolint:errcheck,gosec
		}
	}
	return nil
}

func listGatewayDevices(ctx context.Context, w io.Writer, all, verbose bool, now time.Time) error {
	account, err := gatewayAccount(ctx, "")
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
		fmt.Fprintf(w, "No devices are connected to %s's AI Gateway.\n", account) //nolint:errcheck,gosec
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

// matchDeviceKey finds the key a user named, by full id or by key prefix. A
// trailing ellipsis, as the CLI prints prefixes, is accepted.
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
	account, err := gatewayAccount(ctx, "")
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
	fmt.Fprintf(w, "✓ Revoked %s's key (%s…)\n", key.DeviceName, key.KeyPrefix) //nolint:errcheck,gosec
	if state, err := loadGatewayState(); err == nil && state.KeyID == key.KeyID {
		fmt.Fprintln(w, msgGatewayRevokedThisDevice(buildinfo.BinaryName)) //nolint:errcheck,gosec
	}
	return nil
}

// gatewayAccount is the account a gateway command acts on: --account when
// given, else the account this machine is connected to, else the active one.
func gatewayAccount(ctx context.Context, flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
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

// gatewayAccountForConnect defaults to the active account, not the connected
// one: connect is how a machine switches accounts.
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
