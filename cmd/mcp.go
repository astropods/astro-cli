package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/envvar"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Serve the CLI as an MCP server on stdio",
	Long: fmt.Sprintf(`Start a Model Context Protocol server on stdin and stdout.

Each %[1]s command that an agent can run unattended becomes an MCP tool. A tool
call runs that command as you, with your stored login, and returns its output.

Add it to Claude Code:

  claude mcp add astropods -- %[1]s mcp`, buildinfo.BinaryName),
	Args: cobra.NoArgs,
	RunE: runMCP,
}

func init() {
	mcpCmd.Flags().Bool("read-only", false, "Serve only tools that read, never ones that change anything")
	mcpCmd.Flags().String("account", "", "Run every tool in this account instead of the current one")
	rootCmd.AddCommand(mcpCmd)
}

const (
	mcpDirProperty  = "dir"
	mcpJSONProperty = "json"
)

var mcpExecutable = os.Executable

func runMCP(cmd *cobra.Command, _ []string) error {
	readOnly, _ := cmd.Flags().GetBool("read-only")
	account, _ := cmd.Flags().GetString("account")

	if account != "" {
		if err := os.Setenv(envvar.Account, account); err != nil {
			return err
		}
	}
	resolved, accountErr := accountNewStorage().GetCurrentAccount()
	if account != "" && accountErr != nil {
		if _, profileErr := accountNewStorage().GetCurrentProfile(); profileErr == nil {
			return accountErr
		}
	}

	exe, err := mcpExecutable()
	if err != nil {
		return err
	}
	tools, err := buildMCPTools(rootCmd, readOnly)
	if err != nil {
		return err
	}

	server := newMCPServer(tools, &mcpExecRunner{executable: exe}, resolved)
	fmt.Fprintln(cmd.ErrOrStderr(), msgMCPServing(len(tools), resolved)) //nolint:errcheck
	return server.Run(cmd.Context(), &mcp.StdioTransport{})
}

func newMCPServer(tools []mcpTool, runner mcpRunner, account string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "astropods",
		Title:   "Astropods",
		Version: buildinfo.Version,
	}, &mcp.ServerOptions{Instructions: msgMCPInstructions(account)})
	for _, t := range tools {
		server.AddTool(t.tool, mcpToolHandler(t, runner))
	}
	return server
}

type mcpParam struct {
	name       string
	flag       *pflag.Flag
	required   bool
	positional bool
}

type mcpTool struct {
	tool    *mcp.Tool
	path    []string
	params  []mcpParam
	hasJSON bool
	timeout time.Duration
}

func mcpCommandKey(c *cobra.Command) string {
	return strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" ")
}

// mcpLeafCommands returns the visible leaf commands. Of two leaves with the same name and handler, such as the
// top-level build alias and blueprint build, it keeps the one with the longer path.
func mcpLeafCommands(root *cobra.Command) []*cobra.Command {
	var leaves []*cobra.Command
	seen := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || c == root && (sub.Name() == "help" || sub.Name() == "completion") {
				continue
			}
			if sub.HasSubCommands() {
				walk(sub)
				continue
			}
			if key := mcpCommandKey(sub); !seen[key] {
				seen[key] = true
				leaves = append(leaves, sub)
			}
		}
	}
	walk(root)

	canonical := map[string]*cobra.Command{}
	for _, c := range leaves {
		if key, ok := mcpAliasKey(c); ok {
			if prev, found := canonical[key]; !found || len(strings.Fields(mcpCommandKey(c))) > len(strings.Fields(mcpCommandKey(prev))) {
				canonical[key] = c
			}
		}
	}
	out := leaves[:0]
	for _, c := range leaves {
		if key, ok := mcpAliasKey(c); !ok || canonical[key] == c {
			out = append(out, c)
		}
	}
	return out
}

func mcpAliasKey(c *cobra.Command) (string, bool) {
	var handler uintptr
	switch {
	case c.RunE != nil:
		handler = reflect.ValueOf(c.RunE).Pointer()
	case c.Run != nil:
		handler = reflect.ValueOf(c.Run).Pointer()
	default:
		return "", false
	}
	return fmt.Sprintf("%s/%x", c.Name(), handler), true
}

func buildMCPTools(root *cobra.Command, readOnly bool) ([]mcpTool, error) {
	var tools []mcpTool
	for _, c := range mcpLeafCommands(root) {
		policy, ok := mcpPolicies[mcpCommandKey(c)]
		if !ok || policy.class == mcpExclude {
			continue
		}
		if readOnly && policy.class != mcpRead {
			continue
		}
		t, err := buildMCPTool(c, policy)
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].tool.Name < tools[j].tool.Name })
	return tools, nil
}

var mcpSkippedFlags = map[string]bool{"help": true, "verbose": true, "quiet": true, nonInteractiveFlag: true}

