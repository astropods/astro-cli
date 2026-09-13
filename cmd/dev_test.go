package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/config"
	"github.com/astropods/astro-cli/internal/utils"
	spec "github.com/astropods/astro-spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevStatePath(t *testing.T) {
	path, err := devStatePath()
	if err != nil {
		t.Fatalf("devStatePath() error = %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join(buildinfo.AppDirName, ".running")) {
		t.Errorf("devStatePath() = %q, want suffix %q", path, filepath.Join(buildinfo.AppDirName, ".running"))
	}
}

func TestReadDevProjectName(t *testing.T) {
	t.Run("reads project name from file", func(t *testing.T) {
		dir := t.TempDir()
		statePath := filepath.Join(dir, ".running")
		if err := os.WriteFile(statePath, []byte("my-agent\n"), 0600); err != nil {
			t.Fatal(err)
		}
		name, err := readDevProjectName(statePath, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "my-agent" {
			t.Errorf("name = %q, want %q", name, "my-agent")
		}
	})

	// Regression guard for the scoped-name cleanup issue: older CLIs wrote the
	// raw scoped spec name into .running. The current writer stores the
	// sanitized compose project name, but readDevProjectName must still
	// normalize legacy files so `ast dev logs`/`ast dev stop` can find the
	// actual compose project.
	t.Run("normalizes legacy scoped spec name in state file", func(t *testing.T) {
		dir := t.TempDir()
		statePath := filepath.Join(dir, ".running")
		if err := os.WriteFile(statePath, []byte("@org/my-agent\n"), 0600); err != nil {
			t.Fatal(err)
		}
		name, err := readDevProjectName(statePath, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "my-agent" {
			t.Errorf("name = %q, want %q (legacy scoped name must be normalized)", name, "my-agent")
		}
	})

	t.Run("returns error when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		_, err := readDevProjectName(filepath.Join(dir, ".running"), nil)
		if err == nil {
			t.Fatal("expected error when state file is missing")
		}
	})
}

func TestDevTriggerHasEnvFlag(t *testing.T) {
	f := devTriggerCmd.Flags().Lookup("env")
	require.NotNil(t, f, "dev trigger is missing the --env flag")
	assert.Equal(t, utils.DefaultEnvFile, f.DefValue)
}

// assembleDevEnv is what stands between a triggered ingestion job and an
// unreachable gateway. Removing the injectAIGatewayDevKey call from it still
// compiles and leaves every other test green, so this is the only thing that
// catches that.
func TestAssembleDevEnv_InjectsGatewayKeyOnlyWhenSpecUsesGateway(t *testing.T) {
	const (
		fakeKey = "sk-dev-fake"
		fakeURL = "https://gateway.example.test/v1"
	)

	tests := []struct {
		name       string
		spec       *spec.AstroSpec
		wantKey    bool
		wantNotice bool
	}{
		{
			name:       "spec uses the gateway",
			spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x", AIGateway: true}},
			wantKey:    true,
			wantNotice: true,
		},
		{
			name:    "spec does not use the gateway",
			spec:    &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
			wantKey: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// applyAIGatewayDevKey also exports into the process env, so clear
			// both keys first or a previous subtest's values look like this
			// subtest's result.
			t.Setenv("ASTRO_GATEWAY_API_KEY", "")
			t.Setenv("ASTRO_GATEWAY_URL", "")
			t.Setenv("HOME", t.TempDir())
			writeAccountTestCredentials(t, accountTestCreds("alice"))

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"key_id":"k1","api_key":"` + fakeKey +
					`","base_url":"` + fakeURL + `","expires_at":"2026-01-01T00:00:00Z"}`))
			}))
			defer srv.Close()
			prev := aiGatewayServerURLOverride
			aiGatewayServerURLOverride = srv.URL
			defer func() { aiGatewayServerURLOverride = prev }()

			workingDir := t.TempDir()
			require.NoError(t, os.WriteFile(
				filepath.Join(workingDir, ".env"), []byte("FROM_ENV_FILE=1\n"), 0o600,
			))

			var out strings.Builder
			envVars, _, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
				Spec: tc.spec, WorkingDir: workingDir, EnvFile: ".env",
			})
			require.NoError(t, err)

			assert.Equal(t, "1", envVars["FROM_ENV_FILE"],
				"the env file must still be loaded either way")

			if tc.wantKey {
				assert.Equal(t, fakeKey, envVars["ASTRO_GATEWAY_API_KEY"],
					"a triggered job cannot reach the gateway without this key")
				assert.Equal(t, fakeURL, envVars["ASTRO_GATEWAY_URL"])
			} else {
				assert.NotContains(t, envVars, "ASTRO_GATEWAY_API_KEY",
					"a spec without the gateway must not mint a key")
				assert.NotContains(t, envVars, "ASTRO_GATEWAY_URL")
			}

			if tc.wantNotice {
				assert.Contains(t, out.String(), "AI Gateway: dev key minted",
					"the notice must go to the command writer, not stdout")
			} else {
				assert.Empty(t, out.String())
			}
		})
	}
}

