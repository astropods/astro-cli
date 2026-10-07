package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/claudesettings"
	"github.com/astropods/astro-cli/internal/deviceid"
	"github.com/astropods/astro-cli/internal/tui"
)

const testGatewayURL = "https://aig.example.com/anthropic"

type fakeGatewayServer struct {
	mu          sync.Mutex
	enabled     map[string]bool // account -> auto_connect_on_login
	keys        map[string][]gatewayKeyMeta
	mints       []map[string]string
	revoked     []string
	unavailable bool
	failRevoke  bool
	mintRefusal string
	userEnv     func(apiKey string) map[string]string
	seq         int
}

func (f *fakeGatewayServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.unavailable {
			http.Error(w, `{"error":"AI Gateway is not configured in this environment"}`, http.StatusServiceUnavailable)
			return
		}
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == "/api/v1/me/devtool-gateway":
			type acct struct {
				Account            string `json:"account"`
				AutoConnectOnLogin bool   `json:"auto_connect_on_login"`
			}
			out := struct {
				BaseURL  string `json:"base_url"`
				Accounts []acct `json:"accounts"`
			}{BaseURL: testGatewayURL, Accounts: []acct{}}
			for name, auto := range f.enabled {
				out.Accounts = append(out.Accounts, acct{Account: name, AutoConnectOnLogin: auto})
			}
			_ = json.NewEncoder(w).Encode(out)
		case strings.HasSuffix(path, "/devtool-gateway/my-keys") && r.Method == http.MethodPost:
			account := accountFromPath(path)
			if _, ok := f.enabled[account]; !ok {
				http.Error(w, `{"error":"the dev-tool gateway is not enabled for this account","code":"DEVTOOL_GATEWAY_DISABLED"}`, http.StatusConflict)
				return
			}
			if f.mintRefusal != "" {
				http.Error(w, f.mintRefusal, http.StatusConflict)
				return
			}
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mints = append(f.mints, body)
			f.seq++
			apiKey := fmt.Sprintf("sk-bf-%04dkeykeykey", f.seq)
			meta := gatewayKeyMeta{
				KeyID: fmt.Sprintf("vk-%d", f.seq), KeyPrefix: apiKey[:10], Email: "user@example.com",
				DeviceID: body["device_id"], DeviceName: body["device_name"], OS: body["os"], Arch: body["arch"],
				CreatedAt: time.Now().UTC(),
			}
			f.keys[account] = append(f.keys[account], meta)
			userEnv := map[string]string{
				claudesettings.EnvBaseURL:       testGatewayURL,
				claudesettings.EnvCustomHeaders: "x-bf-direct-key: true\nx-bf-vk: " + apiKey,
			}
			if f.userEnv != nil {
				userEnv = f.userEnv(apiKey)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"key": meta, "api_key": apiKey,
				"profile": map[string]any{
					"base_url":         testGatewayURL,
					"managed_settings": map[string]any{"env": map[string]string{claudesettings.EnvBaseURL: testGatewayURL}},
					"user_settings":    map[string]any{"env": userEnv},
				},
			})
		case (strings.HasSuffix(path, "/devtool-gateway/my-keys") || strings.HasSuffix(path, "/devtool-gateway/keys")) && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": f.keys[accountFromPath(path)]})
		case strings.Contains(path, "/devtool-gateway/") && r.Method == http.MethodDelete:
			if f.failRevoke {
				http.Error(w, `{"error":"upstream unavailable"}`, http.StatusBadGateway)
				return
			}
			id := path[strings.LastIndex(path, "/")+1:]
			for account, keys := range f.keys {
				for i := range keys {
					if keys[i].KeyID == id {
						now := time.Now().UTC()
						f.keys[account][i].RevokedAt = &now
						f.revoked = append(f.revoked, id)
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"message":"dev-tool key revoked"}`))
						return
					}
				}
			}
			http.Error(w, `{"error":"dev-tool key not found"}`, http.StatusNotFound)
		default:
			t.Errorf("unexpected request %s %s", r.Method, path)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}
}

func accountFromPath(path string) string {
	rest := strings.TrimPrefix(path, "/api/v1/accounts/")
	return rest[:strings.Index(rest, "/")]
}

type gatewayHarness struct {
	server        *fakeGatewayServer
	settingsPath  string
	confirmAnswer bool
	confirmed     []string
	picked        string
}

func newGatewayHarness(t *testing.T) *gatewayHarness {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	t.Setenv(claudesettings.EnvBaseURL, "")
	claudeDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	t.Chdir(t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("alice"))

	f := &fakeGatewayServer{enabled: map[string]bool{"alice": false}, keys: map[string][]gatewayKeyMeta{}}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	gatewayServerURLOverride = srv.URL
	t.Cleanup(func() { gatewayServerURLOverride = "" })

	h := &gatewayHarness{server: f, settingsPath: filepath.Join(claudeDir, "settings.json")}
	prevSource, prevInteractive, prevConfirm, prevPick := deviceid.Source, interactiveTerminal, gatewayConfirm, gatewayPick
	deviceid.Source = func() (string, error) { return "machine-1", nil }
	interactiveTerminal = func() bool { return false }
	gatewayConfirm = func(title, _ string) (bool, error) {
		h.confirmed = append(h.confirmed, title)
		return h.confirmAnswer, nil
	}
	gatewayPick = func(_, _ string, options []string) (string, error) {
		if h.picked == "" {
			return options[0], nil
		}
		return h.picked, nil
	}
	t.Cleanup(func() {
		deviceid.Source, interactiveTerminal, gatewayConfirm, gatewayPick = prevSource, prevInteractive, prevConfirm, prevPick
	})
	return h
}

func (h *gatewayHarness) writeSettings(t *testing.T, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(h.settingsPath, []byte(content), 0o600))
}

func (h *gatewayHarness) env(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(h.settingsPath)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	env, _ := doc["env"].(map[string]any)
	return env
}

func connectOpts(account string) gatewayConnectOptions {
	return gatewayConnectOptions{account: account, interactive: interactiveTerminal()}
}

func TestGatewayConnect_WritesTheProfileAndRecordsWhatItChanged(t *testing.T) {
	h := newGatewayHarness(t)
	h.writeSettings(t, `{"model":"opus","env":{"ANTHROPIC_CUSTOM_HEADERS":"x-team: platform"}}`)
	buf := &bytes.Buffer{}

	require.NoError(t, connectGateway(context.Background(), buf, connectOpts("alice")))

	env := h.env(t)
	assert.Equal(t, testGatewayURL, env[claudesettings.EnvBaseURL])
	assert.Equal(t, "x-team: platform\nx-bf-direct-key: true\nx-bf-vk: sk-bf-0001keykeykey", env[claudesettings.EnvCustomHeaders],
		"the profile's headers are merged after the user's own")
	require.Len(t, h.server.mints, 1)
	mint := h.server.mints[0]
	assert.Len(t, mint["device_id"], 32, "the device id is a hash")
	assert.NotContains(t, mint["device_id"], "machine-1", "the OS machine id never leaves the machine")
	assert.Equal(t, buildinfo.BinaryName+" "+buildinfo.Version, mint["client_version"])
	hostname, _ := os.Hostname()
	assert.Equal(t, msgGatewayConnecting("alice")+"\n\n"+
		msgGatewayConnectTarget(hostname, runtime.GOOS, runtime.GOARCH, h.settingsPath)+"\n"+
		msgGatewayKeyIssued("sk-bf-0001")+"\n"+
		msgGatewaySettingsUpdated(h.settingsPath)+"\n"+
		msgGatewaySettingWritten(claudesettings.EnvBaseURL, testGatewayURL)+"\n"+
		msgGatewaySettingWritten(claudesettings.EnvCustomHeaders, "x-bf-direct-key, x-bf-vk")+"\n"+
		"\n"+msgGatewayConnected("alice", buildinfo.BinaryName)+"\n", buf.String())

	state, err := loadGatewayState()
	require.NoError(t, err)
	assert.Equal(t, "alice", state.Account)
	assert.Equal(t, "vk-1", state.KeyID)
	require.NotNil(t, state.Change)
	assert.Equal(t, h.settingsPath, state.Change.Path)
}

func federatedUserEnv(string) map[string]string {
	return map[string]string{
		claudesettings.EnvBaseURL: testGatewayURL,
		"ANTHROPIC_AUTH_TOKEN":    "sk-federated-secret",
	}
}

func TestGatewayConnect_NeverPrintsACredentialItWrote(t *testing.T) {
	h := newGatewayHarness(t)
	h.server.userEnv = federatedUserEnv
	buf := &bytes.Buffer{}

	require.NoError(t, connectGateway(context.Background(), buf, connectOpts("alice")))

	assert.Equal(t, "sk-federated-secret", h.env(t)["ANTHROPIC_AUTH_TOKEN"])
	assert.NotContains(t, buf.String(), "sk-federated-secret", "a token in the profile must never reach the terminal")
	assert.Contains(t, buf.String(), msgGatewaySettingWritten("ANTHROPIC_AUTH_TOKEN", msgGatewaySettingHidden())+"\n")
	assert.Contains(t, buf.String(), msgGatewaySettingWritten(claudesettings.EnvBaseURL, testGatewayURL)+"\n")
}

func TestGatewayConnect_MapsTheServersRefusalCodes(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr error
	}{
		{
			name:    "the gateway was turned off",
			body:    `{"error":"the dev-tool gateway is not enabled for this account","code":"DEVTOOL_GATEWAY_DISABLED"}`,
			wantErr: errGatewayNotEnabled("alice"),
		},
		{
			name:    "another setup is running",
			body:    `{"error":"another setup for this device is in progress; retry","code":"DEVTOOL_DEVICE_SETUP_IN_PROGRESS"}`,
			wantErr: errGatewaySetupInProgress(),
		},
		{
			name:    "an uncoded conflict",
			body:    `{"error":"conflict"}`,
			wantErr: errGatewayRequestFailed("issue a key for this device", newAPIError(http.StatusConflict, []byte(`{"error":"conflict"}`))),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newGatewayHarness(t)
			h.server.mintRefusal = tc.body

			err := connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice"))
			require.Error(t, err)
			assert.Equal(t, tc.wantErr.Error(), err.Error())
			_, statErr := os.Stat(h.settingsPath)
			assert.True(t, os.IsNotExist(statErr), "a refused mint must not touch the settings file")
		})
	}
}

func TestGatewayConnect_StopsBeforeMintingWhenTheAccountHasNotTurnedTheGatewayOn(t *testing.T) {
	h := newGatewayHarness(t)
	delete(h.server.enabled, "alice")

	err := connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice"))
	require.Error(t, err)
	assert.Equal(t, errGatewayNotEnabled("alice").Error(), err.Error())
	assert.Empty(t, h.server.mints)
	_, statErr := os.Stat(h.settingsPath)
	assert.True(t, os.IsNotExist(statErr), "a refusal must not create a settings file")
}

func TestGatewayConnect_NeverReplacesAnotherGatewayWithoutConsent(t *testing.T) {
	foreign := `{"env":{"ANTHROPIC_BASE_URL":"https://llm.internal.example.com"}}`
	cases := []struct {
		name        string
		interactive bool
		answer      bool
		replace     bool
		wantErr     error
		wantMinted  bool
	}{
		{name: "script without the flag", wantErr: errGatewayForeignBaseURL("", "https://llm.internal.example.com")},
		{name: "script with --replace-existing", replace: true, wantMinted: true},
		{name: "person declines", interactive: true, answer: false, wantErr: tui.ErrCanceled},
		{name: "person accepts", interactive: true, answer: true, wantMinted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newGatewayHarness(t)
			h.writeSettings(t, foreign)
			h.confirmAnswer = tc.answer
			opts := gatewayConnectOptions{account: "alice", interactive: tc.interactive, replaceExisting: tc.replace}

			err := connectGateway(context.Background(), &bytes.Buffer{}, opts)
			switch {
			case tc.wantErr == tui.ErrCanceled:
				assert.ErrorIs(t, err, tui.ErrCanceled)
			case tc.wantErr != nil:
				require.Error(t, err)
				assert.Equal(t, errGatewayForeignBaseURL(h.settingsPath, "https://llm.internal.example.com").Error(), err.Error())
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantMinted, len(h.server.mints) == 1, "a key is minted only once the conflict is resolved")
			if !tc.wantMinted {
				assert.Equal(t, "https://llm.internal.example.com", h.env(t)[claudesettings.EnvBaseURL], "a refusal leaves the other gateway in place")
			}
		})
	}
}

func TestGatewayConnect_AFailedSettingsWriteSaysToConnectAgain(t *testing.T) {
	h := newGatewayHarness(t)
	require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
	dir := filepath.Dir(h.settingsPath)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("run `%s gateway connect` again", buildinfo.BinaryName),
		"the rerun's mint replaced this device's key, so the developer must connect again")
	assert.Equal(t, []string{"vk-2"}, h.server.revoked, "the key minted for the failed attempt does not stay live")
}

func TestGatewayConnect_RerunReplacesThisDevicesHeaderWithoutAsking(t *testing.T) {
	h := newGatewayHarness(t)
	require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
	require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))

	assert.Empty(t, h.confirmed, "a URL connect wrote itself is not a conflict")
	assert.Equal(t, "x-bf-direct-key: true\nx-bf-vk: sk-bf-0002keykeykey", h.env(t)[claudesettings.EnvCustomHeaders],
		"the new key replaces the old header rather than adding a second")
	assert.Equal(t, h.server.mints[0]["device_id"], h.server.mints[1]["device_id"], "the same machine reports the same device, so the server replaces its key")
}

func TestGatewayConnect_RefusesToSwitchAccountsFromAScript(t *testing.T) {
	h := newGatewayHarness(t)
	require.NoError(t, saveGatewayState(&gatewayState{Account: "gone-org", KeyID: "vk-old", KeyPrefix: "sk-bf-old"}))

	err := connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice"))
	require.Error(t, err)
	assert.Equal(t, errGatewayOtherAccount("gone-org", "alice").Error(), err.Error())
	assert.Empty(t, h.server.mints)

	require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, gatewayConnectOptions{account: "alice", replaceExisting: true}))
	state, err := loadGatewayState()
	require.NoError(t, err)
	assert.Equal(t, "alice", state.Account, "--replace-existing moves the machine to the new account")
}

func TestGatewayConnect_WarnsWhenTheOldAccountsKeyCannotBeRevoked(t *testing.T) {
	h := newGatewayHarness(t)
	require.NoError(t, saveGatewayState(&gatewayState{Account: "gone-org", KeyID: "vk-old", KeyPrefix: "sk-bf-old"}))
	h.server.failRevoke = true
	buf := &bytes.Buffer{}

	require.NoError(t, connectGateway(context.Background(), buf, gatewayConnectOptions{account: "alice", replaceExisting: true}),
		"the new connection works, so a stale key elsewhere must not fail it")
	assert.Contains(t, buf.String(), "! This device's old key on gone-org could not be revoked",
		"nothing records the old key once the machine moves, so the developer has to hear about it")
	assert.Contains(t, buf.String(), fmt.Sprintf("`%s gateway revoke sk-bf-old`", buildinfo.BinaryName))
}

func TestGatewayDisconnect_RestoresTheSettingsFileThenRevokes(t *testing.T) {
	h := newGatewayHarness(t)
	original := `{"model":"opus","env":{"ANTHROPIC_CUSTOM_HEADERS":"x-team: platform"}}`
	h.writeSettings(t, original)
	require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
	buf := &bytes.Buffer{}

	require.NoError(t, disconnectGateway(context.Background(), buf, false))

	var got, want map[string]any
	data, err := os.ReadFile(h.settingsPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &got))
	require.NoError(t, json.Unmarshal([]byte(original), &want))
	assert.Equal(t, want, got, "disconnect must leave the file exactly as it was before connect")
	assert.Equal(t, []string{"vk-1"}, h.server.revoked)
	assert.Equal(t, msgGatewaySettingsRestored(h.settingsPath)+"\n"+msgGatewayThisDeviceKeyRevoked("sk-bf-0001")+"\n", buf.String())

	state, err := loadGatewayState()
	require.NoError(t, err)
	assert.False(t, state.connected())
	assert.True(t, state.disconnectedFrom("alice"), "a deliberate disconnect is remembered, so login won't reconnect")
}

func TestGatewayDisconnect_LeavesAValueChangedAfterConnect(t *testing.T) {
	h := newGatewayHarness(t)
	require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
	h.writeSettings(t, `{"env":{"ANTHROPIC_BASE_URL":"https://elsewhere.example.com"}}`)
	buf := &bytes.Buffer{}

	require.NoError(t, disconnectGateway(context.Background(), buf, false))

	assert.Equal(t, "https://elsewhere.example.com", h.env(t)[claudesettings.EnvBaseURL])
	assert.Contains(t, buf.String(), msgGatewaySettingKept(claudesettings.EnvBaseURL)+"\n")
}

func TestGatewayDisconnect_StillRestoresSettingsWhenTheServerIsDown(t *testing.T) {
	h := newGatewayHarness(t)
	require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
	h.server.failRevoke = true
	buf := &bytes.Buffer{}

	require.NoError(t, disconnectGateway(context.Background(), buf, false), "disconnect is the escape hatch, so a server error must not fail it")

	assert.NotContains(t, h.env(t), claudesettings.EnvBaseURL, "the settings are restored even though the revoke failed")
	assert.Contains(t, buf.String(), fmt.Sprintf("Retry with `%s gateway revoke sk-bf-0001`", buildinfo.BinaryName))
}

func TestGatewayStatus(t *testing.T) {
	now := time.Now().UTC()

	t.Run("not connected", func(t *testing.T) {
		newGatewayHarness(t)
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.Equal(t, msgGatewayNotConnected(buildinfo.BinaryName)+"\n", buf.String())
	})

	t.Run("connected and routed", func(t *testing.T) {
		h := newGatewayHarness(t)
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		used := now.Add(-2 * time.Minute)
		h.server.keys["alice"][0].LastUsedAt = &used
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		out := buf.String()
		assert.Contains(t, out, msgGatewayStatusConnected("alice", h.server.keys["alice"][0].DeviceName, "sk-bf-0001", "2 minutes ago"))
		assert.Contains(t, out, msgGatewayStatusRouting(testGatewayURL, msgGatewaySettingsScope(claudesettings.ScopeUser), nil)+"\n")
		assert.NotContains(t, out, "!", "a healthy connection shows no warnings")
	})

	t.Run("gateway turned off", func(t *testing.T) {
		h := newGatewayHarness(t)
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		delete(h.server.enabled, "alice")
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.Contains(t, buf.String(), msgGatewayStatusCollectionOff("alice", buildinfo.BinaryName)+"\n",
			"a device that still routes after the admin turned the gateway off must say how to stop")
	})

	t.Run("revoked key", func(t *testing.T) {
		h := newGatewayHarness(t)
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		revoked := now.Add(-time.Hour)
		h.server.keys["alice"][0].RevokedAt = &revoked
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.Contains(t, buf.String(), msgGatewayStatusRevoked("1 hour ago", buildinfo.BinaryName))
	})

	t.Run("a project overrides routing", func(t *testing.T) {
		newGatewayHarness(t)
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		require.NoError(t, os.MkdirAll(".claude", 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(".claude", "settings.json"), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://elsewhere.example.com"}}`), 0o600))
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.Contains(t, buf.String(), msgGatewayStatusOverridden("this project's shared settings"),
			"a repo that routes elsewhere bypasses the gateway, and status must say so")
	})

	t.Run("headers removed by hand", func(t *testing.T) {
		h := newGatewayHarness(t)
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		h.writeSettings(t, `{"env":{"ANTHROPIC_BASE_URL":"`+testGatewayURL+`"}}`)
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.Contains(t, buf.String(), msgGatewayStatusSettingsMissing(buildinfo.BinaryName))
	})

	t.Run("a header replaced by hand", func(t *testing.T) {
		h := newGatewayHarness(t)
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		h.writeSettings(t, `{"env":{"ANTHROPIC_BASE_URL":"`+testGatewayURL+`","ANTHROPIC_CUSTOM_HEADERS":"x-bf-direct-key: true\nx-bf-vk: sk-bf-stale"}}`)
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.Contains(t, buf.String(), msgGatewayStatusSettingsMissing(buildinfo.BinaryName), "another device's key in the header is not this device's connection")
	})

	t.Run("a profile without headers", func(t *testing.T) {
		h := newGatewayHarness(t)
		h.server.userEnv = federatedUserEnv
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.NotContains(t, buf.String(), msgGatewayStatusSettingsMissing(buildinfo.BinaryName), "a profile that sets no headers is intact without them")

		h.writeSettings(t, `{"env":{"ANTHROPIC_BASE_URL":"`+testGatewayURL+`"}}`)
		buf.Reset()
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.Contains(t, buf.String(), msgGatewayStatusSettingsMissing(buildinfo.BinaryName), "a removed token is reported like removed headers")
	})

	t.Run("a shell export may override", func(t *testing.T) {
		newGatewayHarness(t)
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		t.Setenv(claudesettings.EnvBaseURL, "https://shell.example.com")
		buf := &bytes.Buffer{}
		require.NoError(t, statusGateway(context.Background(), buf, false, now))
		assert.Contains(t, buf.String(), msgGatewayStatusShellExport(claudesettings.EnvBaseURL, "https://shell.example.com"))
	})
}

