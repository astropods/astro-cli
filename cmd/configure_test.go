package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	"github.com/astropods/astro/apps/astro-cli/internal/config"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// collectConfigureVars reproduces the credential + non-secret provider var logic from runConfigure.
func collectConfigureVars(astroSpec *spec.AstroSpec) []varEntry {
	var credVars []varEntry

	credKeys := spec.AllCredentialKeys(astroSpec)
	sortedCredKeys := make([]string, 0, len(credKeys))
	for k := range credKeys {
		sortedCredKeys = append(sortedCredKeys, k)
	}
	sort.Strings(sortedCredKeys)
	for _, k := range sortedCredKeys {
		meta := credKeys[k]
		credVars = append(credVars, varEntry{
			key:         k,
			description: meta.Description,
			secret:      true,
		})
	}

	var providerPlainVars []varEntry
	for provName, cp := range referencedCustomProviders(astroSpec) {
		prefix := spec.SanitizeEnvName(provName)
		for _, v := range cp.Variables {
			if v.Secret {
				continue
			}
			providerPlainVars = append(providerPlainVars, varEntry{
				key:         prefix + "_" + v.Name,
				description: v.Description,
				secret:      false,
			})
		}
	}
	sort.Slice(providerPlainVars, func(i, j int) bool { return providerPlainVars[i].key < providerPlainVars[j].key })
	credVars = append(credVars, providerPlainVars...)
	return credVars
}

func configureVarKeys(entries []varEntry) map[string]varEntry {
	m := make(map[string]varEntry, len(entries))
	for _, e := range entries {
		m[e.key] = e
	}
	return m
}

// ─── Regression: non-secret provider vars must appear prefixed in configure ──

func TestConfigureCollectsNonSecretProviderVars(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Variables: []spec.Input{
					{Name: "AI_API_KEY", Secret: true, Description: "Cloudflare Workers AI API key"},
					{Name: "ACCOUNT_ID", Secret: false, Description: "Cloudflare account ID"},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))

	if _, ok := keys["CLOUDFLARE_AI_API_KEY"]; !ok {
		t.Error("expected CLOUDFLARE_AI_API_KEY (secret)")
	}
	if _, ok := keys["CLOUDFLARE_ACCOUNT_ID"]; !ok {
		t.Error("expected CLOUDFLARE_ACCOUNT_ID (non-secret, prefixed)")
	}
	if _, ok := keys["ACCOUNT_ID"]; ok {
		t.Error("bare ACCOUNT_ID must not appear — should be CLOUDFLARE_ACCOUNT_ID")
	}
}