func TestAssembleDevEnv_StoredVarsOverrideEnvFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(workingDir, ".env"),
		[]byte("SHARED=from-env-file\nONLY_FILE=1\n"), 0o600,
	))
	require.NoError(t, config.SetProjectVars(
		buildinfo.BinaryName, workingDir, "ag",
		map[string]string{"SHARED": "from-store", "ONLY_STORE": "2"},
	))

	var out strings.Builder
	envVars, counts, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
		WorkingDir: workingDir, EnvFile: ".env",
	})
	require.NoError(t, err)

	assert.Equal(t, "from-store", envVars["SHARED"],
		"the project store wins over the env file, which is what configure relies on")
	assert.Equal(t, "1", envVars["ONLY_FILE"], "a file-only value survives the overlay")
	assert.Equal(t, "2", envVars["ONLY_STORE"], "a store-only value is added")
	assert.Equal(t, 2, counts.FromFile)
	assert.Equal(t, 2, counts.FromStore)
}

func TestAssembleDevEnv_ReportsMissingLoginForGatewaySpec(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no credentials written

	var out strings.Builder
	_, _, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x", AIGateway: true}},
		WorkingDir: t.TempDir(), EnvFile: ".env",
	})

	require.Error(t, err)
	// Exact-string comparison against the message function, per CLAUDE.md:
	// rebuilding it from this error's own cause fails if anyone inlines a
	// different string at the call site.
	assert.Equal(t, errAIGatewayRequiresLogin(errors.Unwrap(err)).Error(), err.Error(),
		"the login error must come from errAIGatewayRequiresLogin")
}

func TestAssembleDevEnv_ReportsAnAbsentFileDistinctlyFromAnEmptyOne(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var out strings.Builder
	_, counts, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
		WorkingDir: t.TempDir(), EnvFile: ".env", // no file written
	})
	require.NoError(t, err)

	assert.False(t, counts.FileFound,
		"an absent file must be distinguishable from an empty one, which start words differently")
	assert.Zero(t, counts.FromFile)
}

func TestAssembleDevEnv_ExportsToProcessEnvOnlyWhenAsked(t *testing.T) {
	const key = "ASTRO_TEST_EXPORTED"

	for _, tc := range []struct {
		name   string
		export bool
		want   string
	}{
		{name: "export on", export: true, want: "yes"},
		{name: "export off", export: false, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv(key, "")
			workingDir := t.TempDir()
			require.NoError(t, os.WriteFile(
				filepath.Join(workingDir, ".env"), []byte(key+"=yes\n"), 0o600,
			))

			var out strings.Builder
			envVars, _, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
				Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
				WorkingDir: workingDir, EnvFile: ".env", Export: tc.export,
			})
			require.NoError(t, err)

			assert.Equal(t, "yes", envVars[key], "the returned map carries it either way")
			assert.Equal(t, tc.want, os.Getenv(key),
				"only the start path mirrors env into this process; a triggered job gets it via compose")
		})
	}
}

// The gateway notice is written from inside assembleDevEnv, so a caller that
// printed from the returned counts would emit its own lines after it. OnStage
// exists to keep start's narration in front, and this pins that order.
func TestAssembleDevEnv_OnStageRunsBeforeTheGatewayNotice(t *testing.T) {
	t.Setenv("ASTRO_GATEWAY_API_KEY", "")
	t.Setenv("ASTRO_GATEWAY_URL", "")
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("alice"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key_id":"k1","api_key":"sk","base_url":"u","expires_at":"t"}`))
	}))
	defer srv.Close()
	prev := aiGatewayServerURLOverride
	aiGatewayServerURLOverride = srv.URL
	defer func() { aiGatewayServerURLOverride = prev }()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(workingDir, ".env"), []byte("A=1\n"), 0o600,
	))

	var out strings.Builder
	var stages []devEnvStage
	_, _, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x", AIGateway: true}},
		WorkingDir: workingDir, EnvFile: ".env",
		OnStage: func(stage devEnvStage, _ devEnvCounts) {
			stages = append(stages, stage)
			fmt.Fprintf(&out, "stage-%d\n", stage)
		},
	})
	require.NoError(t, err)

	assert.Equal(t, []devEnvStage{devEnvStageFile, devEnvStageStore}, stages,
		"the file is read before the store, which is what gives the store priority")
	got := out.String()
	assert.Less(t, strings.Index(got, "stage-1"), strings.Index(got, "AI Gateway"),
		"both stage callbacks must fire before the gateway notice is written")
}
