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
	"github.com/spf13/cobra"
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

func TestDevCommandsHaveEnvFileFlag(t *testing.T) {
	for _, c := range []*cobra.Command{devCmd, devStartCmd, devTriggerCmd} {
		t.Run(c.CommandPath(), func(t *testing.T) {
			f := c.Flags().Lookup(envFileFlag)
			require.NotNil(t, f, "%s is missing the --env-file flag", c.CommandPath())
			assert.Equal(t, utils.DefaultEnvFile, f.DefValue)

			old := c.Flags().Lookup(deprecatedEnvFileFlag)
			require.NotNil(t, old, "%s must keep --env as an alias until it is removed", c.CommandPath())
			assert.Equal(t, "use --env-file instead", old.Deprecated, "--env must warn and point at --env-file")
			assert.True(t, old.Hidden, "--env must not appear in help")
		})
	}
}

// assembleDevEnv is what stands between a triggered job and an
// unreachable gateway. Removing the injectAIGatewayDevKey call from it still
// compiles and leaves every other test green, so this is the only thing that
// catches that.
func TestAssembleDevEnv_InjectsGatewayKeyOnlyWhenSpecUsesGateway(t *testing.T) {
	const (
		fakeKey    = "sk-dev-fake"
		fakeURL    = "https://gateway.example.test/v1"
		fakeExpiry = "2026-01-01T00:00:00Z"
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
					`","base_url":"` + fakeURL + `","expires_at":"` + fakeExpiry + `"}`))
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
				assert.Contains(t, out.String(), msgAIGatewayKeyMinted(fakeExpiry),
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

func TestAssembleDevEnv_AsksForASandboxOnlyWhenTheSpecDeclaresOne(t *testing.T) {
	// The section's presence is the request. Without a spec field the CLI had
	// to be told on the command line every time.
	for _, tc := range []struct {
		name    string
		sandbox *spec.Sandbox
		wants   bool
	}{
		{name: "no section", sandbox: nil, wants: false},
		{name: "a section", sandbox: &spec.Sandbox{Toolchain: "auto"}, wants: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir()) // no credentials, so a request fails loudly

			var out strings.Builder
			_, _, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
				Spec: &spec.AstroSpec{
					Name:    "ag",
					Agent:   spec.Container{Image: "x"},
					Sandbox: tc.sandbox,
				},
				WorkingDir: t.TempDir(), EnvFile: ".env",
			})

			if !tc.wants {
				require.NoError(t, err, "a spec with no sandbox section must not ask the author to log in")
				assert.NotContains(t, out.String(), "andbox")
				return
			}
			require.Error(t, err, "a declared sandbox needs a session, which needs credentials")
			assert.Equal(t, errSandboxRequiresLogin(errors.Unwrap(err)).Error(), err.Error(),
				"the login error must come from errSandboxRequiresLogin")
		})
	}
}

func TestDeclaresSandboxReadsTheSectionAndNothingElse(t *testing.T) {
	assert.False(t, declaresSandbox(nil))
	assert.False(t, declaresSandbox(&spec.AstroSpec{Name: "ag"}))
	assert.True(t, declaresSandbox(&spec.AstroSpec{
		Name:    "ag",
		Sandbox: &spec.Sandbox{Toolchain: "never"},
	}))
}

func setEnvFlagForTest(t *testing.T, c *cobra.Command, name, value string) {
	t.Helper()
	f := c.Flags().Lookup(name)
	require.NotNil(t, f)
	t.Cleanup(func() {
		require.NoError(t, f.Value.Set(f.DefValue))
		f.Changed = false
	})
	require.NoError(t, c.Flags().Set(name, value))
}

func TestDevEnvCommands_FailBeforeWorkOnMissingExplicitEnvFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.env")

	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{name: "project", cmd: devCmd},
		{name: "project start", cmd: devStartCmd},
		{name: "project trigger", cmd: devTriggerCmd, args: []string{"job"}},
	} {
		for _, flag := range []string{envFileFlag, deprecatedEnvFileFlag} {
			t.Run(tc.name+" --"+flag, func(t *testing.T) {
				setEnvFlagForTest(t, tc.cmd, flag, missing)

				err := tc.cmd.RunE(tc.cmd, tc.args)

				require.Error(t, err)
				assert.Equal(t, errEnvFileNotFound(missing).Error(), err.Error(),
					"an explicit --%s that does not exist must stop the command before Docker or the spec is touched", flag)
			})
		}
	}
}