func TestConfigureNonSecretProviderVarNotSecret(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"myapi": {
				Variables: []spec.Input{
					{Name: "TOKEN", Secret: true},
					{Name: "REGION", Secret: false, Description: "API region"},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"myapi": {Provider: "myapi"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))

	v, ok := keys["MYAPI_REGION"]
	if !ok {
		t.Fatal("MYAPI_REGION not found in configure vars")
	}
	if v.secret {
		t.Error("MYAPI_REGION should not be marked as secret")
	}
	if v.description != "API region" {
		t.Errorf("MYAPI_REGION description = %q, want %q", v.description, "API region")
	}
}

func TestConfigureNonSecretVars_NoDoubleCounting(t *testing.T) {
	// Secret vars come from AllCredentialKeys; non-secret vars come from our new path.
	// Ensure secrets don't appear twice.
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"myapi": {
				Variables: []spec.Input{
					{Name: "TOKEN", Secret: true},
					{Name: "REGION", Secret: false},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"myapi": {Provider: "myapi"},
		},
	}

	vars := collectConfigureVars(s)
	count := make(map[string]int)
	for _, v := range vars {
		count[v.key]++
	}
	for key, n := range count {
		if n > 1 {
			t.Errorf("key %q appears %d times — should appear exactly once", key, n)
		}
	}
}

func TestConfigureNonSecretVars_AllSecretsProducesNoPlainVars(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"vault": {
				Variables: []spec.Input{
					{Name: "TOKEN", Secret: true},
					{Name: "ADDR", Secret: true},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"vault": {Provider: "vault"},
		},
	}

	vars := collectConfigureVars(s)
	for _, v := range vars {
		if !v.secret {
			t.Errorf("unexpected non-secret var %q — all provider vars are secret", v.key)
		}
	}
}

func TestConfigureNonSecretVars_HyphenatedProviderName(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"my-api": {
				Variables: []spec.Input{
					{Name: "HOST", Secret: false},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"t": {Provider: "my-api"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))
	if _, ok := keys["MY_API_HOST"]; !ok {
		t.Error("expected MY_API_HOST (hyphen sanitized to underscore)")
	}
	if _, ok := keys["MY-API_HOST"]; ok {
		t.Error("unsanitized MY-API_HOST must not appear")
	}
}

func TestConfigureNonSecretVars_MultipleProviders(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Variables: []spec.Input{
					{Name: "API_KEY", Secret: true},
					{Name: "ACCOUNT_ID", Secret: false},
				},
			},
			"github": {
				Variables: []spec.Input{
					{Name: "TOKEN", Secret: true},
					{Name: "ORG", Secret: false, Description: "GitHub org"},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"cloudflare": {Provider: "cloudflare"},
			"github":     {Provider: "github"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))

	for _, want := range []string{"CLOUDFLARE_API_KEY", "CLOUDFLARE_ACCOUNT_ID", "GITHUB_TOKEN", "GITHUB_ORG"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("expected %s in configure vars", want)
		}
	}
	for _, bare := range []string{"ACCOUNT_ID", "ORG", "API_KEY", "TOKEN"} {
		if _, ok := keys[bare]; ok {
			t.Errorf("bare %s must not appear in configure vars", bare)
		}
	}
}

func TestConfigureNonSecretVars_ReferencedViaModel(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"custom-llm": {
				Variables: []spec.Input{
					{Name: "ENDPOINT", Secret: false, Description: "LLM endpoint"},
				},
			},
		},
		Models: map[string]spec.Model{
			"main": {Provider: "custom-llm"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))
	v, ok := keys["CUSTOM_LLM_ENDPOINT"]
	if !ok {
		t.Fatal("expected CUSTOM_LLM_ENDPOINT for model-referenced provider")
	}
	if v.description != "LLM endpoint" {
		t.Errorf("description = %q, want %q", v.description, "LLM endpoint")
	}
}

func TestConfigureNonSecretVars_ReferencedViaKnowledge(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"custom-db": {
				Variables: []spec.Input{
					{Name: "HOST", Secret: false},
					{Name: "PORT", Secret: false},
				},
			},
		},
		Knowledge: map[string]spec.Knowledge{
			"docs": {Provider: "custom-db"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))
	for _, want := range []string{"CUSTOM_DB_HOST", "CUSTOM_DB_PORT"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("expected %s in configure vars", want)
		}
	}
}

func TestConfigureNonSecretVars_UnreferencedProviderExcluded(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"used":   {Variables: []spec.Input{{Name: "X", Secret: false}}},
			"unused": {Variables: []spec.Input{{Name: "Y", Secret: false}}},
		},
		Integrations: map[string]spec.Integration{
			"t": {Provider: "used"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))
	if _, ok := keys["USED_X"]; !ok {
		t.Error("expected USED_X")
	}
	if _, ok := keys["UNUSED_Y"]; ok {
		t.Error("UNUSED_Y must not appear — provider is unreferenced")
	}
}

// ─── formatVars ──────────────────────────────────────────────────────────────

