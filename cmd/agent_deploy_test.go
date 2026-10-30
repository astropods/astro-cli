package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// setupBlueprintDeployTest starts a test server and redirects blueprint API calls to it.
func setupBlueprintDeployTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	blueprintServerURLOverride = srv.URL
	t.Cleanup(func() { blueprintServerURLOverride = "" })
}

func TestParseDeployVars(t *testing.T) {
	cases := []struct {
		name    string
		flags   []string
		want    map[string]deployVarInput
		wantErr string
	}{
		{
			name:  "inline value",
			flags: []string{"FOO=bar"},
			want:  map[string]deployVarInput{"FOO": {Value: "bar"}},
		},
		{
			name:  "ref",
			flags: []string{"FOO=@my-secret"},
			want:  map[string]deployVarInput{"FOO": {Ref: "my-secret"}},
		},
		{
			name:  "escaped at becomes literal value",
			flags: []string{`FOO=\@not-a-ref`},
			want:  map[string]deployVarInput{"FOO": {Value: "@not-a-ref"}},
		},
		{
			name:  "empty value allowed",
			flags: []string{"FOO="},
			want:  map[string]deployVarInput{"FOO": {Value: ""}},
		},
		{
			name:  "value with equals sign",
			flags: []string{"FOO=a=b"},
			want:  map[string]deployVarInput{"FOO": {Value: "a=b"}},
		},
		{
			name:  "self-reference @ uses key as secret name",
			flags: []string{"FOO=@"},
			want:  map[string]deployVarInput{"FOO": {Ref: "FOO"}},
		},
		{
			name:  "escaped @ becomes literal @",
			flags: []string{`FOO=\@`},
			want:  map[string]deployVarInput{"FOO": {Value: "@"}},
		},
		{
			name:  "multiple vars",
			flags: []string{"A=1", "B=@sec", `C=\@lit`, "D=@"},
			want: map[string]deployVarInput{
				"A": {Value: "1"},
				"B": {Ref: "sec"},
				"C": {Value: "@lit"},
				"D": {Ref: "D"},
			},
		},
		{
			name:    "missing equals",
			flags:   []string{"BADVAR"},
			wantErr: "invalid --var",
		},
		{
			name:    "empty string",
			flags:   []string{""},
			wantErr: "invalid --var",
		},
		{
			name:    "empty key",
			flags:   []string{"=value"},
			wantErr: "invalid --var",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDeployVars(tc.flags)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPatchTemplateTarget(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		fields map[string]string
		want   map[string]any
	}{
		{
			name:   "sets display_name and keeps existing target fields",
			input:  `{"spec":"deployment/v1","target":{"runtime":"kubernetes"}}`,
			fields: map[string]string{"display_name": "my deployment"},
			want:   map[string]any{"runtime": "kubernetes", "display_name": "my deployment"},
		},
		{
			name:   "creates target if absent",
			input:  `{"spec":"deployment/v1"}`,
			fields: map[string]string{"display_name": "new name"},
			want:   map[string]any{"display_name": "new name"},
		},
		{
			name:   "sets environment_id beside display_name",
			input:  `{"spec":"deployment/v1","target":{}}`,
			fields: map[string]string{"display_name": "bot", "environment_id": "env-1"},
			want:   map[string]any{"display_name": "bot", "environment_id": "env-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := patchTemplateTarget(json.RawMessage(tc.input), tc.fields)
			require.NoError(t, err)

			var out map[string]any
			require.NoError(t, json.Unmarshal(result, &out))
			assert.Equal(t, tc.want, out["target"])
		})
	}
}

func TestRunBlueprintDeploy(t *testing.T) {
	validTemplate := json.RawMessage(`{"spec":"deployment/v1","source":{"name":"my-agent"}}`)
	validTmplResp := map[string]any{
		"template":   json.RawMessage(validTemplate),
		"validation": map[string]any{"valid": true},
	}
	deployResp := map[string]any{
		"status":        "pending",
		"deployment_id": "dep-abc-123",
		"name":          "my-agent",
		"build_id":      "abc12345",
	}

	cases := []struct {
		name       string
		tmplStatus int
		tmplResp   any
		deplStatus int
		deplResp   any
		dryRun     bool
		jsonOut    bool
		wantErr    string
		wantOut    string
		wantNoDep  bool // deploy POST should not be called
	}{
		{
			name:       "happy path shows deployment id",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusAccepted,
			deplResp:   deployResp,
			wantOut:    "dep-abc-123",
		},
		{
			name:       "happy path shows deployed checkmark",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusAccepted,
			deplResp:   deployResp,
			wantOut:    "✓ deployed",
		},
		{
			name:       "json output",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusAccepted,
			deplResp:   deployResp,
			jsonOut:    true,
			wantOut:    `"deployment_id"`,
		},
		{
			name:       "dry run does not call deploy",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			dryRun:     true,
			wantOut:    "template valid",
			wantNoDep:  true,
		},
		{
			name:       "validation failure prints errors",
			tmplStatus: http.StatusOK,
			tmplResp: map[string]any{
				"template": validTemplate,
				"validation": map[string]any{
					"valid": false,
					"errors": []any{
						map[string]any{"field": "variables.SLACK_BOT_TOKEN", "message": "required for slack adapter"},
						map[string]any{"field": "variables.SLACK_APP_TOKEN", "message": "required for slack adapter"},
					},
				},
			},
			wantErr:   "deployment validation failed",
			wantOut:   "variable SLACK_BOT_TOKEN",
			wantNoDep: true,
		},
		{
			name:       "blueprint not found",
			tmplStatus: http.StatusNotFound,
			tmplResp:   map[string]any{"error": "not found"},
			wantErr:    `blueprint "my-agent" not found`,
			wantNoDep:  true,
		},
		{
			name:       "template server error",
			tmplStatus: http.StatusInternalServerError,
			tmplResp:   map[string]any{"error": "internal error"},
			wantErr:    "status 500",
			wantNoDep:  true,
		},
		{
			name:       "deploy endpoint 404 surfaces the server reason",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusNotFound,
			deplResp:   map[string]any{"error": "not found"},
			wantErr:    `could not deploy`,
		},
		{
			name:       "deploy endpoint error",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusInternalServerError,
			deplResp:   map[string]any{"error": "k8s down"},
			wantErr:    "status 500",
		},
		{
			name:       "deploy name conflict 409 returns actionable error",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusConflict,
			deplResp:   map[string]any{"error": "display_name already in use by another active deployment"},
			wantErr:    errDeployNameConflict("my-agent").Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deployCalled := false

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/deployment-template") {
					jsonHandler(tc.tmplStatus, tc.tmplResp)(w, r)
				} else if r.URL.Path == "/api/v1/deploy" {
					deployCalled = true
					jsonHandler(tc.deplStatus, tc.deplResp)(w, r)
				} else {
					http.NotFound(w, r)
				}
			})
			setupBlueprintDeployTest(t, handler)

			if tc.dryRun {
				require.NoError(t, blueprintDeployCmd.Flags().Set("dry-run", "true"))
				t.Cleanup(func() { blueprintDeployCmd.Flags().Set("dry-run", "false") }) //nolint:errcheck
			}
			if tc.jsonOut {
				require.NoError(t, blueprintDeployCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { blueprintDeployCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			blueprintDeployCmd.SetOut(buf)
			blueprintDeployCmd.SetContext(context.Background())

			err := runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"})

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantOut != "" {
				assert.Contains(t, stripANSI(buf.String()), tc.wantOut)
			}
			if tc.wantNoDep {
				assert.False(t, deployCalled, "deploy POST should not have been called")
			}
		})
	}
}

func TestRunBlueprintDeployValidationErrors(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deployment-template") {
			jsonHandler(http.StatusOK, map[string]any{
				"template": json.RawMessage(`{}`),
				"validation": map[string]any{
					"valid": false,
					"errors": []any{
						map[string]any{"field": "variables.WEATHER_API_KEY", "message": "required variable is empty"},
					},
				},
			})(w, r)
		}
	})
	setupBlueprintDeployTest(t, handler)

	buf := &bytes.Buffer{}
	blueprintDeployCmd.SetOut(buf)
	blueprintDeployCmd.SetContext(context.Background())

	err := runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"})
	require.Error(t, err)
	out := stripANSI(buf.String())
	assert.Contains(t, out, "variable WEATHER_API_KEY")
	assert.NotContains(t, out, "variables.WEATHER_API_KEY")
	assert.Contains(t, out, "required variable is empty")
}

