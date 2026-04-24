package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupBlueprintTest starts a test server, redirects blueprint API calls to it,
// and writes credentials so getCurrentAccountToken resolves without real tokens.
func setupBlueprintTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	blueprintServerURLOverride = srv.URL
	t.Cleanup(func() { blueprintServerURLOverride = "" })
}

// jsonHandler returns an HTTP handler that responds with status and body.
func jsonHandler(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			json.NewEncoder(w).Encode(body) //nolint:errcheck
		}
	}
}

func TestBlueprintLatestVersion(t *testing.T) {
	cases := []struct {
		name        string
		versions    []blueprintVersionSummary
		wantNil     bool
		wantBuildID string
		wantDate    string
	}{
		{
			name:    "nil when no versions",
			wantNil: true,
		},
		{
			name:        "single version",
			versions:    []blueprintVersionSummary{{BuildID: "aaa", PublishedAt: "2026-01-01T10:00:00"}},
			wantBuildID: "aaa",
			wantDate:    "2026-01-01T10:00:00",
		},
		{
			name: "returns version with latest published_at",
			versions: []blueprintVersionSummary{
				{BuildID: "old", PublishedAt: "2026-01-01T10:00:00"},
				{BuildID: "new", PublishedAt: "2026-03-01T12:00:00"},
			},
			wantBuildID: "new",
			wantDate:    "2026-03-01T12:00:00",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := blueprintLatestVersion(tc.versions)
			if tc.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tc.wantBuildID, got.BuildID)
				assert.Equal(t, tc.wantDate, got.PublishedAt)
			}
		})
	}
}

func TestBlueprintList(t *testing.T) {
	payload := map[string]any{
		"agents": []any{
			map[string]any{
				"name":       "agent-one",
				"visibility": "private",
				"versions":   []any{map[string]any{"build_id": "abc12345", "published_at": "2026-01-01T00:00:00"}},
				"metrics":    map[string]any{"deploy_count": 5, "lifetime_messages": 200},
			},
			map[string]any{"name": "agent-two", "visibility": "public", "versions": []any{}, "metrics": nil},
		},
		"count": 2,
	}

	cases := []struct {
		name       string
		statusCode int
		body       any
		jsonOutput bool
		wantErr    bool
		wantOut    string
	}{
		{
			name:       "shows published_at and build_id",
			statusCode: http.StatusOK,
			body:       payload,
			wantOut:    "abc12345",
		},
		{
			name:       "shows pending for unpublished blueprint",
			statusCode: http.StatusOK,
			body:       payload,
			wantOut:    "pending",
		},
		{
			name:       "shows deploy count",
			statusCode: http.StatusOK,
			body:       payload,
			wantOut:    "5",
		},
		{
			name:       "empty account",
			statusCode: http.StatusOK,
			body:       map[string]any{"agents": []any{}, "count": 0},
			wantOut:    "No blueprints",
		},
		{
			name:       "json output",
			statusCode: http.StatusOK,
			body:       payload,
			jsonOutput: true,
			wantOut:    `"count"`,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			body:       map[string]any{"error": "internal error"},
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupBlueprintTest(t, jsonHandler(tc.statusCode, tc.body))
			if tc.jsonOutput {
				require.NoError(t, blueprintListCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { blueprintListCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}
			buf := &bytes.Buffer{}
			blueprintListCmd.SetOut(buf)
			blueprintListCmd.SetContext(context.Background())

			err := runBlueprintList(blueprintListCmd, nil)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
		})
	}
}

func TestBlueprintCreate(t *testing.T) {
	cases := []struct {
		name       string
		agentName  string
		visibility string
		statusCode int
		body       any
		wantErr    bool
		wantVis    string
	}{
		{
			name:       "private by default",
			agentName:  "my-agent",
			statusCode: http.StatusCreated,
			body:       map[string]any{"account": "testaccount", "name": "my-agent"},
			wantVis:    "private",
		},
		{
			name:       "public with flag",
			agentName:  "pub-agent",
			visibility: "public",
			statusCode: http.StatusCreated,
			body:       map[string]any{"account": "testaccount", "name": "pub-agent"},
			wantVis:    "public",
		},
		{
			name:       "invalid visibility rejected",
			agentName:  "bad-agent",
			visibility: "publc",
			wantErr:    true,
		},
		{
			name:      "name too short",
			agentName: "ab",
			wantErr:   true,
		},
		{
			name:      "name with invalid characters",
			agentName: "my agent!",
			wantErr:   true,
		},
		{
			name:       "name with hyphen allowed",
			agentName:  "my-agent",
			statusCode: http.StatusCreated,
			body:       map[string]any{"account": "testaccount", "name": "my-agent"},
			wantVis:    "private",
		},
		{
			name:       "conflict",
			agentName:  "existing",
			statusCode: http.StatusConflict,
			body:       map[string]any{"error": "conflict"},
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]string
			setupBlueprintTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				json.NewDecoder(r.Body).Decode(&gotBody) //nolint:errcheck
				jsonHandler(tc.statusCode, tc.body)(w, r)
			}))

			if tc.visibility != "" {
				require.NoError(t, blueprintCreateCmd.Flags().Set("visibility", tc.visibility))
				t.Cleanup(func() { blueprintCreateCmd.Flags().Set("visibility", "private") }) //nolint:errcheck
			}
			blueprintCreateCmd.SetContext(context.Background())

			err := runBlueprintCreate(blueprintCreateCmd, []string{tc.agentName})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantVis, gotBody["visibility"])
			assert.Equal(t, tc.agentName, gotBody["name"])
		})
	}
}

