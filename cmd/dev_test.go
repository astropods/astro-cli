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
	"github.com/spf13/pflag"
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

func TestEnvFileFlagsAreWiredToTheValidatingValue(t *testing.T) {
	t.Chdir(t.TempDir())

	// The flags registered through a helper are checked on a fresh command
	// rather than the package-level one: agent_deploy_test.go calls
	// ResetFlags on blueprintDeployCmd and re-registers --vars-file by hand as
	// a plain string, so the singleton's flag depends on test order.
	deployFlags := &cobra.Command{}
	registerDeployCommonFlags(deployFlags)
	configureFlags := &cobra.Command{}
	registerConfigureFlags(configureFlags)

	tests := []struct {
		name    string
		flags   *pflag.FlagSet
		flag    string
		wantDef string
	}{
		{name: "project start", flags: devStartCmd.Flags(), flag: "env", wantDef: utils.DefaultEnvFile},
		{name: "project trigger", flags: devTriggerCmd.Flags(), flag: "env", wantDef: utils.DefaultEnvFile},
		{name: "secrets import", flags: secretImportCmd.Flags(), flag: "file"},
		{name: "deploy and redeploy", flags: deployFlags.Flags(), flag: "vars-file"},
		{name: "configure", flags: configureFlags.Flags(), flag: "vars-file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.flags.Lookup(tt.flag)
			require.NotNil(t, f, "the flag must exist")
			assert.Equal(t, tt.wantDef, f.DefValue, "the default must survive the switch to a Var")
			assert.Equal(t, "string", f.Value.Type(),
				"pflag's GetString rejects any other type, so flagString would read empty")

			err := f.Value.Set("does-not-exist.env")

			require.ErrorIs(t, err, utils.ErrEnvFileNotFound,
				"a missing path must be rejected while flags parse, not inside the command")
			assert.Equal(t, tt.wantDef, f.Value.String(),
				"a rejected value must not become the flag's value")
			t.Cleanup(func() { _ = f.Value.Set(tt.wantDef) })
		})
	}
}

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

func TestAssembleDevEnv_EnvFileOverridesStoredVars(t *testing.T) {
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

	assert.Equal(t, "from-env-file", envVars["SHARED"],
		"the env file overlays the project store, so the file you just edited wins")
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

func TestAssembleDevEnv_FailsOnAnExplicitEnvFileThatIsGone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()

	var out strings.Builder
	_, _, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
		WorkingDir: dir, EnvFile: "named.env", EnvFileExplicit: true, // never written
	})

	require.Error(t, err, "a file the user named and that is gone must not be treated as an absent default")
	assert.Contains(t, err.Error(), filepath.Join(dir, "named.env"),
		"the resolved path is the useful fact, since a relative name resolves against the project dir")
}

func TestAssembleDevEnv_TreatsAnAbsentDefaultAsNoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var out strings.Builder
	_, counts, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
		WorkingDir: t.TempDir(), EnvFile: ".env", // no file, and the user never asked for one
	})

	require.NoError(t, err, "a default the user never named must stay a graceful fall-through")
	assert.False(t, counts.FileFound)
}

func TestDevTrigger_RejectsAMissingEnvFileWhileParsingFlags(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	c := &cobra.Command{Use: "trigger", RunE: func(*cobra.Command, []string) error {
		return errors.New("the command body must not run on a bad --env")
	}}
	c.Flags().Var(utils.NewEnvFileFlag(utils.DefaultEnvFile), "env", "")
	c.SetArgs([]string{"--env", "env/typo.env"})
	c.SilenceUsage, c.SilenceErrors = true, true

	err := c.Execute()

	require.Error(t, err,
		"a typo in --env must stop before the command body, not fall through to the spec defaults")
	assert.ErrorIs(t, err, utils.ErrEnvFileNotFound,
		"callers match on the sentinel, so the flag must report it rather than a bare string")
	assert.Contains(t, err.Error(), filepath.Join(dir, "env/typo.env"),
		"the error names the path that was looked for")
}

func TestAssembleDevEnv_ReadsAnAbsoluteEnvFileFromWhereItPoints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	absFile := filepath.Join(t.TempDir(), "prod.env")
	require.NoError(t, os.WriteFile(absFile, []byte("FROM_ABS=1\n"), 0o600))

	var out strings.Builder
	envVars, counts, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
		WorkingDir: t.TempDir(), EnvFile: absFile,
	})
	require.NoError(t, err)

	assert.True(t, counts.FileFound,
		"an absolute --env must not be rejoined onto the working directory")
	assert.Equal(t, "1", envVars["FROM_ABS"])
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

func TestAssembleDevEnv_ReportsWhatItLeftAlone(t *testing.T) {
	const key = "ASTRO_TEST_NARRATED"
	t.Setenv("HOME", t.TempDir())
	t.Setenv(key, "from-shell")

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(workingDir, ".env"), []byte(key+"=from-file\n"), 0o600,
	))

	var staged []devEnvCounts
	var out strings.Builder
	_, counts, err := assembleDevEnv(context.Background(), &out, devEnvOptions{
		Spec:       &spec.AstroSpec{Name: "ag", Agent: spec.Container{Image: "x"}},
		WorkingDir: workingDir, EnvFile: ".env", Export: true,
		OnStage: func(_ devEnvStage, c devEnvCounts) { staged = append(staged, c) },
	})
	require.NoError(t, err)

	assert.Equal(t, 1, counts.AlreadySet,
		"a deferred variable must be counted, or start cannot say it kept the existing value")
	require.Len(t, staged, 2, "both sources still report a stage")
	assert.Zero(t, staged[0].AlreadySet,
		"the file stage fires before the layering, so it cannot know yet")
	assert.Equal(t, 1, staged[1].AlreadySet,
		"the count is known once the layers have resolved")
}

func TestExportEnv_MirrorsTheResolvedMap(t *testing.T) {
	const key = "ASTRO_TEST_EXPORT_MIRROR"
	t.Setenv(key, "stale")

	// Precedence is resolved in utils.LayerEnv before this point, so the map
	// handed here is the answer and overwrites whatever is present.
	require.NoError(t, exportEnv(map[string]string{key: "resolved"}))

	assert.Equal(t, "resolved", os.Getenv(key))
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
