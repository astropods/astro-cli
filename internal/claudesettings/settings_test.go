package claudesettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	gatewayURL     = "https://aig.example.com/anthropic"
	gatewayHeaders = "x-bf-direct-key: true\nx-bf-vk: sk-bf-abc"
)

func profile() map[string]string {
	return map[string]string{EnvBaseURL: gatewayURL, EnvCustomHeaders: gatewayHeaders}
}

func writeFile(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	return path
}

func readDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc
}

func TestApplyThenUndo_LeavesEveryOtherSettingAsItWas(t *testing.T) {
	path := writeFile(t, `{
  "model": "opus",
  "permissions": {"allow": ["Bash(git status)"]},
  "env": {"MY_VAR": "keep", "ANTHROPIC_CUSTOM_HEADERS": "x-team: platform"}
}`, 0o640)

	f, err := Load(path)
	require.NoError(t, err)
	change, err := Apply(f, profile(), nil)
	require.NoError(t, err)
	require.NoError(t, f.Save())

	doc := readDoc(t, path)
	env := doc["env"].(map[string]any)
	assert.Equal(t, gatewayURL, env[EnvBaseURL])
	assert.Equal(t, "x-team: platform\nx-bf-direct-key: true\nx-bf-vk: sk-bf-abc", env[EnvCustomHeaders],
		"the user's own header must survive, with ours appended")
	assert.Equal(t, "keep", env["MY_VAR"])
	assert.Equal(t, "opus", doc["model"], "keys outside env are not ours to touch")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "saving must keep the file's permissions")

	f, err = Load(path)
	require.NoError(t, err)
	kept, err := Undo(f, change)
	require.NoError(t, err)
	assert.Empty(t, kept)
	require.NoError(t, f.Save())

	doc = readDoc(t, path)
	env = doc["env"].(map[string]any)
	assert.NotContains(t, env, EnvBaseURL, "a key that was absent before must be removed, not blanked")
	assert.Equal(t, "x-team: platform", env[EnvCustomHeaders], "only our headers are removed")
	assert.Equal(t, "keep", env["MY_VAR"])
	assert.Equal(t, map[string]any{"allow": []any{"Bash(git status)"}}, doc["permissions"])
}

func TestUndo_RestoresAPreviousValueAndRemovesAnEnvBlockItCreated(t *testing.T) {
	path := writeFile(t, `{"theme": "dark"}`, 0o600)
	f, err := Load(path)
	require.NoError(t, err)
	change, err := Apply(f, profile(), nil)
	require.NoError(t, err)
	require.NoError(t, f.Save())

	f, err = Load(path)
	require.NoError(t, err)
	_, err = Undo(f, change)
	require.NoError(t, err)
	require.NoError(t, f.Save())
	assert.Equal(t, map[string]any{"theme": "dark"}, readDoc(t, path), "the file must read exactly as it did before connect")
}

func TestUndo_LeavesAValueSomeoneChangedSinceApply(t *testing.T) {
	path := writeFile(t, `{}`, 0o600)
	f, _ := Load(path)
	change, err := Apply(f, profile(), nil)
	require.NoError(t, err)
	require.NoError(t, f.SetEnv(EnvBaseURL, "https://other-gateway.example.com"))

	kept, err := Undo(f, change)
	require.NoError(t, err)
	assert.Equal(t, []string{EnvBaseURL}, kept, "a value changed after connect belongs to someone else now")
	got, _, _ := f.Env(EnvBaseURL)
	assert.Equal(t, "https://other-gateway.example.com", got)
}

func TestConflicts_FlagsAForeignBaseURLButNotOneWeWrote(t *testing.T) {
	path := writeFile(t, `{"env": {"ANTHROPIC_BASE_URL": "https://llm.internal.example.com"}}`, 0o600)
	f, err := Load(path)
	require.NoError(t, err)

	conflicts, err := Conflicts(f, profile(), nil)
	require.NoError(t, err)
	require.Len(t, conflicts, 1, "replacing another gateway must be the user's decision")
	assert.Equal(t, Conflict{Key: EnvBaseURL, Current: "https://llm.internal.example.com", Proposed: gatewayURL}, conflicts[0])

	path = writeFile(t, `{}`, 0o600)
	f, _ = Load(path)
	first, err := Apply(f, profile(), nil)
	require.NoError(t, err)
	again := map[string]string{EnvBaseURL: gatewayURL + "/v2", EnvCustomHeaders: gatewayHeaders}
	conflicts, err = Conflicts(f, again, first)
	require.NoError(t, err)
	assert.Empty(t, conflicts, "a value connect itself wrote is safe to replace on a re-run")
}

