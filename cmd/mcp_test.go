package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/envvar"
)

func mcpTestWidgetCmd(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "ast"}
	root.PersistentFlags().Bool("verbose", false, "")
	group := &cobra.Command{Use: "widget"}
	group.PersistentFlags().String("env", "", "Environment")
	get := &cobra.Command{Use: "get <name> [rev]", Short: "Get a widget", Long: "Get one widget by name.", RunE: func(*cobra.Command, []string) error { return nil }}
	get.Flags().Bool("json", false, "Output as JSON")
	get.Flags().Bool("card", false, "Show the card")
	get.Flags().Int("limit", 50, "Max rows")
	get.Flags().Float64("ratio", 0, "Ratio")
	get.Flags().String("template", "mastra", "Template")
	get.Flags().StringArray("var", nil, "Variable")
	get.Flags().StringSlice("severity", nil, "Severity")
	get.Flags().Bool("tail", false, "Stream")
	get.Flags().String("secret-thing", "", "Hidden")
	require.NoError(t, get.Flags().MarkHidden("secret-thing"))
	group.AddCommand(get)
	root.AddCommand(group)
	return get
}

func mcpTestWidgetTool(t *testing.T) mcpTool {
	t.Helper()
	tool, err := buildMCPTool(mcpTestWidgetCmd(t), mcpPolicy{class: mcpRead, excludeFlags: []string{"tail"}})
	require.NoError(t, err)
	return tool
}

func mcpSchema(t *testing.T, tool mcpTool) *jsonschema.Schema {
	t.Helper()
	s, ok := tool.tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok, "input schema is a *jsonschema.Schema")
	return s
}

func TestMCPPoliciesCoverEveryLeafCommand(t *testing.T) {
	leaves := map[string]bool{}
	for _, c := range mcpLeafCommands(rootCmd) {
		key := mcpCommandKey(c)
		leaves[key] = true
		_, ok := mcpPolicies[key]
		assert.True(t, ok, "leaf command %q has no entry in mcpPolicies", key)
	}
	for key := range mcpPolicies {
		assert.True(t, leaves[key], "mcpPolicies entry %q names no leaf command", key)
	}
}

func TestMCPEveryServedCommandBuildsATool(t *testing.T) {
	tools, err := buildMCPTools(rootCmd, false)
	require.NoError(t, err, "a served command's positional collides with a flag; set args in mcpPolicies")

	var want, got []string
	for key, p := range mcpPolicies {
		if p.class != mcpExclude {
			want = append(want, strings.ReplaceAll(key, " ", "_"))
		}
	}
	for _, tool := range tools {
		got = append(got, tool.tool.Name)
	}
	assert.ElementsMatch(t, want, got, "one tool per served policy, with no duplicates")
}

func TestMCPReadOnlyRegistersOnlyReadTools(t *testing.T) {
	all, err := buildMCPTools(rootCmd, false)
	require.NoError(t, err)
	readOnly, err := buildMCPTools(rootCmd, true)
	require.NoError(t, err)

	var want, got []string
	for key, p := range mcpPolicies {
		if p.class == mcpRead {
			want = append(want, strings.ReplaceAll(key, " ", "_"))
		}
	}
	for _, tool := range readOnly {
		got = append(got, tool.tool.Name)
	}
	require.ElementsMatch(t, want, got)
	assert.Less(t, len(readOnly), len(all), "the full set also serves write tools")
	for _, tool := range readOnly {
		assert.True(t, tool.tool.Annotations.ReadOnlyHint, "%s is served under --read-only but is not read-only", tool.tool.Name)
	}
}