func TestBlueprintGet(t *testing.T) {
	payload := map[string]any{
		"account":    "testaccount",
		"name":       "my-agent",
		"visibility": "public",
		"versions":   []any{map[string]any{"build_id": "abc123", "published_at": "2026-01-01T00:00:00Z"}},
		"metrics":    map[string]any{"deploy_count": 3, "lifetime_messages": 500},
	}

	cases := []struct {
		name       string
		statusCode int
		body       any
		jsonOutput bool
		wantErr    bool
		wantOut    string
	}{
		{
			name:       "displays detail",
			statusCode: http.StatusOK,
			body:       payload,
			wantOut:    "my-agent",
		},
		{
			name:       "json output",
			statusCode: http.StatusOK,
			body:       payload,
			jsonOutput: true,
			wantOut:    `"visibility"`,
		},
		{
			name:       "shows pending hint when no versions",
			statusCode: http.StatusOK,
			body:       map[string]any{"account": "testaccount", "name": "my-agent", "visibility": "private", "versions": []any{}},
			wantOut:    "Waiting for first push",
		},
		{
			name:       "not found",
			statusCode: http.StatusNotFound,
			body:       map[string]any{"error": "not found"},
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupBlueprintTest(t, jsonHandler(tc.statusCode, tc.body))
			if tc.jsonOutput {
				require.NoError(t, blueprintGetCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { blueprintGetCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}
			buf := &bytes.Buffer{}
			blueprintGetCmd.SetOut(buf)
			blueprintGetCmd.SetContext(context.Background())

			err := runBlueprintGet(blueprintGetCmd, []string{"my-agent"})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
		})
	}
}

func TestBlueprintArchive(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       any
		wantErr    bool
	}{
		{name: "success", statusCode: http.StatusNoContent},
		{name: "not found", statusCode: http.StatusNotFound, body: map[string]any{"error": "not found"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupBlueprintTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.True(t, strings.HasSuffix(r.URL.Path, "/archive"))
				jsonHandler(tc.statusCode, tc.body)(w, r)
			}))
			blueprintArchiveCmd.SetContext(context.Background())

			err := runBlueprintArchive(blueprintArchiveCmd, []string{"my-agent"})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBlueprintUpdate(t *testing.T) {
	cases := []struct {
		name       string
		agentName  string
		visibility string
		statusCode int
		body       any
		wantErr    bool
		wantCalled bool
	}{
		{
			name:      "no flags returns error without calling server",
			agentName: "my-agent",
			wantErr:   true,
		},
		{
			name:       "invalid visibility returns error without calling server",
			agentName:  "my-agent",
			visibility: "readonly",
			wantErr:    true,
		},
		{
			name:       "sets public",
			agentName:  "my-agent",
			visibility: "public",
			statusCode: http.StatusOK,
			body:       map[string]any{"visibility": "public"},
			wantCalled: true,
		},
		{
			name:       "sets private",
			agentName:  "my-agent",
			visibility: "private",
			statusCode: http.StatusOK,
			body:       map[string]any{"visibility": "private"},
			wantCalled: true,
		},
		{
			name:       "not found",
			agentName:  "ghost",
			visibility: "public",
			statusCode: http.StatusNotFound,
			body:       map[string]any{"error": "not found"},
			wantErr:    true,
			wantCalled: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			setupBlueprintTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				assert.Equal(t, http.MethodPut, r.Method)
				assert.True(t, strings.HasSuffix(r.URL.Path, "/visibility"))
				jsonHandler(tc.statusCode, tc.body)(w, r)
			}))
			blueprintUpdateCmd.SetContext(context.Background())
			if tc.visibility != "" {
				require.NoError(t, blueprintUpdateCmd.Flags().Set("visibility", tc.visibility))
				t.Cleanup(func() { blueprintUpdateCmd.Flags().Set("visibility", "") }) //nolint:errcheck
			}

			err := runBlueprintUpdate(blueprintUpdateCmd, []string{tc.agentName})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantCalled, called)
		})
	}
}