func TestDevEnvFileFlag(t *testing.T) {
	workingDir := t.TempDir()
	present := filepath.Join(t.TempDir(), "run.env")
	require.NoError(t, os.WriteFile(present, []byte("A=1\n"), 0o600))

	for _, c := range []*cobra.Command{devCmd, devStartCmd, devTriggerCmd} {
		t.Run(c.CommandPath()+" default with no .env", func(t *testing.T) {
			envFile, explicit, err := devEnvFileFlag(c, workingDir)
			require.NoError(t, err, "a project without a .env must still start")
			assert.Equal(t, utils.DefaultEnvFile, envFile)
			assert.False(t, explicit)
		})
		for _, flag := range []string{envFileFlag, deprecatedEnvFileFlag} {
			t.Run(c.CommandPath()+" explicit existing file via --"+flag, func(t *testing.T) {
				setEnvFlagForTest(t, c, flag, present)
				envFile, explicit, err := devEnvFileFlag(c, workingDir)
				require.NoError(t, err)
				assert.Equal(t, present, envFile)
				assert.True(t, explicit, "a set --%s must reach assembleDevEnv as explicit", flag)
			})
		}
		t.Run(c.CommandPath()+" an existing file passes the early check without being parsed", func(t *testing.T) {
			broken := filepath.Join(t.TempDir(), "broken.env")
			require.NoError(t, os.WriteFile(broken, []byte("A=\"unterminated\n"), 0o600))
			setEnvFlagForTest(t, c, envFileFlag, broken)
			envFile, explicit, err := devEnvFileFlag(c, workingDir)
			require.NoError(t, err, "the early check confirms the file exists; assembleDevEnv is the one place that parses it")
			assert.Equal(t, broken, envFile)
			assert.True(t, explicit)
		})
		t.Run(c.CommandPath()+" --env-file wins over --env", func(t *testing.T) {
			setEnvFlagForTest(t, c, envFileFlag, present)
			setEnvFlagForTest(t, c, deprecatedEnvFileFlag, filepath.Join(t.TempDir(), "absent.env"))
			envFile, explicit, err := devEnvFileFlag(c, workingDir)
			require.NoError(t, err, "a missing --env must be ignored when --env-file is set")
			assert.Equal(t, present, envFile)
			assert.True(t, explicit)
		})
	}
}

func TestAssembleDevEnv_EnvFileResolution(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "rel.env"), []byte("REL=1\n"), 0o600))
	absFile := filepath.Join(t.TempDir(), "abs.env")
	require.NoError(t, os.WriteFile(absFile, []byte("ABS=1\n"), 0o600))
	missing := filepath.Join(t.TempDir(), "absent.env")
	broken := filepath.Join(t.TempDir(), "broken.env")
	require.NoError(t, os.WriteFile(broken, []byte("A=\"unterminated\n"), 0o600))
	_, parseErr := utils.LoadEnvFile(workingDir, broken, true)
	require.Error(t, parseErr, "the fixture must be a file godotenv rejects")

	for _, tc := range []struct {
		name     string
		envFile  string
		explicit bool
		wantKey  string
		wantPath string
		wantErr  error
	}{
		{name: "a file that doesn't parse names that file", envFile: broken, explicit: true, wantErr: errEnvFileUnreadable(broken, parseErr)},
		{name: "explicit absolute path is read as given", envFile: absFile, explicit: true, wantKey: "ABS", wantPath: absFile},
		{name: "explicit relative path resolves against the working directory", envFile: "rel.env", explicit: true, wantKey: "REL", wantPath: filepath.Join(workingDir, "rel.env")},
		{name: "explicit missing file fails", envFile: missing, explicit: true, wantErr: errEnvFileNotFound(missing)},
		{name: "explicit directory fails", envFile: workingDir, explicit: true, wantErr: errEnvFileNotFound(workingDir)},
		{name: "default missing file is skipped", envFile: utils.DefaultEnvFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			envVars, counts, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
				Spec:        &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
				WorkingDir:  workingDir,
				EnvFile:     tc.envFile,
				EnvExplicit: tc.explicit,
			})
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			if tc.wantKey == "" {
				assert.False(t, counts.FileFound)
				return
			}
			assert.Equal(t, "1", envVars[tc.wantKey])
			assert.True(t, counts.FileFound)
			assert.Equal(t, tc.wantPath, counts.FilePath, "start and trigger log this resolved path")
		})
	}
}

func TestPrintDevEnvStage(t *testing.T) {
	var out strings.Builder
	printDevEnvStage(&out, devEnvStageFile, devEnvCounts{FileFound: true, FromFile: 2, FilePath: "/tmp/run.env"})
	printDevEnvStage(&out, devEnvStageStore, devEnvCounts{FromStore: 1})
	assert.Equal(t, msgDevEnvFileLoaded(2, "/tmp/run.env")+msgDevEnvStoreLoaded(1), out.String())
}