func TestBuildDeployInterfaces(t *testing.T) {
	oidcAuth := &deployInterfacesAuth{Web: &deployWebAuth{Type: "oidc"}}

	cases := []struct {
		name      string
		adapters  []string
		wantIface *deployTemplateInterfaces
		wantErr   string
	}{
		{
			name:     "default (no adapters) → web with oidc auth",
			adapters: nil,
			wantIface: &deployTemplateInterfaces{
				Adapters: []string{"web"},
				Auth:     oidcAuth,
			},
		},
		{
			name:     "explicit web → oidc auth",
			adapters: []string{"web"},
			wantIface: &deployTemplateInterfaces{
				Adapters: []string{"web"},
				Auth:     oidcAuth,
			},
		},
		{
			name:     "insecure-web → web adapter, no auth",
			adapters: []string{"insecure-web"},
			wantIface: &deployTemplateInterfaces{
				Adapters: []string{"web"},
			},
		},
		{
			name:     "slack only → no auth",
			adapters: []string{"slack"},
			wantIface: &deployTemplateInterfaces{
				Adapters: []string{"slack"},
			},
		},
		{
			name:     "web + slack → oidc auth",
			adapters: []string{"web", "slack"},
			wantIface: &deployTemplateInterfaces{
				Adapters: []string{"web", "slack"},
				Auth:     oidcAuth,
			},
		},
		{
			name:     "insecure-web + slack → no auth",
			adapters: []string{"insecure-web", "slack"},
			wantIface: &deployTemplateInterfaces{
				Adapters: []string{"web", "slack"},
			},
		},
		{
			name:     "web and insecure-web mutually exclusive",
			adapters: []string{"web", "insecure-web"},
			wantErr:  "mutually exclusive",
		},
		{
			name:     "unknown adapter",
			adapters: []string{"grpc"},
			wantErr:  "unknown adapter",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildDeployInterfaces(tc.adapters)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantIface, got)
		})
	}
}

