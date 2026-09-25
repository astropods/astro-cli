package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envTestServer fakes the blueprint, environment, vault, and deployment
// routes for account testaccount and blueprint mybot.
type envTestServer struct {
	visibility   string
	environments []map[string]any
	deployments  []map[string]any
	accountVars  []map[string]any
	envVars      map[string][]map[string]any
	status       map[string]int
	requests     []string
	bodies       map[string]map[string]any
}

func newEnvTestServer() *envTestServer {
	return &envTestServer{
		visibility: "private",
		environments: []map[string]any{
			{"id": "env-main", "name": "main", "account_name": "testaccount", "variables_available": true, "deployment_id": "dep-1"},
			{"id": "env-staging", "name": "staging", "account_name": "testaccount", "variables_available": true},
			{"id": "env-other", "name": "main", "account_name": "someone-else", "variables_available": false},
		},
		deployments: []map[string]any{
			{"id": "dep-1", "name": "mybot", "display_name": "Mybot", "status": "active", "environment_id": "env-main", "environment_name": "main"},
		},
		accountVars: []map[string]any{{"name": "API_KEY", "secret": true}},
		envVars: map[string][]map[string]any{
			"env-main":    {{"name": "API_KEY", "secret": true}, {"name": "REGION", "secret": false, "value": "eu"}},
			"env-staging": {},
		},
		status: map[string]int{},
		bodies: map[string]map[string]any{},
	}
}

func (s *envTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.Path
	s.requests = append(s.requests, key)
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		s.bodies[key] = body
	}
	if code, ok := s.status[key]; ok {
		jsonHandler(code, map[string]any{"error": "forced", "error_code": s.errorCode(code)})(w, r)
		return
	}
	switch key {
	case "GET /api/v1/agents/testaccount/mybot":
		jsonHandler(http.StatusOK, map[string]any{"name": "mybot", "visibility": s.visibility})(w, r)
	case "GET /api/v1/agents/testaccount/mybot/environments":
		jsonHandler(http.StatusOK, map[string]any{"environments": s.environments})(w, r)
	case "POST /api/v1/agents/testaccount/mybot/environments":
		jsonHandler(http.StatusCreated, map[string]any{"id": "env-new", "name": s.bodies[key]["name"], "account_name": "testaccount"})(w, r)
	case "PATCH /api/v1/accounts/testaccount/environments/env-staging":
		jsonHandler(http.StatusOK, map[string]any{"id": "env-staging", "name": s.bodies[key]["name"]})(w, r)
	case "DELETE /api/v1/accounts/testaccount/environments/env-staging":
		jsonHandler(http.StatusOK, map[string]any{"message": "environment deleted"})(w, r)
	case "GET /api/v1/accounts/testaccount/variables":
		jsonHandler(http.StatusOK, map[string]any{"variables": s.accountVars})(w, r)
	case "GET /api/v1/accounts/testaccount/environments/env-main/variables":
		jsonHandler(http.StatusOK, map[string]any{"variables": s.envVars["env-main"]})(w, r)
	case "GET /api/v1/accounts/testaccount/environments/env-staging/variables":
		jsonHandler(http.StatusOK, map[string]any{"variables": s.envVars["env-staging"]})(w, r)
	case "POST /api/v1/accounts/testaccount/environments/env-staging/variables":
		jsonHandler(http.StatusCreated, map[string]any{"results": []any{map[string]any{"name": "DB_URL", "status": "created"}}})(w, r)
	case "GET /api/v1/deployments":
		jsonHandler(http.StatusOK, map[string]any{"deployments": s.deployments, "count": len(s.deployments)})(w, r)
	case "GET /api/v1/deployments/dep-1":
		jsonHandler(http.StatusOK, map[string]any{"deployment": s.deployments[0]})(w, r)
	case "POST /api/v1/agents/testaccount/mybot/deployment-template":
		jsonHandler(http.StatusOK, map[string]any{
			"template":   json.RawMessage(`{"spec":"deployment/v1","target":{}}`),
			"validation": map[string]any{"valid": true},
			"signature":  "sig",
		})(w, r)
	case "POST /api/v1/deploy":
		jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-2", "environment_id": "env-staging", "environment_name": "staging"})(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *envTestServer) errorCode(code int) string {
	if code == http.StatusConflict {
		return errCodeEnvironmentTaken
	}
	return ""
}

func (s *envTestServer) called(key string) bool {
	for _, r := range s.requests {
		if r == key {
			return true
		}
	}
	return false
}