func TestFormatVars_Env(t *testing.T) {
	vars := map[string]string{"FOO": "bar", "BAZ": "qux"}
	out, err := formatVars("env", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{`FOO="bar"`, `BAZ="qux"`} {
		if !strings.Contains(out, want) {
			t.Errorf("env output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestFormatVars_JSON(t *testing.T) {
	vars := map[string]string{"KEY": "value"}
	out, err := formatVars("json", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got["KEY"] != "value" {
		t.Errorf("got KEY=%q, want %q", got["KEY"], "value")
	}
}

func TestFormatVars_UnknownFormat(t *testing.T) {
	_, err := formatVars("yaml", map[string]string{"X": "1"})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error should mention the bad format, got: %v", err)
	}
}

// ─── Configure persistence ───────────────────────────────────────────────────

type configPersistenceStore struct {
	Projects map[string]struct {
		Name string            `json:"name"`
		Vars map[string]string `json:"vars"`
	} `json:"projects"`
}

func writePersistenceSpec(t *testing.T, dir, name string) {
	t.Helper()
	content := fmt.Sprintf("spec: \"1.0\"\nname: \"%s\"\nmeta:\n  description: test\nagent:\n  image: nginx:alpine\n", name)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "astropods.yml"), []byte(content), 0o600))
}

func readPersistenceStore(t *testing.T) configPersistenceStore {
	t.Helper()
	path, err := config.ConfigsPath(buildinfo.BinaryName)
	require.NoError(t, err)
	data, err := os.ReadFile(path) //nolint:gosec
	require.NoError(t, err)
	var cfg configPersistenceStore
	require.NoError(t, json.Unmarshal(data, &cfg))
	return cfg
}

func persistenceVarsFor(t *testing.T, cfg configPersistenceStore, projectDir string) map[string]string {
	t.Helper()
	if resolved, err := filepath.EvalSymlinks(projectDir); err == nil {
		if entry, ok := cfg.Projects[resolved]; ok {
			return entry.Vars
		}
	}
	return cfg.Projects[projectDir].Vars
}

func TestConfigurePersistence_VarPersistsValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	writePersistenceSpec(t, projectDir, "@org/my-agent")
	t.Chdir(projectDir)

	require.NoError(t, runConfigureFlags(configureCmd, "", []string{"API_KEY=sk-1"}, nil))
	require.NoError(t, runConfigureFlags(configureCmd, "", []string{"OTHER_KEY=other-1"}, nil))

	vars := persistenceVarsFor(t, readPersistenceStore(t), projectDir)
	require.Equal(t, "sk-1", vars["API_KEY"])
	require.Equal(t, "other-1", vars["OTHER_KEY"], "second --var must not clobber first")
}

func TestConfigurePersistence_VarEmptyValueIsStored(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	writePersistenceSpec(t, projectDir, "@org/my-agent")
	t.Chdir(projectDir)

	require.NoError(t, runConfigureFlags(configureCmd, "", []string{"API_KEY=sk-1"}, nil))
	// --var KEY= stores an empty string, replacing any existing value.
	require.NoError(t, runConfigureFlags(configureCmd, "", []string{"API_KEY="}, nil))

	vars := persistenceVarsFor(t, readPersistenceStore(t), projectDir)
	require.Contains(t, vars, "API_KEY", "--var KEY= must store the key (with empty value)")
	require.Equal(t, "", vars["API_KEY"], "--var KEY= must store empty string")
}

func TestConfigurePersistence_RmVarRemovesKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	writePersistenceSpec(t, projectDir, "@org/my-agent")
	t.Chdir(projectDir)

	require.NoError(t, runConfigureFlags(configureCmd, "", []string{"API_KEY=sk-1", "OTHER=keep"}, nil))
	require.NoError(t, runConfigureFlags(configureCmd, "", nil, []string{"API_KEY"}))

	vars := persistenceVarsFor(t, readPersistenceStore(t), projectDir)
	require.NotContains(t, vars, "API_KEY", "API_KEY should be removed by --rm-var")
	require.Equal(t, "keep", vars["OTHER"], "unrelated key must survive")
}

func TestConfigurePersistence_DoesNotEchoSecret(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	writePersistenceSpec(t, projectDir, "@org/my-agent")
	t.Chdir(projectDir)

	const secret = "sk-DO-NOT-ECHO-12345"
	out := &strings.Builder{}
	configureCmd.SetOut(out)
	t.Cleanup(func() { configureCmd.SetOut(nil) })

	require.NoError(t, runConfigureFlags(configureCmd, "", []string{"API_KEY=" + secret}, nil))
	require.NotContains(t, out.String(), secret, "--var must not echo the secret value")
}

func TestConfigurePersistence_SymlinkPathConsistency(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	t.Setenv("HOME", t.TempDir())
	realDir := t.TempDir()
	writePersistenceSpec(t, realDir, "@org/my-agent")

	linkDir := filepath.Join(t.TempDir(), "aliased")
	require.NoError(t, os.Symlink(realDir, linkDir))

	t.Chdir(realDir)
	require.NoError(t, runConfigureFlags(configureCmd, "", []string{"API_KEY=sk-real"}, nil))

	t.Chdir(linkDir)
	require.NoError(t, runConfigureFlags(configureCmd, "", []string{"EXTRA=added-via-symlink"}, nil))

	cfg := readPersistenceStore(t)
	require.Len(t, cfg.Projects, 1, "symlinked path must resolve to same store entry")
	vars := persistenceVarsFor(t, cfg, realDir)
	require.Equal(t, "sk-real", vars["API_KEY"])
	require.Equal(t, "added-via-symlink", vars["EXTRA"])
}