func TestRunBlueprintDeployWithVar(t *testing.T) {
	var capturedReq deployTemplateRequest

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deployment-template") {
			json.NewDecoder(r.Body).Decode(&capturedReq) //nolint:errcheck
			jsonHandler(http.StatusOK, map[string]any{
				"template":   json.RawMessage(`{}`),
				"validation": map[string]any{"valid": true},
			})(w, r)
		} else if r.URL.Path == "/api/v1/deploy" {
			jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-1"})(w, r)
		}
	})
	setupBlueprintDeployTest(t, handler)

	// Set multiple --var flags: inline, ref, and escaped @.
	// StringArray accumulates across Set calls so we reset with ResetFlags afterwards.
	require.NoError(t, blueprintDeployCmd.Flags().Set("var", "FOO=bar"))
	require.NoError(t, blueprintDeployCmd.Flags().Set("var", "TOK=@my-secret"))
	require.NoError(t, blueprintDeployCmd.Flags().Set("var", `LIT=\@literal`))
	t.Cleanup(func() {
		blueprintDeployCmd.ResetFlags()
		blueprintCmd.AddCommand(blueprintDeployCmd)
		blueprintDeployCmd.Flags().String("name", "", "")
		blueprintDeployCmd.Flags().StringArray("adapter", nil, "")
		blueprintDeployCmd.Flags().StringArray("var", nil, "")
		blueprintDeployCmd.Flags().String("vars-file", "", "")
		blueprintDeployCmd.Flags().String("build", "", "")
		blueprintDeployCmd.Flags().Bool("dry-run", false, "")
		blueprintDeployCmd.Flags().Bool("json", false, "")
	})

	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())
	require.NoError(t, runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"}))

	require.NotNil(t, capturedReq.Variables)
	assert.Equal(t, deployVarInput{Value: "bar"}, capturedReq.Variables["FOO"])
	assert.Equal(t, deployVarInput{Ref: "my-secret"}, capturedReq.Variables["TOK"])
	assert.Equal(t, deployVarInput{Value: "@literal"}, capturedReq.Variables["LIT"])
}