func setupEnvTest(t *testing.T) *envTestServer {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))
	fake := newEnvTestServer()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	for _, override := range []*string{&environmentsServerURLOverride, &secretsServerURLOverride, &agentServerURLOverride, &blueprintServerURLOverride} {
		*override = srv.URL
	}
	t.Cleanup(func() {
		environmentsServerURLOverride, secretsServerURLOverride, agentServerURLOverride, blueprintServerURLOverride = "", "", "", ""
	})
	return fake
}

func setFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	cmd.InheritedFlags()
	require.NoError(t, cmd.Flags().Set(name, value))
	t.Cleanup(func() { _ = cmd.Flags().Set(name, "") })
}

func runWithOutput(t *testing.T, cmd *cobra.Command, run func(*cobra.Command, []string) error, args ...string) (string, error) {
	t.Helper()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetContext(context.Background())
	err := run(cmd, args)
	return stripANSI(buf.String()), err
}

func TestListBlueprintEnvironments(t *testing.T) {
	t.Run("keeps only the active account's environments", func(t *testing.T) {
		setupEnvTest(t)
		at := AccountToken{Account: "testaccount"}
		envs, err := listBlueprintEnvironments(context.Background(), at, "mybot", false)
		require.NoError(t, err)
		require.Len(t, envs, 2, "the environment in someone-else must be filtered out")
		assert.Equal(t, "main", envs[0].Name)
		assert.Equal(t, "staging", envs[1].Name)
	})

	t.Run("refuses a public blueprint", func(t *testing.T) {
		fake := setupEnvTest(t)
		fake.visibility = "public"
		_, err := listBlueprintEnvironments(context.Background(), AccountToken{Account: "testaccount"}, "mybot", false)
		require.EqualError(t, err, errEnvironmentsUnavailable("mybot").Error())
	})

	t.Run("reports an unknown blueprint", func(t *testing.T) {
		setupEnvTest(t)
		_, err := listBlueprintEnvironments(context.Background(), AccountToken{Account: "testaccount"}, "ghost", false)
		require.EqualError(t, err, errBlueprintNotFound("ghost", "testaccount").Error())
	})

	t.Run("names the available environments for an unknown one", func(t *testing.T) {
		setupEnvTest(t)
		_, err := findBlueprintEnvironment(context.Background(), AccountToken{Account: "testaccount"}, "mybot", "prod", false)
		require.EqualError(t, err, errEnvironmentNotFound("prod", "mybot", []string{"main", "staging"}).Error())
	})
}

func TestEnvList(t *testing.T) {
	setupEnvTest(t)
	setFlag(t, envListCmd, "blueprint", "mybot")

	out, err := runWithOutput(t, envListCmd, runEnvList)
	require.NoError(t, err)
	assert.Regexp(t, `main\s+Mybot\s+active\s+1 variable, 1 secret`, out)
	assert.Regexp(t, `staging\s+—\s+empty\s+none`, out)
}

func TestEnvCreate(t *testing.T) {
	t.Run("creates and suggests a deploy", func(t *testing.T) {
		fake := setupEnvTest(t)
		setFlag(t, envCreateCmd, "blueprint", "mybot")
		out, err := runWithOutput(t, envCreateCmd, runEnvCreate, "qa")
		require.NoError(t, err)
		assert.Equal(t, "qa", fake.bodies["POST /api/v1/agents/testaccount/mybot/environments"]["name"])
		assert.Contains(t, out, msgEnvironmentCreated("qa", "mybot"))
	})

	t.Run("reports a taken name", func(t *testing.T) {
		fake := setupEnvTest(t)
		fake.status["POST /api/v1/agents/testaccount/mybot/environments"] = http.StatusConflict
		setFlag(t, envCreateCmd, "blueprint", "mybot")
		_, err := runWithOutput(t, envCreateCmd, runEnvCreate, "main")
		require.EqualError(t, err, errEnvironmentNameTaken("main", "mybot").Error())
	})
}

func TestEnvRename(t *testing.T) {
	fake := setupEnvTest(t)
	setFlag(t, envRenameCmd, "blueprint", "mybot")

	out, err := runWithOutput(t, envRenameCmd, runEnvRename, "staging", "qa")
	require.NoError(t, err)
	assert.Equal(t, "qa", fake.bodies["PATCH /api/v1/accounts/testaccount/environments/env-staging"]["name"])
	assert.Contains(t, out, msgEnvironmentRenamed("staging", "qa"))
}