func TestGatewayDevices_MarksThisDevice(t *testing.T) {
	h := newGatewayHarness(t)
	require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
	h.server.keys["alice"] = append(h.server.keys["alice"], gatewayKeyMeta{KeyID: "vk-9", KeyPrefix: "sk-bf-9999", DeviceName: "desktop", OS: "linux", Arch: "amd64", CreatedAt: time.Now()})
	buf := &bytes.Buffer{}

	require.NoError(t, listGatewayDevices(context.Background(), buf, false, false, time.Now()))
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[0], "DEVICE")
	assert.Contains(t, buf.String(), "(this device)")
	assert.Equal(t, 1, strings.Count(buf.String(), "(this device)"), "only the key this machine holds is marked")
}

func TestGatewayDevices_SaysSoWhenThereAreNone(t *testing.T) {
	newGatewayHarness(t)
	buf := &bytes.Buffer{}
	require.NoError(t, listGatewayDevices(context.Background(), buf, false, false, time.Now()))
	assert.Equal(t, msgGatewayNoDevices("alice")+"\n", buf.String())
}

func TestGatewayRevoke(t *testing.T) {
	t.Run("by prefix, with the hint when it is this machine", func(t *testing.T) {
		h := newGatewayHarness(t)
		require.NoError(t, connectGateway(context.Background(), &bytes.Buffer{}, connectOpts("alice")))
		buf := &bytes.Buffer{}
		require.NoError(t, revokeGatewayDevice(context.Background(), buf, "sk-bf-0001…", false, false))
		assert.Equal(t, []string{"vk-1"}, h.server.revoked)
		hostname, _ := os.Hostname()
		assert.Equal(t, msgGatewayDeviceKeyRevoked(hostname, "sk-bf-0001")+"\n"+msgGatewayRevokedThisDevice(buildinfo.BinaryName)+"\n", buf.String())
	})
	t.Run("unknown key", func(t *testing.T) {
		newGatewayHarness(t)
		err := revokeGatewayDevice(context.Background(), &bytes.Buffer{}, "sk-bf-nope", false, false)
		require.Error(t, err)
		assert.Equal(t, errGatewayKeyNotFound("sk-bf-nope").Error(), err.Error())
	})
}