func TestRunBlueprintDeployVarsFile(t *testing.T) {
	var capturedReq deployTemplateRequest

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deployment-template") {
			json.NewDecoder(r.Body).Decode(&capturedReq) //nolint:errcheck
			jsonHandler(http.StatusOK, map[string]any{
				"template":   json.RawMessage(`{}`),
				"validation": map[string]any{"valid": true},
			})(w, r)
		} else if r.URL.Path == "/api/v1/deploy" {
			jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-1"})(w, r)
		}
	})
	setupBlueprintDeployTest(t, handler)

	tmp := t.TempDir()
	envFile := filepath.Join(tmp, ".env")
	require.NoError(t, os.WriteFile(envFile, []byte("FILE_VAR=from_file\nOVERRIDDEN=file_val\n"), 0600))

	require.NoError(t, blueprintDeployCmd.Flags().Set("vars-file", envFile))
	require.NoError(t, blueprintDeployCmd.Flags().Set("var", "OVERRIDDEN=flag_val"))
	t.Cleanup(func() {
		blueprintDeployCmd.ResetFlags()
		blueprintCmd.AddCommand(blueprintDeployCmd)
		blueprintDeployCmd.Flags().String("name", "", "")
		blueprintDeployCmd.Flags().StringArray("adapter", nil, "")
		blueprintDeployCmd.Flags().StringArray("var", nil, "")
		blueprintDeployCmd.Flags().String("vars-file", "", "")
		blueprintDeployCmd.Flags().String("build", "", "")
		blueprintDeployCmd.Flags().Bool("dry-run", false, "")
		blueprintDeployCmd.Flags().Bool("json", false, "")
	})

	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())
	require.NoError(t, runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"}))

	require.NotNil(t, capturedReq.Variables)
	assert.Equal(t, deployVarInput{Value: "from_file"}, capturedReq.Variables["FILE_VAR"])
	// --var takes precedence over --vars-file
	assert.Equal(t, deployVarInput{Value: "flag_val"}, capturedReq.Variables["OVERRIDDEN"])
}

func TestRunBlueprintDeployBuildFlag(t *testing.T) {
	var capturedReq deployTemplateRequest

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deployment-template") {
			json.NewDecoder(r.Body).Decode(&capturedReq) //nolint:errcheck
			jsonHandler(http.StatusOK, map[string]any{
				"template":   json.RawMessage(`{}`),
				"validation": map[string]any{"valid": true},
			})(w, r)
		} else if r.URL.Path == "/api/v1/deploy" {
			jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-1"})(w, r)
		}
	})
	setupBlueprintDeployTest(t, handler)

	require.NoError(t, blueprintDeployCmd.Flags().Set("build", "abc12345"))
	t.Cleanup(func() { blueprintDeployCmd.Flags().Set("build", "") }) //nolint:errcheck

	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())
	require.NoError(t, runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"}))

	assert.Equal(t, "abc12345", capturedReq.Build)
}

func TestRunBlueprintDeployDisplayName(t *testing.T) {
	var capturedDeployBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deployment-template") {
			jsonHandler(http.StatusOK, map[string]any{
				"template":   json.RawMessage(`{"spec":"deployment/v1","target":{"runtime":"kubernetes"}}`),
				"validation": map[string]any{"valid": true},
			})(w, r)
		} else if r.URL.Path == "/api/v1/deploy" {
			json.NewDecoder(r.Body).Decode(&capturedDeployBody) //nolint:errcheck
			jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-1"})(w, r)
		}
	})
	setupBlueprintDeployTest(t, handler)

	require.NoError(t, blueprintDeployCmd.Flags().Set("name", "My Cool Agent"))
	t.Cleanup(func() { blueprintDeployCmd.Flags().Set("name", "") }) //nolint:errcheck

	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())
	require.NoError(t, runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"}))

	require.NotNil(t, capturedDeployBody)
	target := capturedDeployBody["target"].(map[string]any)
	assert.Equal(t, "My Cool Agent", target["display_name"])
}

func TestRunBlueprintDeployDefaultDisplayName(t *testing.T) {
	// When --name is not set, display_name should default to the blueprint name.
	var capturedDeployBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deployment-template") {
			jsonHandler(http.StatusOK, map[string]any{
				"template":   json.RawMessage(`{"spec":"deployment/v1","target":{"runtime":"kubernetes"}}`),
				"validation": map[string]any{"valid": true},
			})(w, r)
		} else if r.URL.Path == "/api/v1/deploy" {
			json.NewDecoder(r.Body).Decode(&capturedDeployBody) //nolint:errcheck
			jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-1"})(w, r)
		}
	})
	setupBlueprintDeployTest(t, handler)

	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())
	require.NoError(t, runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"}))

	require.NotNil(t, capturedDeployBody)
	target := capturedDeployBody["target"].(map[string]any)
	assert.Equal(t, "my-agent", target["display_name"])
}