func TestMCPToolAnnotationsFollowPolicyClass(t *testing.T) {
	tools, err := buildMCPTools(rootCmd, false)
	require.NoError(t, err)
	byName := map[string]mcpTool{}
	for _, tool := range tools {
		byName[tool.tool.Name] = tool
	}

	tests := []struct {
		name        string
		readOnly    bool
		destructive bool
	}{
		{"agent_list", true, false},
		{"agent_redeploy", false, false},
		{"agent_delete", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := byName[tt.name]
			require.True(t, ok, "%s is served", tt.name)
			assert.Equal(t, tt.readOnly, tool.tool.Annotations.ReadOnlyHint)
			require.NotNil(t, tool.tool.Annotations.DestructiveHint)
			assert.Equal(t, tt.destructive, *tool.tool.Annotations.DestructiveHint)
		})
	}

	for _, name := range []string{"secrets_create", "secrets_update", "login", "account_switch", "knowledge_credentials"} {
		_, ok := byName[name]
		assert.False(t, ok, "%s must not be served", name)
	}
	_, hasSecretFlag := mcpSchema(t, byName["blueprint_deploy"]).Properties["secret"]
	assert.False(t, hasSecretFlag, "blueprint_deploy must not take a secret value")
}

func TestMCPToolSchemaFromFlags(t *testing.T) {
	tool := mcpTestWidgetTool(t)
	s := mcpSchema(t, tool)

	assert.Equal(t, "widget_get", tool.tool.Name)
	assert.Equal(t, buildinfo.BinaryName+" widget get", tool.tool.Title)
	assert.Equal(t, "Get one widget by name.", tool.tool.Description)
	assert.Equal(t, "object", s.Type)
	assert.Equal(t, []string{"name"}, s.Required)
	assert.True(t, tool.hasJSON)
	assert.Equal(t, mcpDefaultTimeout, tool.timeout)

	tests := []struct {
		prop     string
		typ      string
		def      string
		itemType string
	}{
		{"json", "boolean", "true", ""},
		{"card", "boolean", "", ""},
		{"limit", "integer", "50", ""},
		{"ratio", "number", "", ""},
		{"template", "string", `"mastra"`, ""},
		{"var", "array", "", "string"},
		{"severity", "array", "", "string"},
		{"env", "string", "", ""},
		{"name", "string", "", ""},
		{"rev", "string", "", ""},
		{mcpDirProperty, "string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.prop, func(t *testing.T) {
			p, ok := s.Properties[tt.prop]
			require.True(t, ok, "property %q is in the schema", tt.prop)
			assert.Equal(t, tt.typ, p.Type)
			assert.Equal(t, tt.def, string(p.Default))
			if tt.itemType != "" {
				require.NotNil(t, p.Items)
				assert.Equal(t, tt.itemType, p.Items.Type)
			}
		})
	}
	assert.Equal(t, msgMCPPositionalDescription("name"), s.Properties["name"].Description)

	for _, absent := range []string{"tail", "secret-thing", "verbose", "help"} {
		_, ok := s.Properties[absent]
		assert.False(t, ok, "property %q must not be in the schema", absent)
	}
}

func TestMCPToolArgsOverrideRenamesPositional(t *testing.T) {
	c := &cobra.Command{Use: "deploy <name>"}
	c.Flags().String("name", "", "Display name")
	(&cobra.Command{Use: "ast"}).AddCommand(c)

	_, err := buildMCPTool(c, mcpPolicy{class: mcpWrite})
	require.Error(t, err, "a positional that shares a flag's name is rejected")

	tool, err := buildMCPTool(c, mcpPolicy{class: mcpWrite, args: "<blueprint>", timeout: mcpLongTimeout})
	require.NoError(t, err)
	s := mcpSchema(t, tool)
	assert.Equal(t, []string{"blueprint"}, s.Required)
	assert.Equal(t, mcpLongTimeout, tool.timeout)
}