func TestEnvDelete(t *testing.T) {
	t.Run("refuses an environment with an agent before calling the server", func(t *testing.T) {
		fake := setupEnvTest(t)
		setFlag(t, envDeleteCmd, "blueprint", "mybot")
		setFlag(t, envDeleteCmd, "confirm", "main")
		_, err := runWithOutput(t, envDeleteCmd, runEnvDelete, "main")
		require.EqualError(t, err, errEnvironmentHasAgent("main", "mybot").Error())
		assert.False(t, fake.called("DELETE /api/v1/accounts/testaccount/environments/env-main"))
	})

	t.Run("deletes an empty environment when confirmed", func(t *testing.T) {
		fake := setupEnvTest(t)
		setFlag(t, envDeleteCmd, "blueprint", "mybot")
		setFlag(t, envDeleteCmd, "confirm", "staging")
		out, err := runWithOutput(t, envDeleteCmd, runEnvDelete, "staging")
		require.NoError(t, err)
		assert.True(t, fake.called("DELETE /api/v1/accounts/testaccount/environments/env-staging"))
		assert.Contains(t, out, msgEnvironmentDeleted("staging"))
	})
}

func TestSecretsInAnEnvironment(t *testing.T) {
	t.Run("lists the environment's values and marks account overrides", func(t *testing.T) {
		fake := setupEnvTest(t)
		setFlag(t, secretListCmd, "blueprint", "mybot")
		setFlag(t, secretListCmd, "env", "main")
		out, err := runWithOutput(t, secretListCmd, runSecretList)
		require.NoError(t, err)
		assert.True(t, fake.called("GET /api/v1/accounts/testaccount/environments/env-main/variables"))
		assert.Regexp(t, `API_KEY\s+secret\s+`+msgOverridesAccountValue(), out)
		assert.NotRegexp(t, `REGION.*`+msgOverridesAccountValue(), out, "REGION has no account value to override")
	})

	t.Run("creates in the environment", func(t *testing.T) {
		fake := setupEnvTest(t)
		fake.status["GET /api/v1/accounts/testaccount/environments/env-staging/variables/DB_URL"] = http.StatusNotFound
		setFlag(t, secretCreateCmd, "blueprint", "mybot")
		setFlag(t, secretCreateCmd, "env", "staging")
		setFlag(t, secretCreateCmd, "value", "postgres://x")
		out, err := runWithOutput(t, secretCreateCmd, runSecretCreate, "DB_URL")
		require.NoError(t, err)
		assert.True(t, fake.called("POST /api/v1/accounts/testaccount/environments/env-staging/variables"))
		assert.Contains(t, out, `Created secret "DB_URL" in environment staging`)
	})

	cases := []struct {
		name, env, blueprint string
		want                 error
	}{
		{name: "env outside a project without blueprint", env: "main", want: errBlueprintRequired()},
		{name: "blueprint without env", blueprint: "mybot", want: errBlueprintNeedsEnvironment()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupEnvTest(t)
			t.Chdir(t.TempDir())
			setFlag(t, secretListCmd, "env", tc.env)
			setFlag(t, secretListCmd, "blueprint", tc.blueprint)
			_, err := runWithOutput(t, secretListCmd, runSecretList)
			require.EqualError(t, err, tc.want.Error())
		})
	}
}

func TestResolveAgentTargetByEnvironment(t *testing.T) {
	t.Run("returns the environment's agent", func(t *testing.T) {
		setupEnvTest(t)
		cmd := &cobra.Command{}
		registerAgentTargetFlags(cmd)
		cmd.SetContext(context.Background())
		setFlag(t, cmd, "blueprint", "mybot")
		setFlag(t, cmd, "env", "main")
		dep, err := resolveAgentTarget(cmd, AccountToken{Account: "testaccount"}, false)
		require.NoError(t, err)
		assert.Equal(t, "dep-1", dep.ID)
		assert.Equal(t, "main", dep.EnvironmentName)
	})

	t.Run("reports an environment with no agent", func(t *testing.T) {
		setupEnvTest(t)
		cmd := &cobra.Command{}
		registerAgentTargetFlags(cmd)
		cmd.SetContext(context.Background())
		setFlag(t, cmd, "blueprint", "mybot")
		setFlag(t, cmd, "env", "staging")
		_, err := resolveAgentTarget(cmd, AccountToken{Account: "testaccount"}, false)
		require.EqualError(t, err, errEnvironmentHasNoAgent("staging", "mybot").Error())
	})
}

func TestFindDeploymentByTargetWithSeveralAgentsOfOneBlueprint(t *testing.T) {
	fake := setupEnvTest(t)
	fake.deployments = append(fake.deployments, map[string]any{
		"id": "dep-2", "name": "mybot", "display_name": "Mybot QA", "status": "active", "environment_name": "qa",
	})
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	at := AccountToken{Account: "testaccount"}

	dep, err := findDeploymentByTarget(cmd, "Mybot QA", at, false)
	require.NoError(t, err, "an exact display name is unique, so it still resolves")
	assert.Equal(t, "dep-2", dep.ID)

	_, err = findDeploymentByTarget(cmd, "mybot", at, false)
	require.EqualError(t, err, errAgentTargetAmbiguous("mybot", []string{
		"Mybot  --id dep-1  --env main",
		"Mybot QA  --id dep-2  --env qa",
	}).Error())
}

