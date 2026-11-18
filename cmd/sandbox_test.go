package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shellServer stands in for the control plane's dev session routes: the list
// and the terminal WebSocket.
type shellServer struct {
	mu        sync.Mutex
	sandboxes string
	runtime   string
	listCode  int
	shellCode int
	origin    string
	bearer    string
	path      string
	resizes   []shellResize
	input     bytes.Buffer
}

func newShellServer(t *testing.T, s *shellServer) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/terminal") {
			s.mu.Lock()
			s.path, s.origin, s.bearer = r.URL.Path, r.Header.Get("Origin"), r.Header.Get("Authorization")
			code := s.shellCode
			s.mu.Unlock()
			if code != 0 {
				w.WriteHeader(code)
				return
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close() //nolint:errcheck
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"session_init","session_id":"s"}`))
			_ = conn.WriteMessage(websocket.BinaryMessage, []byte("root@sandbox:/workspace# "))
			for {
				kind, data, err := conn.ReadMessage()
				if err != nil {
					return
				}
				s.mu.Lock()
				if kind == websocket.TextMessage {
					var rz shellResize
					_ = json.Unmarshal(data, &rz)
					s.resizes = append(s.resizes, rz)
				} else {
					s.input.Write(data)
				}
				s.mu.Unlock()
				if kind == websocket.BinaryMessage && bytes.HasSuffix(data, []byte{endOfTransmission}) {
					_ = conn.WriteMessage(websocket.BinaryMessage, []byte("exit\r\n"))
					_ = conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shell exited"), time.Now().Add(time.Second))
					return
				}
				if kind == websocket.BinaryMessage {
					_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte("got: "), data...))
				}
			}
		}
		if strings.HasSuffix(r.URL.Path, "/runtime") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(s.runtime))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/deployments/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"deployment":{"id":"dep-1","name":"my-agent","display_name":"My Agent"}}`))
			return
		}
		code := s.listCode
		if code == 0 {
			code = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		body := s.sandboxes
		if body == "" {
			body = `{"sandboxes":[]}`
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	prev, prevAgent := sandboxServerURLOverride, agentServerURLOverride
	sandboxServerURLOverride, agentServerURLOverride = srv.URL, srv.URL
	t.Cleanup(func() { sandboxServerURLOverride, agentServerURLOverride = prev, prevAgent })
	return srv
}

func runSandboxCmd(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetArgs(append([]string{"sandbox"}, args...))
	rootCmd.SetIn(strings.NewReader(stdin))
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() {
		for _, c := range []*cobra.Command{sandboxListCmd, sandboxShellCmd} {
			for _, f := range []string{"agent", "id", "env", "name", "blueprint"} {
				_ = c.Flags().Set(f, "")
				c.Flags().Lookup(f).Changed = false
			}
		}
		rootCmd.SetArgs(nil)
		rootCmd.SetIn(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})
	_, err := rootCmd.ExecuteC()
	return out.String(), err
}

func TestSandboxListShowsTheDevSessionsSandboxes(t *testing.T) {
	loggedIn(t)
	newShellServer(t, &shellServer{sandboxes: `{"sandboxes":[{"name":"conv-1","state":"running"},{"name":"conv-2","state":"suspended"}]}`})

	out, err := runSandboxCmd(t, "", "list", "--agent", "my-agent")

	require.NoError(t, err)
	assert.Contains(t, out, "conv-1")
	assert.Contains(t, out, "running")
	assert.Contains(t, out, "conv-2")
}

func TestSandboxListWithNoSandboxesSaysSo(t *testing.T) {
	loggedIn(t)
	newShellServer(t, &shellServer{})

	out, err := runSandboxCmd(t, "", "list", "--agent", "my-agent")

	require.NoError(t, err)
	assert.Equal(t, msgSandboxNoneYet("my-agent")+"\n", out)
}

func TestSandboxCommandsWithNoDevSessionSayToRunDev(t *testing.T) {
	loggedIn(t)
	newShellServer(t, &shellServer{listCode: http.StatusNotFound})

	for _, sub := range []string{"list", "shell"} {
		_, err := runSandboxCmd(t, "", sub, "--agent", "my-agent")
		require.Error(t, err, sub)
		assert.Equal(t, errSandboxNoDevSession("my-agent").Error(), err.Error(), sub)
	}
}

func TestSandboxShellRelaysKeystrokesAndOutput(t *testing.T) {
	loggedIn(t)
	s := &shellServer{}
	srv := newShellServer(t, s)

	out, err := runSandboxCmd(t, "ls\n", "shell", "conv-1", "--agent", "my-agent")

	require.NoError(t, err)
	assert.Contains(t, out, "root@sandbox:/workspace# ", "shell output must reach the terminal")
	assert.Contains(t, out, "got: ls\n", "keystrokes must reach the shell and its reply must come back")
	assert.NotContains(t, out, "session_init", "a text control frame from the shell is not output")
	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Equal(t, "/api/v1/accounts/alice/dev-sessions/my-agent/sandboxes/conv-1/terminal", s.path)
	assert.Equal(t, srv.URL, s.origin, "the server refuses a terminal with no Origin, so the CLI sends its own URL")
	assert.True(t, strings.HasPrefix(s.bearer, "Bearer "))
	assert.Equal(t, "ls\n\x04", s.input.String(), "end of input must reach the shell as Ctrl-D, so it exits on its own")
	require.NotEmpty(t, s.resizes, "the shell starts at 0x0, so the CLI must size it on open")
	assert.Equal(t, shellResize{Type: "resize", Cols: 80, Rows: 24}, s.resizes[0])
}

func TestSandboxShellPicksTheOnlySandbox(t *testing.T) {
	loggedIn(t)
	s := &shellServer{sandboxes: `{"sandboxes":[{"name":"only-one","state":"running"}]}`}
	newShellServer(t, s)

	_, err := runSandboxCmd(t, "", "shell", "--agent", "my-agent")

	require.NoError(t, err)
	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Equal(t, "/api/v1/accounts/alice/dev-sessions/my-agent/sandboxes/only-one/terminal", s.path)
}

func TestSandboxShellWithoutANameNeedsExactlyOneSandbox(t *testing.T) {
	loggedIn(t)
	for _, tc := range []struct {
		name      string
		sandboxes string
		want      error
	}{
		{"none", `{"sandboxes":[]}`, errSandboxShellNoSandbox("my-agent")},
		{"several", `{"sandboxes":[{"name":"a","state":"running"},{"name":"b","state":"running"}]}`, errSandboxShellWhich([]string{"a", "b"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newShellServer(t, &shellServer{sandboxes: tc.sandboxes})
			_, err := runSandboxCmd(t, "", "shell", "--agent", "my-agent")
			require.Error(t, err)
			assert.Equal(t, tc.want.Error(), err.Error())
		})
	}
}

func TestSandboxShellForAnUnknownSandboxSaysHowToCheck(t *testing.T) {
	loggedIn(t)
	newShellServer(t, &shellServer{shellCode: http.StatusNotFound})

	_, err := runSandboxCmd(t, "", "shell", "nope", "--agent", "my-agent")

	require.Error(t, err)
	assert.Equal(t, errSandboxShellNotFound().Error(), err.Error())
}

const deployedRuntime = `{"runtime":{"sandboxes":{"total":3,"recent":[{"name":"old","state":"stopped"},{"name":"live","state":"running"}]}}}`

func TestSandboxListShowsADeployedAgentsSandboxes(t *testing.T) {
	loggedIn(t)
	newShellServer(t, &shellServer{runtime: deployedRuntime})

	out, err := runSandboxCmd(t, "", "list", "--id", "dep-1")

	require.NoError(t, err)
	assert.Contains(t, out, "live")
	assert.Contains(t, out, "old", "the list shows stopped sandboxes too")
	assert.True(t, strings.HasSuffix(out, msgSandboxListTruncated(2, 3)+"\n"),
		"a deployment's list holds only its most recent sandboxes, so the output must say when more exist")
}

func TestSandboxShellOpensADeployedAgentsSandbox(t *testing.T) {
	loggedIn(t)
	s := &shellServer{runtime: deployedRuntime}
	srv := newShellServer(t, s)

	out, err := runSandboxCmd(t, "pwd\n", "shell", "live", "--id", "dep-1")

	require.NoError(t, err)
	assert.Contains(t, out, "got: pwd\n")
	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Equal(t, "/api/v1/deployments/dep-1/sandboxes/live/terminal", s.path,
		"a deployment target must use the deployment terminal, not the dev session's")
	assert.Equal(t, srv.URL, s.origin)
}

func TestSandboxShellOnADeploymentPicksTheOnlyOpenSandbox(t *testing.T) {
	loggedIn(t)
	s := &shellServer{runtime: deployedRuntime}
	newShellServer(t, s)

	_, err := runSandboxCmd(t, "", "shell", "--id", "dep-1")

	require.NoError(t, err)
	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Equal(t, "/api/v1/deployments/dep-1/sandboxes/live/terminal", s.path, "a stopped sandbox has no shell to open")
}

func TestSandboxShellOnADeploymentExplainsARefusal(t *testing.T) {
	loggedIn(t)
	for _, tc := range []struct {
		name string
		code int
		want error
	}{
		{"no operate access", http.StatusForbidden, errSandboxShellForbidden()},
		{"unknown sandbox", http.StatusNotFound, errDeploymentSandboxShellNotFound()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newShellServer(t, &shellServer{runtime: deployedRuntime, shellCode: tc.code})
			_, err := runSandboxCmd(t, "", "shell", "live", "--id", "dep-1")
			require.Error(t, err)
			assert.Equal(t, tc.want.Error(), err.Error())
		})
	}
}

func TestSandboxCommandsTakeADevSessionOrADeploymentNotBoth(t *testing.T) {
	loggedIn(t)
	newShellServer(t, &shellServer{runtime: deployedRuntime})

	_, err := runSandboxCmd(t, "", "list", "--agent", "my-agent", "--id", "dep-1")

	require.Error(t, err, "--agent names a dev session and --id a deployment, so both at once is ambiguous")
}

func TestSandboxShellRunsPipedInputAndWaitsForTheShellToExit(t *testing.T) {
	loggedIn(t)
	newShellServer(t, &shellServer{})

	out, err := runSandboxCmd(t, "ls\n", "shell", "conv-1", "--agent", "my-agent")

	require.NoError(t, err)
	assert.Contains(t, out, "got: ls\n", "piped commands must run and their output must arrive before the CLI returns")
	assert.True(t, strings.HasSuffix(out, "exit\r\n"),
		"the CLI must return only after the shell exits, not as soon as stdin ends")
}
