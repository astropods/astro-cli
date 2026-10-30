package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deployFileTemplatePath = "POST /api/v1/agents/testaccount/mybot/deployment-template"

func writeDeployFileProject(t *testing.T, deployFile string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "astropods.yml"), []byte("name: \"@testaccount/mybot\"\n"), 0o600))
	path := filepath.Join(dir, "astropods.deploy.yml")
	require.NoError(t, os.WriteFile(path, []byte(deployFile), 0o600))
	return path
}

func setDeployFlags(t *testing.T, pairs ...string) {
	t.Helper()
	setDeployFlag(t, pairs[0], pairs[1])
	for i := 2; i < len(pairs); i += 2 {
		require.NoError(t, blueprintDeployCmd.Flags().Set(pairs[i], pairs[i+1]))
	}
}

func TestDeployFileDeploysEveryEnvironment(t *testing.T) {
	fake := setupEnvTest(t)
	setDeployFlags(t, "file", writeDeployFileProject(t, `
spec: deploy/v1
environments:
  main:
    interfaces: [web]
    public: { web: false }
    variables:
      REGION: eu
      API_KEY: { ref: API_KEY }
    models: { default: claude-sonnet-5-5 }
    knowledge: { docs: kb_docs }
    resources:
      agent: { cpu: "1", memory: 2Gi }
  staging: {}
  qa: {}
`))

	_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)
	require.NoError(t, err)

	templates := fake.bodyLog[deployFileTemplatePath]
	deploys := fake.bodyLog["POST /api/v1/deploy"]
	require.Len(t, templates, 3, "one template per environment, in sorted order: main, qa, staging")
	require.Len(t, deploys, 3)

	main := templates[0]
	assert.Equal(t, "dep-1", main["deployment_id"], "main holds dep-1, so it is a redeploy")
	assert.Equal(t, map[string]any{"adapters": []any{"web"}, "auth": map[string]any{"web": map[string]any{"public": false}}}, main["interfaces"],
		"public: false must be sent, or a stored true survives")
	assert.Equal(t, map[string]any{"REGION": map[string]any{"value": "eu"}, "API_KEY": map[string]any{"ref": "API_KEY"}}, main["variables"])
	assert.Equal(t, map[string]any{"default": "claude-sonnet-5-5"}, main["models"])
	assert.Equal(t, map[string]any{"knowledge": map[string]any{"docs": "kb_docs"}}, main["bindings"])
	assert.Equal(t, map[string]any{"agent": map[string]any{"compute": map[string]any{"cpu": "1", "memory": "2Gi"}}}, main["provisioning"])
	assert.Equal(t, map[string]any{"display_name": "Mybot"}, deploys[0]["target"], "a redeploy keeps the agent's name")

	assert.Equal(t, "qa", fake.bodies["POST /api/v1/agents/testaccount/mybot/environments"]["name"], "a missing environment is created")
	assert.Nil(t, templates[1]["deployment_id"])
	assert.Equal(t, map[string]any{"display_name": "mybot-qa", "environment_id": "env-new"}, deploys[1]["target"])

	assert.Nil(t, templates[2]["deployment_id"])
	assert.Nil(t, templates[2]["interfaces"], "an environment that names no interfaces must not send any, or the stored adapters are replaced")
	assert.Equal(t, map[string]any{"display_name": "mybot-staging", "environment_id": "env-staging"}, deploys[2]["target"])
}

func TestDeployFileDeploysOnlyTheNamedEnvironments(t *testing.T) {
	fake := setupEnvTest(t)
	setDeployFlags(t, "file", writeDeployFileProject(t, "spec: deploy/v1\nenvironments: {main: {}, staging: {}, qa: {}}\n"),
		"env", "staging", "env", "qa")

	_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)
	require.NoError(t, err)
	assert.Len(t, fake.bodyLog["POST /api/v1/deploy"], 2)
	assert.False(t, fake.called("GET /api/v1/deployments/dep-1"), "main was not named, so its agent is not touched")
}

func TestDeployFileDryRunCreatesAndDeploysNothing(t *testing.T) {
	fake := setupEnvTest(t)
	setDeployFlags(t, "file", writeDeployFileProject(t, "spec: deploy/v1\nenvironments: {qa: {}}\n"), "dry-run", "true")

	_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)
	require.NoError(t, err)
	assert.True(t, fake.called(deployFileTemplatePath), "a dry run still validates the template")
	assert.False(t, fake.called("POST /api/v1/agents/testaccount/mybot/environments"))
	assert.False(t, fake.called("POST /api/v1/deploy"))
}

func TestDeployFileKeepsGoingPastAFailedEnvironment(t *testing.T) {
	fake := setupEnvTest(t)
	fake.templateVars = map[string]any{"API_KEY": map[string]any{"secret": true}}
	setDeployFlags(t, "file", writeDeployFileProject(t, `
spec: deploy/v1
environments:
  main:
    variables: { API_KEY: sk-live }
  staging:
    variables: { API_KEY: { ref: API_KEY } }
`))

	out, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)
	require.EqualError(t, err, errDeployFileFailed([]string{"main"}, 2).Error())
	assert.Contains(t, out, msgDeployFileEnvironmentFailed("main", errDeployFileSecretLiteral("API_KEY")))
	deploys := fake.bodyLog["POST /api/v1/deploy"]
	require.Len(t, deploys, 1, "main stops before /deploy, and staging still deploys")
	assert.Equal(t, "env-staging", deploys[0]["target"].(map[string]any)["environment_id"])
}

func TestDeployFileRefusals(t *testing.T) {
	const twoEnvs = "spec: deploy/v1\nenvironments: {main: {}, staging: {}}\n"

	t.Run("a blueprint name", func(t *testing.T) {
		setupEnvTest(t)
		setDeployFlags(t, "file", writeDeployFileProject(t, twoEnvs))
		_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy, "mybot")
		require.EqualError(t, err, errDeployFileTakesNoName("mybot").Error())
	})

	t.Run("flags the file sets", func(t *testing.T) {
		setupEnvTest(t)
		setDeployFlags(t, "file", writeDeployFileProject(t, twoEnvs), "var", "X=1", "cluster", "c1")
		_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)
		require.EqualError(t, err, errDeployFileFlags([]string{"--var", "--cluster"}).Error())
	})

	t.Run("an environment the file does not declare", func(t *testing.T) {
		fake := setupEnvTest(t)
		path := writeDeployFileProject(t, twoEnvs)
		setDeployFlags(t, "file", path, "env", "qa")
		_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)
		require.EqualError(t, err, errDeployFileUnknownEnv("qa", path, []string{"main", "staging"}).Error())
		assert.Empty(t, fake.requests, "nothing is called before the file checks out")
	})

	t.Run("no astropods.yml beside the file", func(t *testing.T) {
		setupEnvTest(t)
		dir := t.TempDir()
		path := filepath.Join(dir, "astropods.deploy.yml")
		require.NoError(t, os.WriteFile(path, []byte(twoEnvs), 0o600))
		setDeployFlags(t, "file", path)
		_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)
		require.EqualError(t, err, errDeployFileNoSpec(dir).Error())
	})

	t.Run("a repeated --env without -f", func(t *testing.T) {
		setupEnvTest(t)
		setDeployFlags(t, "env", "main", "env", "staging")
		_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy, "mybot")
		require.EqualError(t, err, errEnvRepeatNeedsDeployFile().Error())
	})
}