func buildMCPTool(c *cobra.Command, policy mcpPolicy) (mcpTool, error) {
	key := mcpCommandKey(c)
	path := strings.Fields(key)
	t := mcpTool{path: path, timeout: policy.timeout}
	if t.timeout == 0 {
		t.timeout = mcpDefaultTimeout
	}

	schema := &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{}}
	excluded := map[string]bool{}
	for _, name := range policy.excludeFlags {
		excluded[name] = true
	}

	addFlag := func(f *pflag.Flag) {
		if f.Hidden || f.Deprecated != "" || mcpSkippedFlags[f.Name] || excluded[f.Name] {
			return
		}
		if _, dup := schema.Properties[f.Name]; dup {
			return
		}
		prop := mcpFlagSchema(f)
		if f.Name == mcpJSONProperty && f.Value.Type() == "bool" {
			t.hasJSON = true
			prop.Default = json.RawMessage("true")
		}
		schema.Properties[f.Name] = prop
		t.params = append(t.params, mcpParam{name: f.Name, flag: f})
	}
	c.NonInheritedFlags().VisitAll(addFlag)
	c.InheritedFlags().VisitAll(addFlag)

	argSpec := policy.args
	if argSpec == "" {
		argSpec = strings.TrimSpace(strings.TrimPrefix(c.Use, c.Name()))
	}
	for _, a := range mcpParseArgs(argSpec) {
		if _, clash := schema.Properties[a.name]; clash || a.name == mcpDirProperty {
			return mcpTool{}, fmt.Errorf("mcp: %s: positional <%s> has the same name as a flag; set args in mcpPolicies", key, a.name)
		}
		schema.Properties[a.name] = &jsonschema.Schema{Type: "string", Description: msgMCPPositionalDescription(a.name)}
		if a.required {
			schema.Required = append(schema.Required, a.name)
		}
		t.params = append(t.params, a)
	}

	schema.Properties[mcpDirProperty] = &jsonschema.Schema{Type: "string", Description: msgMCPDirDescription()}

	description := c.Short
	if c.Long != "" {
		description = c.Long
	}
	readOnly := policy.class == mcpRead
	destructive := policy.class == mcpDestructive
	t.tool = &mcp.Tool{
		Name:        strings.Join(path, "_"),
		Title:       buildinfo.BinaryName + " " + key,
		Description: description,
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    readOnly,
			DestructiveHint: &destructive,
		},
	}
	return t, nil
}

func mcpFlagSchema(f *pflag.Flag) *jsonschema.Schema {
	s := &jsonschema.Schema{Description: f.Usage}
	switch f.Value.Type() {
	case "bool":
		s.Type = "boolean"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		s.Type = "integer"
		if f.DefValue != "0" {
			s.Default = json.RawMessage(f.DefValue)
		}
	case "float32", "float64":
		s.Type = "number"
	case "stringArray", "stringSlice":
		s.Type = "array"
		s.Items = &jsonschema.Schema{Type: "string"}
	default:
		s.Type = "string"
		if f.DefValue != "" {
			if raw, err := json.Marshal(f.DefValue); err == nil {
				s.Default = raw
			}
		}
	}
	return s
}

func mcpParseArgs(spec string) []mcpParam {
	var params []mcpParam
	for _, tok := range strings.Fields(spec) {
		var name string
		required := false
		switch {
		case strings.HasPrefix(tok, "<") && strings.HasSuffix(tok, ">"):
			name, required = tok[1:len(tok)-1], true
		case strings.HasPrefix(tok, "[") && strings.HasSuffix(tok, "]"):
			name = tok[1 : len(tok)-1]
		default:
			return params
		}
		if name == "" || strings.ContainsAny(name, "<>[]| ") {
			return params
		}
		params = append(params, mcpParam{name: name, required: required, positional: true})
	}
	return params
}

func (t mcpTool) argv(input map[string]any) ([]string, string, error) {
	known := map[string]mcpParam{}
	for _, p := range t.params {
		known[p.name] = p
	}
	for name := range input {
		if _, ok := known[name]; !ok && name != mcpDirProperty {
			return nil, "", errMCPUnknownInput(name)
		}
	}

	dir := ""
	if v, ok := input[mcpDirProperty]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, "", errMCPInputType(mcpDirProperty, "a string")
		}
		dir = s
	}

	args := append(append([]string{}, t.path...), "--"+nonInteractiveFlag)
	var positionals []string
	missing := ""
	for _, p := range t.params {
		v, present := input[p.name]
		if p.positional {
			if !present || v == nil {
				if p.required {
					return nil, "", errMCPMissingInput(p.name)
				}
				if missing == "" {
					missing = p.name
				}
				continue
			}
			s, ok := v.(string)
			if !ok {
				return nil, "", errMCPInputType(p.name, "a string")
			}
			if missing != "" {
				return nil, "", errMCPMissingInput(missing)
			}
			positionals = append(positionals, s)
			continue
		}
		if !present || v == nil {
			if p.name == mcpJSONProperty && t.hasJSON {
				args = append(args, "--json")
			}
			continue
		}
		flagArgs, err := mcpFlagArgs(p, v)
		if err != nil {
			return nil, "", err
		}
		args = append(args, flagArgs...)
	}
	if len(positionals) > 0 {
		args = append(append(args, "--"), positionals...)
	}
	return args, dir, nil
}