func TestDeployIntoAnEnvironment(t *testing.T) {
	t.Run("sends the environment id in the deploy target", func(t *testing.T) {
		fake := setupEnvTest(t)
		setDeployFlag(t, "env", "staging")
		out, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy, "mybot")
		require.NoError(t, err)
		target, _ := fake.bodies["POST /api/v1/deploy"]["target"].(map[string]any)
		assert.Equal(t, "env-staging", target["environment_id"])
		assert.Contains(t, out, msgDeployed("staging"), "the result names the environment the server bound")
	})

	t.Run("names the environment the server chose when none was asked for", func(t *testing.T) {
		setupEnvTest(t)
		out, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy, "mybot")
		require.NoError(t, err)
		assert.Contains(t, out, msgDeployed("staging"))
	})

	t.Run("refuses an environment that has an agent", func(t *testing.T) {
		fake := setupEnvTest(t)
		setDeployFlag(t, "env", "main")
		_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy, "mybot")
		require.EqualError(t, err, errEnvironmentOccupied("main", "mybot").Error())
		assert.False(t, fake.called("POST /api/v1/deploy"))
	})

	t.Run("maps a race lost to another deploy", func(t *testing.T) {
		fake := setupEnvTest(t)
		fake.status["POST /api/v1/deploy"] = http.StatusConflict
		setDeployFlag(t, "env", "staging")
		_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy, "mybot")
		require.EqualError(t, err, errEnvironmentOccupied("staging", "mybot").Error())
	})
}

func TestBlueprintDefaultsToTheProjectSpec(t *testing.T) {
	enterProject := func(t *testing.T, name string) {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "astropods.yml"), []byte("name: \""+name+"\"\n"), 0o600))
		t.Chdir(dir)
	}

	t.Run("env list reads the blueprint from astropods.yml", func(t *testing.T) {
		fake := setupEnvTest(t)
		enterProject(t, "@testaccount/mybot")
		_, err := runWithOutput(t, envListCmd, runEnvList)
		require.NoError(t, err)
		assert.True(t, fake.called("GET /api/v1/agents/testaccount/mybot/environments"))
	})

	t.Run("an explicit --blueprint wins over the project", func(t *testing.T) {
		fake := setupEnvTest(t)
		enterProject(t, "otherbot")
		setFlag(t, envListCmd, "blueprint", "mybot")
		_, err := runWithOutput(t, envListCmd, runEnvList)
		require.NoError(t, err)
		assert.True(t, fake.called("GET /api/v1/agents/testaccount/mybot/environments"))
	})

	t.Run("agent targeting takes just --env", func(t *testing.T) {
		setupEnvTest(t)
		enterProject(t, "mybot")
		cmd := &cobra.Command{}
		registerAgentTargetFlags(cmd)
		cmd.SetContext(context.Background())
		setFlag(t, cmd, "env", "main")
		dep, err := resolveAgentTarget(cmd, AccountToken{Account: "testaccount"}, false)
		require.NoError(t, err)
		assert.Equal(t, "dep-1", dep.ID)
	})

	t.Run("deploy takes no blueprint argument", func(t *testing.T) {
		fake := setupEnvTest(t)
		enterProject(t, "mybot")
		setDeployFlag(t, "env", "staging")
		_, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)
		require.NoError(t, err)
		assert.True(t, fake.called("POST /api/v1/agents/testaccount/mybot/deployment-template"))
	})

	t.Run("outside a project the blueprint is required", func(t *testing.T) {
		setupEnvTest(t)
		t.Chdir(t.TempDir())
		_, err := runWithOutput(t, envListCmd, runEnvList)
		require.EqualError(t, err, errBlueprintRequired().Error())
	})
}

func TestDeploymentStatusColor(t *testing.T) {
	cases := []struct {
		status string
		want   *color.Color
	}{
		{"Running", color.New(color.FgGreen)},
		{"active", color.New(color.FgGreen)},
		{"error", color.New(color.FgRed)},
		{"failed", color.New(color.FgRed)},
		{"pending", color.New(color.Faint)},
		{"empty", color.New(color.Faint)},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			assert.True(t, deploymentStatusColor(tc.status).Equals(tc.want), "the list returns display labels and history returns stored statuses; both must color")
		})
	}
}
