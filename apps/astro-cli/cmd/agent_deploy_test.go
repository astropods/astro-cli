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

func TestPatchTemplateDisplayName(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		displayName string
		wantField   string
	}{
		{
			name:        "sets display_name on existing target",
			input:       `{"spec":"deployment/v1","target":{"runtime":"kubernetes"}}`,
			displayName: "my deployment",
			wantField:   "my deployment",
		},
		{
			name:        "creates target if absent",
			input:       `{"spec":"deployment/v1"}`,
			displayName: "new name",
			wantField:   "new name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := patchTemplateDisplayName(json.RawMessage(tc.input), tc.displayName)
			require.NoError(t, err)

			var out map[string]any
			require.NoError(t, json.Unmarshal(result, &out))
			target := out["target"].(map[string]any)
			assert.Equal(t, tc.wantField, target["display_name"])
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
			name:       "deploy endpoint 404 reports deployment no longer exists",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusNotFound,
			deplResp:   map[string]any{"error": "not found"},
			wantErr:    `no longer exists`,
		},
		{
			name:       "deploy endpoint error",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusInternalServerError,
			deplResp:   map[string]any{"error": "k8s down"},
			wantErr:    "status 500",
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