func TestRunBlueprintDeployTemplateSourcePreserved(t *testing.T) {
	// Verify that source fields from the template response are preserved in the deploy POST.
	templateBody := `{"spec":"deployment/v1","source":{"name":"my-agent","build":"abc12345"},"target":{"runtime":"kubernetes"}}`
	var capturedDeployBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deployment-template") {
			jsonHandler(http.StatusOK, map[string]any{
				"template":   json.RawMessage(templateBody),
				"validation": map[string]any{"valid": true},
			})(w, r)
		} else if r.URL.Path == "/api/v1/deploy" {
			json.NewDecoder(r.Body).Decode(&capturedDeployBody) //nolint:errcheck
			jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-1"})(w, r)
		}
	})
	setupBlueprintDeployTest(t, handler)

	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())
	require.NoError(t, runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"}))

	assert.Equal(t, "deployment/v1", capturedDeployBody["spec"])
	assert.Equal(t, "abc12345", capturedDeployBody["source"].(map[string]any)["build"])
	assert.Equal(t, "my-agent", capturedDeployBody["target"].(map[string]any)["display_name"])
}

func TestNotFoundFromTemplateErr(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		account string
		blue    string
		build   string
		foreign bool
		want    []string
	}{
		// Typed error_code path (current servers).
		{
			name:  "typed: build_not_found with explicit build id",
			body:  `{"error":"no builds found for agent","error_code":"build_not_found"}`,
			blue:  "my-agent",
			build: "deadbeef",
			want:  []string{`build "deadbeef" not found`, `blueprint "my-agent"`, "ast push my-agent"},
		},
		{
			name: "typed: build_not_found without --build",
			body: `{"error":"no builds found for agent","error_code":"build_not_found"}`,
			blue: "my-agent",
			want: []string{`blueprint "my-agent" has no builds yet`, "ast push my-agent"},
		},
		{
			name:    "typed: build_not_found for another account's blueprint",
			body:    `{"error":"no builds found for agent","error_code":"build_not_found"}`,
			account: "other-org", blue: "my-agent", build: "deadbeef", foreign: true,
			want: []string{errPublishedBuildNotFound("deadbeef", "other-org/my-agent").Error()},
		},
		{
			name:    "typed: no builds for another account's blueprint",
			body:    `{"error":"no builds found for agent","error_code":"build_not_found"}`,
			account: "other-org", blue: "my-agent", foreign: true,
			want: []string{errBlueprintNoPublishedBuild("other-org/my-agent").Error()},
		},
		{
			name:    "typed: account_not_found",
			body:    `{"error":"account not found","error_code":"account_not_found"}`,
			account: "ghost", blue: "my-agent",
			want: []string{`account "ghost" not found`},
		},
		{
			name:    "typed: blueprint_not_found",
			body:    `{"error":"agent not found","error_code":"blueprint_not_found"}`,
			account: "team", blue: "missing",
			want: []string{`blueprint "missing" not found`, `account "team"`},
		},
		// Legacy text-fallback path (old servers without error_code).
		{
			name:  "legacy: build text with --build",
			body:  `{"error":"no builds found for agent","details":"sql: no rows in result set"}`,
			blue:  "my-agent",
			build: "deadbeef",
			want:  []string{`build "deadbeef" not found`, `blueprint "my-agent"`, "ast push my-agent"},
		},
		{
			name:    "legacy: account not found text",
			body:    `{"error":"account not found"}`,
			account: "ghost", blue: "my-agent",
			want: []string{`account "ghost" not found`},
		},
		{
			name: "unknown 404 body falls back to generic legacy message",
			body: `{"error":"something else"}`,
			blue: "my-agent",
			want: []string{`blueprint "my-agent" not found`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := errFromAPI(http.StatusNotFound, tc.body)
			out := notFoundFromTemplateErr(err, tc.account, tc.blue, tc.build, !tc.foreign)
			require.Error(t, out)
			for _, sub := range tc.want {
				assert.Contains(t, out.Error(), sub)
			}
		})
	}
}

func errFromAPI(status int, body string) error {
	return &apiCallTestError{msg: "server returned status " + http.StatusText(status) + ": " + body}
}

type apiCallTestError struct{ msg string }

func (e *apiCallTestError) Error() string { return e.msg }

func TestBuildDeployInterfacesSendsNoGrants(t *testing.T) {
	iface, err := buildDeployInterfaces([]string{"web", "slack"})
	require.NoError(t, err)
	body, err := json.Marshal(iface)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "grants",
		"the server refuses grants in a deployment template; they are set with ast agent access set")
}