func mcpFlagArgs(p mcpParam, v any) ([]string, error) {
	flag := "--" + p.name + "="
	switch p.flag.Value.Type() {
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return nil, errMCPInputType(p.name, "a boolean")
		}
		return []string{flag + strconv.FormatBool(b)}, nil
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		n, ok := v.(float64)
		if !ok || n != math.Trunc(n) {
			return nil, errMCPInputType(p.name, "an integer")
		}
		return []string{flag + strconv.FormatInt(int64(n), 10)}, nil
	case "float32", "float64":
		n, ok := v.(float64)
		if !ok {
			return nil, errMCPInputType(p.name, "a number")
		}
		return []string{flag + strconv.FormatFloat(n, 'f', -1, 64)}, nil
	case "stringArray", "stringSlice":
		items, ok := v.([]any)
		if !ok {
			return nil, errMCPInputType(p.name, "an array of strings")
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				return nil, errMCPInputType(p.name, "an array of strings")
			}
			out = append(out, flag+s)
		}
		return out, nil
	default:
		s, ok := v.(string)
		if !ok {
			return nil, errMCPInputType(p.name, "a string")
		}
		return []string{flag + s}, nil
	}
}

type mcpRunResult struct {
	stdout   []byte
	stderr   string
	exitErr  error
	timedOut bool
}

type mcpRunner interface {
	run(ctx context.Context, args []string, dir string, timeout time.Duration, onStderrLine func(string)) mcpRunResult
}

type mcpExecRunner struct {
	executable string
}

func (r *mcpExecRunner) run(ctx context.Context, args []string, dir string, timeout time.Duration, onStderrLine func(string)) mcpRunResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	child := exec.CommandContext(ctx, r.executable, args...) //nolint:gosec
	child.Dir = dir
	child.Env = append(os.Environ(),
		envvar.Via+"=mcp",
		"NO_COLOR=1",
	)
	child.WaitDelay = 5 * time.Second

	var stdout bytes.Buffer
	child.Stdout = &stdout
	stderrPipe, err := child.StderrPipe()
	if err != nil {
		return mcpRunResult{exitErr: err}
	}
	if err := child.Start(); err != nil {
		return mcpRunResult{exitErr: err}
	}

	var stderr strings.Builder
	scanner := bufio.NewScanner(stderrPipe)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		stderr.WriteString(line)
		stderr.WriteString("\n")
		if onStderrLine != nil && strings.TrimSpace(line) != "" {
			onStderrLine(line)
		}
	}
	if scanner.Err() != nil {
		_, _ = io.Copy(io.Discard, stderrPipe)
	}
	waitErr := child.Wait()

	return mcpRunResult{
		stdout:   stdout.Bytes(),
		stderr:   stderr.String(),
		exitErr:  waitErr,
		timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded),
	}
}

func mcpToolHandler(t mcpTool, runner mcpRunner) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		input := map[string]any{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
				return mcpErrorResult(errMCPInputNotObject().Error()), nil
			}
		}
		args, dir, err := t.argv(input)
		if err != nil {
			return mcpErrorResult(err.Error()), nil
		}

		var onLine func(string)
		if token := req.Params.GetProgressToken(); token != nil && req.Session != nil {
			var n float64
			onLine = func(line string) {
				n++
				_ = req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
					ProgressToken: token,
					Message:       ansi.Strip(line),
					Progress:      n,
				})
			}
		}

		res := runner.run(ctx, args, dir, t.timeout, onLine)
		return mcpResult(t, args, res), nil
	}
}

func mcpResult(t mcpTool, args []string, res mcpRunResult) *mcp.CallToolResult {
	stdout := strings.TrimSpace(ansi.Strip(string(res.stdout)))
	stderr := strings.TrimSpace(ansi.Strip(res.stderr))
	switch {
	case res.timedOut:
		return mcpErrorResult(msgMCPTimeout(t.tool.Name, t.timeout))
	case res.exitErr != nil:
		text := stderr
		if text == "" {
			text = stdout
		}
		if text == "" {
			text = res.exitErr.Error()
		}
		return mcpErrorResult(text)
	}

	text := stdout
	if text == "" {
		text = stderr
	}
	if text == "" {
		text = msgMCPNoOutput()
	}
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
	if mcpArgsHaveJSON(args) {
		var obj map[string]any
		if json.Unmarshal(res.stdout, &obj) == nil && obj != nil {
			result.StructuredContent = obj
		}
	}
	return result
}

func mcpArgsHaveJSON(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

func mcpErrorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