func TestMCPParseArgs(t *testing.T) {
	tests := []struct {
		spec string
		want []string
	}{
		{"", nil},
		{"<name>", []string{"name!"}},
		{"<name> [build-id]", []string{"name!", "build-id"}},
		{"[model | --features [feature]]", nil},
		{"--provider <provider>", nil},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			var got []string
			for _, p := range mcpParseArgs(tt.spec) {
				name := p.name
				if p.required {
					name += "!"
				}
				got = append(got, name)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMCPArgv(t *testing.T) {
	tool := mcpTestWidgetTool(t)
	tests := []struct {
		name    string
		input   map[string]any
		want    []string
		wantDir string
		wantErr error
	}{
		{
			name:  "positional follows a double dash and json is on by default",
			input: map[string]any{"name": "-w1"},
			want:  []string{"widget", "get", "--json", "--", "-w1"},
		},
		{
			name:  "json false is passed through",
			input: map[string]any{"name": "w1", "json": false},
			want:  []string{"widget", "get", "--json=false", "--", "w1"},
		},
		{
			name: "every flag type",
			input: map[string]any{
				"name": "w1", "rev": "r2", "card": true, "limit": float64(5), "ratio": 0.5,
				"template": "t", "var": []any{"A=1", "B=2"}, "env": "prod", mcpDirProperty: "/tmp/p",
			},
			want: []string{
				"widget", "get", "--card=true", "--json", "--limit=5", "--ratio=0.5", "--template=t",
				"--var=A=1", "--var=B=2", "--env=prod", "--", "w1", "r2",
			},
			wantDir: "/tmp/p",
		},
		{name: "unknown input", input: map[string]any{"name": "w1", "tail": true}, wantErr: errMCPUnknownInput("tail")},
		{name: "missing required positional", input: map[string]any{}, wantErr: errMCPMissingInput("name")},
		{name: "integer with a fraction", input: map[string]any{"name": "w1", "limit": 1.5}, wantErr: errMCPInputType("limit", "an integer")},
		{name: "bool as a string", input: map[string]any{"name": "w1", "card": "yes"}, wantErr: errMCPInputType("card", "a boolean")},
		{name: "array with a number", input: map[string]any{"name": "w1", "var": []any{1}}, wantErr: errMCPInputType("var", "an array of strings")},
		{name: "dir as a number", input: map[string]any{"name": "w1", mcpDirProperty: 1}, wantErr: errMCPInputType(mcpDirProperty, "a string")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, dir, err := tool.argv(tt.input)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}

func TestMCPArgvRejectsOptionalPositionalGap(t *testing.T) {
	c := &cobra.Command{Use: "get [a] [b]"}
	(&cobra.Command{Use: "ast"}).AddCommand(c)
	tool, err := buildMCPTool(c, mcpPolicy{class: mcpRead})
	require.NoError(t, err)

	_, _, err = tool.argv(map[string]any{"b": "x"})
	require.Error(t, err)
	assert.Equal(t, errMCPMissingInput("a").Error(), err.Error())
}

func mcpText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "content is text")
	return text.Text
}

func TestMCPResult(t *testing.T) {
	tool := mcpTestWidgetTool(t)
	jsonArgs := []string{"widget", "get", "--json", "--", "w1"}
	plainArgs := []string{"widget", "get", "--json=false", "--", "w1"}

	tests := []struct {
		name           string
		args           []string
		res            mcpRunResult
		wantText       string
		wantError      bool
		wantStructured map[string]any
	}{
		{
			name:           "json object becomes structured content",
			args:           jsonArgs,
			res:            mcpRunResult{stdout: []byte(`{"name":"w1"}`)},
			wantText:       `{"name":"w1"}`,
			wantStructured: map[string]any{"name": "w1"},
		},
		{name: "json array stays text", args: jsonArgs, res: mcpRunResult{stdout: []byte(`[1,2]`)}, wantText: `[1,2]`},
		{name: "json object without --json stays text", args: plainArgs, res: mcpRunResult{stdout: []byte(`{"a":1}`)}, wantText: `{"a":1}`},
		{name: "ansi codes are stripped", args: plainArgs, res: mcpRunResult{stdout: []byte("\033[32m✓\033[0m valid\n")}, wantText: "✓ valid"},
		{name: "empty stdout falls back to stderr", args: plainArgs, res: mcpRunResult{stderr: "Paused.\n"}, wantText: "Paused."},
		{name: "no output at all", args: plainArgs, res: mcpRunResult{}, wantText: msgMCPNoOutput()},
		{
			name:      "failure returns stderr",
			args:      jsonArgs,
			res:       mcpRunResult{stdout: []byte("partial"), stderr: "\033[31mnot logged in\033[0m\n", exitErr: errors.New("exit status 1")},
			wantText:  "not logged in",
			wantError: true,
		},
		{
			name:      "failure without stderr returns stdout",
			args:      jsonArgs,
			res:       mcpRunResult{stdout: []byte("oops"), exitErr: errors.New("exit status 1")},
			wantText:  "oops",
			wantError: true,
		},
		{
			name:      "failure without output returns the exit error",
			args:      jsonArgs,
			res:       mcpRunResult{exitErr: errors.New("exit status 2")},
			wantText:  "exit status 2",
			wantError: true,
		},
		{
			name:      "timeout",
			args:      jsonArgs,
			res:       mcpRunResult{exitErr: errors.New("signal: killed"), timedOut: true},
			wantText:  msgMCPTimeout("widget_get", mcpDefaultTimeout),
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mcpResult(tool, tt.args, tt.res)
			assert.Equal(t, tt.wantError, got.IsError)
			assert.Equal(t, tt.wantText, mcpText(t, got))
			if tt.wantStructured == nil {
				assert.Nil(t, got.StructuredContent)
			} else {
				assert.Equal(t, tt.wantStructured, got.StructuredContent)
			}
		})
	}
}

type mcpFakeRunner struct {
	mu      sync.Mutex
	args    []string
	dir     string
	timeout time.Duration
	lines   []string
	result  mcpRunResult
}

func (f *mcpFakeRunner) run(_ context.Context, args []string, dir string, timeout time.Duration, onStderrLine func(string)) mcpRunResult {
	f.mu.Lock()
	f.args, f.dir, f.timeout = args, dir, timeout
	f.mu.Unlock()
	if onStderrLine != nil {
		for _, l := range f.lines {
			onStderrLine(l)
		}
	}
	return f.result
}

func TestMCPServerRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runner := &mcpFakeRunner{
		lines:  []string{"\033[1mbuilding\033[0m", "pushed"},
		result: mcpRunResult{stdout: []byte(`{"name":"w1","rev":"r2"}`)},
	}
	server := newMCPServer([]mcpTool{mcpTestWidgetTool(t)}, runner, "acme-corp")

	progress := make(chan string, 4)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			progress <- req.Params.Message
		},
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close() //nolint:errcheck
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close() //nolint:errcheck

	assert.Equal(t, msgMCPInstructions("acme-corp"), session.InitializeResult().Instructions)
	assert.Equal(t, "astropods", session.InitializeResult().ServerInfo.Name)

	list, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, 1)
	assert.Equal(t, "widget_get", list.Tools[0].Name)

	params := &mcp.CallToolParams{Name: "widget_get", Arguments: map[string]any{"name": "w1", "rev": "r2", mcpDirProperty: "/work"}}
	params.SetProgressToken("tok")
	res, err := session.CallTool(ctx, params)
	require.NoError(t, err)

	assert.False(t, res.IsError, mcpText(t, res))
	assert.Equal(t, map[string]any{"name": "w1", "rev": "r2"}, res.StructuredContent)
	assert.Equal(t, []string{"widget", "get", "--json", "--", "w1", "r2"}, runner.args)
	assert.Equal(t, "/work", runner.dir)
	assert.Equal(t, mcpDefaultTimeout, runner.timeout)

	var got []string
	for len(got) < 2 {
		select {
		case m := <-progress:
			got = append(got, m)
		case <-ctx.Done():
			require.FailNow(t, "progress notifications did not arrive", "got %v", got)
		}
	}
	assert.Equal(t, []string{"building", "pushed"}, got, "each stderr line is a progress message, with ANSI stripped")

	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "widget_get", Arguments: map[string]any{"name": "w1", "nope": 1}})
	require.NoError(t, err)
	assert.True(t, bad.IsError)
	assert.Equal(t, errMCPUnknownInput("nope").Error(), mcpText(t, bad))
}