func TestOptionalBlueprintRef(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{name: "no argument", args: nil},
		{name: "bare name", args: []string{"my-agent"}},
		{name: "account and name", args: []string{"other-org/shared-agent"}},
		{name: "astropods.yml form", args: []string{"@other-org/shared-agent"}},
		{name: "empty account", args: []string{"/shared-agent"}, wantErr: errInvalidBlueprintAccount("/shared-agent")},
		{name: "uppercase account", args: []string{"Other-Org/shared-agent"}, wantErr: errInvalidBlueprintAccount("Other-Org/shared-agent")},
		{name: "invalid name after account", args: []string{"other-org/Shared"}, wantErr: optionalValidAgentName(nil, []string{"Shared"})},
		{name: "extra path segment", args: []string{"other-org/shared/agent"}, wantErr: optionalValidAgentName(nil, []string{"shared/agent"})},
		{name: "two arguments", args: []string{"a1", "b2"}, wantErr: optionalValidAgentName(nil, []string{"a1", "b2"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := optionalBlueprintRef(blueprintDeployCmd, tc.args)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.wantErr.Error())
		})
	}
}

func TestRunBlueprintDeployBlueprintRef(t *testing.T) {
	cases := []struct {
		ref         string
		wantPath    string
		wantLabel   string
		wantDisplay string
	}{
		{
			ref:         "my-agent",
			wantPath:    "/api/v1/agents/testaccount/my-agent/deployment-template",
			wantLabel:   "Deploying blueprint my-agent as agent my-agent",
			wantDisplay: "my-agent",
		},
		{
			ref:         "testaccount/my-agent",
			wantPath:    "/api/v1/agents/testaccount/my-agent/deployment-template",
			wantLabel:   "Deploying blueprint my-agent as agent my-agent",
			wantDisplay: "my-agent",
		},
		{
			ref:         "other-org/shared-agent",
			wantPath:    "/api/v1/agents/other-org/shared-agent/deployment-template",
			wantLabel:   "Deploying blueprint other-org/shared-agent as agent shared-agent",
			wantDisplay: "shared-agent",
		},
		{
			ref:         "@other-org/shared-agent",
			wantPath:    "/api/v1/agents/other-org/shared-agent/deployment-template",
			wantLabel:   "Deploying blueprint other-org/shared-agent as agent shared-agent",
			wantDisplay: "shared-agent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.ref, func(t *testing.T) {
			var (
				tmplPath   string
				tmplReq    deployTemplateRequest
				deployBody map[string]any
			)
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/deployment-template") {
					tmplPath = r.URL.Path
					json.NewDecoder(r.Body).Decode(&tmplReq) //nolint:errcheck
					jsonHandler(http.StatusOK, map[string]any{
						"template":   json.RawMessage(`{"spec":"deployment/v1","target":{"runtime":"kubernetes"}}`),
						"validation": map[string]any{"valid": true},
					})(w, r)
				} else if r.URL.Path == "/api/v1/deploy" {
					json.NewDecoder(r.Body).Decode(&deployBody) //nolint:errcheck
					jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-1"})(w, r)
				} else {
					http.NotFound(w, r)
				}
			})
			setupBlueprintDeployTest(t, handler)

			buf := &bytes.Buffer{}
			blueprintDeployCmd.SetOut(buf)
			blueprintDeployCmd.SetContext(context.Background())
			require.NoError(t, runBlueprintDeploy(blueprintDeployCmd, []string{tc.ref}))

			assert.Equal(t, tc.wantPath, tmplPath)
			assert.Equal(t, "testaccount", tmplReq.TargetAccount)
			assert.Contains(t, stripANSI(buf.String()), tc.wantLabel)
			require.NotNil(t, deployBody)
			assert.Equal(t, tc.wantDisplay, deployBody["target"].(map[string]any)["display_name"])
		})
	}
}

func TestRunBlueprintDeployNotFoundNamesThePublisher(t *testing.T) {
	setupBlueprintDeployTest(t, jsonHandler(http.StatusNotFound, map[string]any{
		"error":      "agent not found",
		"error_code": errCodeBlueprintNotFound,
	}))
	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())

	err := runBlueprintDeploy(blueprintDeployCmd, []string{"other-org/shared-agent"})
	require.EqualError(t, err, errBlueprintNotFound("shared-agent", "other-org").Error())
}