func TestMatchDeviceKey(t *testing.T) {
	keys := []gatewayKeyMeta{
		{KeyID: "vk-1", KeyPrefix: "sk-bf-3f9a"},
		{KeyID: "vk-2", KeyPrefix: "sk-bf-3f81"},
		{KeyID: "vk-3", KeyPrefix: "sk-bf-aaaa", RevokedAt: &time.Time{}},
	}
	cases := []struct {
		query, wantID string
		wantErr       error
	}{
		{query: "vk-2", wantID: "vk-2"},
		{query: "sk-bf-3f9a…", wantID: "vk-1"},
		{query: "sk-bf-3f9a...", wantID: "vk-1"},
		{query: "sk-bf-3f", wantErr: errGatewayKeyAmbiguous("sk-bf-3f", 2)},
		{query: "sk-bf-aaaa", wantErr: errGatewayKeyNotFound("sk-bf-aaaa")},
		{query: "vk-3", wantID: "vk-3"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			got, err := matchDeviceKey(keys, tc.query)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantID, got.KeyID)
		})
	}
}

func TestAutoConnectGatewayAfterLogin(t *testing.T) {
	t.Run("an account that asks for it is connected", func(t *testing.T) {
		h := newGatewayHarness(t)
		h.server.enabled["alice"] = true
		buf := &bytes.Buffer{}
		autoConnectGatewayAfterLogin(context.Background(), buf, false)
		assert.Len(t, h.server.mints, 1)
		assert.Contains(t, buf.String(), msgGatewayConnected("alice", buildinfo.BinaryName))
	})
	t.Run("an account that has not opted in is left alone", func(t *testing.T) {
		h := newGatewayHarness(t)
		buf := &bytes.Buffer{}
		autoConnectGatewayAfterLogin(context.Background(), buf, false)
		assert.Empty(t, h.server.mints)
		assert.Empty(t, buf.String(), "login stays quiet when nothing applies")
	})
	t.Run("a deliberate disconnect is respected", func(t *testing.T) {
		h := newGatewayHarness(t)
		h.server.enabled["alice"] = true
		require.NoError(t, saveGatewayState(&gatewayState{Disconnected: []string{"alice"}}))
		autoConnectGatewayAfterLogin(context.Background(), &bytes.Buffer{}, false)
		assert.Empty(t, h.server.mints)
	})
	t.Run("an existing connection is left alone", func(t *testing.T) {
		h := newGatewayHarness(t)
		h.server.enabled["alice"] = true
		require.NoError(t, saveGatewayState(&gatewayState{Account: "gone-org", KeyID: "vk-old"}))
		autoConnectGatewayAfterLogin(context.Background(), &bytes.Buffer{}, false)
		assert.Empty(t, h.server.mints, "login must never move a connected machine to another account")
	})
	t.Run("several candidates without a terminal prints how to choose", func(t *testing.T) {
		h := newGatewayHarness(t)
		h.server.enabled["alice"] = true
		h.server.enabled["acme-corp"] = true
		buf := &bytes.Buffer{}
		autoConnectGatewayAfterLogin(context.Background(), buf, false)
		assert.Empty(t, h.server.mints)
		assert.Contains(t, buf.String(), "gateway connect --account <name>")
	})
	t.Run("several candidates in a terminal asks which", func(t *testing.T) {
		h := newGatewayHarness(t)
		h.server.enabled["alice"] = true
		h.server.enabled["acme-corp"] = true
		h.picked = "alice"
		interactiveTerminal = func() bool { return true }
		autoConnectGatewayAfterLogin(context.Background(), &bytes.Buffer{}, false)
		require.Len(t, h.server.mints, 1)
		state, err := loadGatewayState()
		require.NoError(t, err)
		assert.Equal(t, "alice", state.Account)
	})
	t.Run("an environment without a gateway stays quiet", func(t *testing.T) {
		h := newGatewayHarness(t)
		h.server.unavailable = true
		buf := &bytes.Buffer{}
		autoConnectGatewayAfterLogin(context.Background(), buf, false)
		assert.Empty(t, buf.String())
	})
}

func TestLastUsedLabel(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	cases := []struct {
		t    *time.Time
		want string
	}{
		{nil, "never"},
		{at(10 * time.Second), "just now"},
		{at(time.Minute), "1 minute ago"},
		{at(5 * time.Minute), "5 minutes ago"},
		{at(3 * time.Hour), "3 hours ago"},
		{at(49 * time.Hour), "2 days ago"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, lastUsedLabel(tc.t, now))
	}
}