func TestMCPExecRunner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	runner := &mcpExecRunner{executable: "/bin/sh"}
	ctx := context.Background()

	t.Run("child gets the mcp environment, no stdin, and the dir", func(t *testing.T) {
		dir := t.TempDir()
		var lines []string
		res := runner.run(ctx, []string{"-c", `printf '%s %s %s\n' "$ASTRO_NONINTERACTIVE" "$ASTRO_VIA" "$NO_COLOR"; pwd; cat; echo one >&2; echo >&2; echo two >&2`}, dir, time.Minute, func(l string) {
			lines = append(lines, l)
		})
		require.NoError(t, res.exitErr)
		resolved, err := filepath.EvalSymlinks(dir)
		require.NoError(t, err)
		assert.Equal(t, "1 mcp 1\n"+resolved+"\n", string(res.stdout))
		assert.Equal(t, "one\n\ntwo\n", res.stderr)
		assert.Equal(t, []string{"one", "two"}, lines, "blank stderr lines are not sent as progress")
		assert.False(t, res.timedOut)
	})

	t.Run("non-zero exit", func(t *testing.T) {
		res := runner.run(ctx, []string{"-c", "echo bad >&2; exit 3"}, "", time.Minute, nil)
		require.Error(t, res.exitErr)
		assert.Equal(t, "bad\n", res.stderr)
		assert.False(t, res.timedOut)
	})

	t.Run("timeout kills the child", func(t *testing.T) {
		start := time.Now()
		res := runner.run(ctx, []string{"-c", "sleep 30"}, "", 100*time.Millisecond, nil)
		assert.True(t, res.timedOut)
		assert.Less(t, time.Since(start), 10*time.Second)
	})

	t.Run("caller cancel kills the child", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		time.AfterFunc(100*time.Millisecond, cancel)
		res := runner.run(cctx, []string{"-c", "sleep 30"}, "", time.Minute, nil)
		require.Error(t, res.exitErr)
		assert.False(t, res.timedOut, "a cancel is not reported as a timeout")
	})
}