func TestApply_RerunKeepsTheOriginalPreviousValue(t *testing.T) {
	path := writeFile(t, `{"env": {"ANTHROPIC_BASE_URL": "https://original.example.com"}}`, 0o600)
	f, _ := Load(path)
	first, err := Apply(f, profile(), nil)
	require.NoError(t, err)
	second, err := Apply(f, profile(), first)
	require.NoError(t, err)

	_, err = Undo(f, second)
	require.NoError(t, err)
	got, _, _ := f.Env(EnvBaseURL)
	assert.Equal(t, "https://original.example.com", got,
		"after two connects, disconnect must restore what was there before the first, not what the first wrote")
}

func TestApply_ReplacesOurOwnHeadersOnARerun(t *testing.T) {
	f, _ := Load(filepath.Join(t.TempDir(), "settings.json"))
	first, err := Apply(f, profile(), nil)
	require.NoError(t, err)
	_, err = Apply(f, map[string]string{EnvBaseURL: gatewayURL, EnvCustomHeaders: "x-bf-direct-key: true\nx-bf-vk: sk-bf-NEW"}, first)
	require.NoError(t, err)
	got, _, _ := f.Env(EnvCustomHeaders)
	assert.Equal(t, "x-bf-direct-key: true\nx-bf-vk: sk-bf-NEW", got, "a new device key replaces the old header instead of adding a second one")
}

func TestLoad_MissingFileIsEmptyAndSaveCreatesItOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	f, err := Load(path)
	require.NoError(t, err)
	assert.False(t, f.Exists())
	_, err = Apply(f, profile(), nil)
	require.NoError(t, err)
	require.NoError(t, f.Save())
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a new settings file holds a key, so only its owner may read it")
}

func TestLoad_RejectsInvalidJSONInsteadOfOverwritingIt(t *testing.T) {
	path := writeFile(t, `{"env": {`, 0o600)
	_, err := Load(path)
	assert.Error(t, err, "a broken file must stop connect, or saving would erase the user's settings")
}

func TestEnv_RejectsAnEnvBlockThatIsNotAnObject(t *testing.T) {
	f, err := Load(writeFile(t, `{"env": "nope"}`, 0o600))
	require.NoError(t, err)
	_, _, err = f.Env(EnvBaseURL)
	assert.Error(t, err)
	assert.Error(t, f.SetEnv(EnvBaseURL, gatewayURL), "writing into a malformed env block must fail, not replace it")
}

func TestUserSettingsPath_FollowsClaudeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/tmp/claude-alt")
	got, err := UserSettingsPath()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/claude-alt/settings.json", got)

	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", "/home/dev")
	got, err = UserSettingsPath()
	require.NoError(t, err)
	assert.Equal(t, "/home/dev/.claude/settings.json", got)
}

func TestMergeHeaders(t *testing.T) {
	cases := []struct {
		name, existing, ours, want string
	}{
		{"into empty", "", "x-bf-vk: a", "x-bf-vk: a"},
		{"keeps foreign headers first", "x-team: p\nx-env: dev", "x-bf-vk: a", "x-team: p\nx-env: dev\nx-bf-vk: a"},
		{"replaces case-insensitively", "X-BF-VK: old\nx-team: p", "x-bf-vk: new", "x-team: p\nx-bf-vk: new"},
		{"drops blank lines", "x-team: p\n\n", "x-bf-vk: a", "x-team: p\nx-bf-vk: a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, MergeHeaders(tc.existing, tc.ours))
		})
	}
}

func TestHeaderValue(t *testing.T) {
	v, ok := HeaderValue("x-team: p\nX-BF-VK: sk-bf-1", "x-bf-vk")
	assert.True(t, ok)
	assert.Equal(t, "sk-bf-1", v)
	_, ok = HeaderValue("x-team: p", "x-bf-vk")
	assert.False(t, ok)
}