func TestRunFormFailsWhenNonInteractive(t *testing.T) {
	t.Setenv(envvar.NonInteractive, "1")
	var ok bool
	err := runForm(huh.NewForm(huh.NewGroup(huh.NewConfirm().Title("Sure?").Value(&ok))))
	require.Error(t, err)
	assert.Equal(t, errPromptNonInteractive().Error(), err.Error())
	assert.False(t, interactiveTerminal(), "the cluster picker treats the shell as non-interactive")
}

func msgTestUnknownAccount(name string) string {
	return fmt.Sprintf("account %q is not one of your accounts; run '%s account list' to see them", name, buildinfo.BinaryName)
}

func TestGetCurrentAccountHonorsAccountEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    string
		wantErr string
	}{
		{name: "unset uses the stored account", env: "", want: "alice"},
		{name: "matches case-insensitively and returns the stored name", env: "ACME-CORP", want: "acme-corp"},
		{name: "unknown account", env: "nope", wantErr: msgTestUnknownAccount("nope")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			writeAccountTestCredentials(t, accountTestCreds("alice"))
			t.Setenv(envvar.Account, tt.env)

			got, err := accountNewStorage().GetCurrentAccount()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			t.Setenv(envvar.Account, "")
			stored, err := accountNewStorage().GetCurrentAccount()
			require.NoError(t, err)
			assert.Equal(t, "alice", stored, "the override never changes the stored account")
		})
	}
}

func TestMCPCommandRejectsUnknownAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("alice"))
	t.Setenv(envvar.Account, "")
	require.NoError(t, mcpCmd.Flags().Set("account", "nope"))
	t.Cleanup(func() { _ = mcpCmd.Flags().Set("account", "") })

	err := runMCP(mcpCmd, nil)
	require.Error(t, err)
	assert.Equal(t, msgTestUnknownAccount("nope"), err.Error())
	assert.Equal(t, "nope", os.Getenv(envvar.Account), "--account is exported for children before validation")
}

func TestMCPToolSchemaMarshalsAsJSONSchema(t *testing.T) {
	raw, err := json.Marshal(mcpSchema(t, mcpTestWidgetTool(t)).Properties["var"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"array","items":{"type":"string"},"description":"Variable"}`, string(raw))
}
