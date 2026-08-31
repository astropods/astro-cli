package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const agentCoreSpecYAML = `spec: package/v1
name: hello-astro
agent:
  image: hello:latest
  annotations:
    runtime: agentcore
`

const agentCoreSecretInput = `inputs:
  API_TOKEN:
    name: API_TOKEN
    datatype: string
    secret: true
`

// writeAgentCoreSpec drops an astropods.yml in a temp dir and returns its path.
func writeAgentCoreSpec(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "astropods.yml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// newDeployTestCmd builds a deploy command with the real flag set, so tests
// exercise registered flags rather than a hand-rolled copy.
func newDeployTestCmd(out *bytes.Buffer) *cobra.Command {
	c := &cobra.Command{Use: "deploy", Args: optionalValidAgentName, RunE: runBlueprintDeploy}
	registerDeployFlags(c)
	c.SetOut(out)
	c.SetErr(out)
	return c
}

func TestMaybeAgentCoreDeploy_RoutesOnTheSpecRuntime(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantHandled bool
	}{
		{
			name:        "agentcore runtime is handled here",
			body:        agentCoreSpecYAML,
			wantHandled: true,
		},
		{
			name:        "no runtime annotation falls through to the server path",
			body:        "spec: package/v1\nname: hello-astro\nagent:\n  image: hello:latest\n",
			wantHandled: false,
		},
		{
			name:        "an explicit default runtime falls through to the server path",
			body:        "spec: package/v1\nname: hello-astro\nagent:\n  image: hello:latest\n  annotations:\n    runtime: kubernetes\n",
			wantHandled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			c := newDeployTestCmd(&out)
			require.NoError(t, c.Flags().Set("file", writeAgentCoreSpec(t, tt.body)))
			require.NoError(t, c.Flags().Set("dry-run", "true"))

			handled, err := maybeAgentCoreDeploy(c, nil)

			assert.Equal(t, tt.wantHandled, handled)
			assert.NoError(t, err)
		})
	}
}

// Without a local spec, `ast deploy <name>` must still reach the server path
// rather than being captured by the agentcore branch.
func TestMaybeAgentCoreDeploy_NoLocalSpecFallsThrough(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	handled, err := maybeAgentCoreDeploy(newDeployTestCmd(&out), nil)

	assert.False(t, handled)
	assert.NoError(t, err)
}

func TestMaybeAgentCoreDeploy_RefusesAPositionalName(t *testing.T) {
	var out bytes.Buffer
	c := newDeployTestCmd(&out)
	require.NoError(t, c.Flags().Set("file", writeAgentCoreSpec(t, agentCoreSpecYAML)))
	require.NoError(t, c.Flags().Set("dry-run", "true"))

	handled, err := maybeAgentCoreDeploy(c, []string{"some-other-agent"})

	assert.True(t, handled, "the agentcore branch must own the error, not fall through")
	require.Error(t, err, "a name argument must not be silently ignored")
	assert.Contains(t, err.Error(), "some-other-agent")
	assert.Contains(t, err.Error(), "hello-astro")
	assert.Empty(t, out.String(), "nothing may be deployed when the target is ambiguous")
}

func TestAWSRegionFromEnv(t *testing.T) {
	tests := []struct {
		name          string
		region        string
		defaultRegion string
		want          string
	}{
		{name: "AWS_REGION wins", region: "us-east-1", defaultRegion: "eu-west-1", want: "us-east-1"},
		{name: "falls back to AWS_DEFAULT_REGION", region: "", defaultRegion: "eu-west-1", want: "eu-west-1"},
		{name: "empty when neither is set", region: "", defaultRegion: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AWS_REGION", tt.region)
			t.Setenv("AWS_DEFAULT_REGION", tt.defaultRegion)
			assert.Equal(t, tt.want, awsRegionFromEnv())
		})
	}
}

// Secret values must never reach stdout: the plan is printed with @SECRET:
// placeholders and the rendered aws commands mask the resolved value.
func TestAgentCoreDeploy_DryRunNeverPrintsSecretValues(t *testing.T) {
	var out bytes.Buffer
	c := newDeployTestCmd(&out)
	c.SetArgs([]string{
		"-f", writeAgentCoreSpec(t, agentCoreSpecYAML+agentCoreSecretInput),
		"--dry-run",
		"--secret", "API_TOKEN=super-secret-value",
	})
	require.NoError(t, c.Execute())

	got := out.String()
	assert.NotContains(t, got, "super-secret-value", "dry-run output leaked the secret value")
	assert.Contains(t, got, "@SECRET:API_TOKEN", "plan should show the @SECRET: placeholder")
	// Dry-run must render the command it would have run, not silently do nothing.
	assert.Contains(t, got, "create-agent-runtime")
	assert.Contains(t, got, `"API_TOKEN":"***"`, "the rendered command must mask the value")
}

// A real deploy has to fail before it can write a placeholder or an incomplete
// runtime to AWS. Each guard is asserted separately so a reordering that skips
// one is caught.
func TestAgentCoreDeploy_FailsClosed(t *testing.T) {
	const image = "123.dkr.ecr.us-east-1.amazonaws.com/x:1"
	const role = "arn:aws:iam::123456789012:role/agentcore-exec"

	tests := []struct {
		name    string
		body    string
		args    []string
		role    string
		wantErr error
	}{
		{
			name:    "unresolved secret",
			body:    agentCoreSpecYAML + agentCoreSecretInput,
			args:    []string{"--image", image},
			role:    role,
			wantErr: errAgentCoreMissingSecrets([]string{"API_TOKEN"}),
		},
		{
			name:    "missing image",
			body:    agentCoreSpecYAML,
			role:    role,
			wantErr: errAgentCoreMissingImage(),
		},
		{
			name:    "missing execution role",
			body:    agentCoreSpecYAML,
			args:    []string{"--image", image},
			role:    "",
			wantErr: errAgentCoreMissingExecRole(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(execRoleEnv, tt.role)
			var out bytes.Buffer
			c := newDeployTestCmd(&out)
			c.SetArgs(append([]string{"-f", writeAgentCoreSpec(t, tt.body)}, tt.args...))

			err := c.Execute()

			require.Error(t, err)
			assert.Equal(t, tt.wantErr.Error(), err.Error())
		})
	}
}

// A dropped option deploys a PUBLIC runtime that cannot see its dependencies.
func TestAgentCoreDeploy_VPCOptionsFromEnv(t *testing.T) {
	const body = agentCoreSpecYAML + `knowledge:
  graph:
    provider: neo4j
`
	t.Setenv(subnetsEnv, "subnet-a, subnet-b,")
	t.Setenv(securityGroupsEnv, "sg-a")
	t.Setenv(dependencyHostsEnv, "neo4j.default.svc.cluster.local=neo4j.astro.internal")

	var out bytes.Buffer
	c := newDeployTestCmd(&out)
	c.SetArgs([]string{"-f", writeAgentCoreSpec(t, body), "--dry-run"})
	require.NoError(t, c.Execute())

	got := out.String()
	assert.Contains(t, got, `"networkMode": "VPC"`, "subnets in the environment must flip the plan to VPC")
	assert.Contains(t, got, "subnet-a")
	assert.Contains(t, got, "subnet-b")
	assert.Contains(t, got, "sg-a")
	// The rewrite must reach the created runtime's env, not just the plan.
	assert.Contains(t, got, `--environment-variables '{"ASTRO_RUNTIME":"agentcore","NEO4J_HOST":"neo4j.astro.internal"}'`,
		"the rewritten host must be in the created runtime's env")
}

func TestAgentCoreDeploy_NoNetworkEnvStaysPublic(t *testing.T) {
	t.Setenv(subnetsEnv, "")
	t.Setenv(securityGroupsEnv, "")
	t.Setenv(dependencyHostsEnv, "")

	var out bytes.Buffer
	c := newDeployTestCmd(&out)
	c.SetArgs([]string{"-f", writeAgentCoreSpec(t, agentCoreSpecYAML), "--dry-run"})
	require.NoError(t, c.Execute())

	assert.Contains(t, out.String(), `"networkMode": "PUBLIC"`)
}

// A half-configured network is rejected up front rather than sent to AWS.
func TestAgentCoreDeploy_RejectsIncompleteNetworkEnv(t *testing.T) {
	tests := []struct {
		name    string
		subnets string
		groups  string
		hosts   string
		wantErr string
	}{
		{
			name:    "subnets without security groups",
			subnets: "subnet-a",
			wantErr: "AGENTCORE_SUBNETS is set but AGENTCORE_SECURITY_GROUPS is empty",
		},
		{
			name:    "malformed dependency host",
			hosts:   "neo4j.default.svc.cluster.local",
			wantErr: `invalid host rewrite "neo4j.default.svc.cluster.local", want from=to`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(subnetsEnv, tt.subnets)
			t.Setenv(securityGroupsEnv, tt.groups)
			t.Setenv(dependencyHostsEnv, tt.hosts)

			var out bytes.Buffer
			c := newDeployTestCmd(&out)
			c.SetArgs([]string{"-f", writeAgentCoreSpec(t, agentCoreSpecYAML), "--dry-run"})

			err := c.Execute()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseHostPairs(t *testing.T) {
	got, err := parseHostPairs(" a=1 , b=2 ")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, got)

	got, err = parseHostPairs("")
	require.NoError(t, err)
	assert.Nil(t, got, "no rewrites is not an error")

	_, err = parseHostPairs("a=")
	assert.Error(t, err, "a rewrite with no target is malformed")
}

// A frontend agent cannot run on AgentCore, and the rejection has to name the
// agent so the operator knows which spec to change.
func TestAgentCoreDeploy_RejectsUnsupportedCapability(t *testing.T) {
	body := `spec: package/v1
name: hello-astro
agent:
  image: hello:latest
  annotations:
    runtime: agentcore
  interfaces:
    frontend: true
`
	var out bytes.Buffer
	c := newDeployTestCmd(&out)
	c.SetArgs([]string{"-f", writeAgentCoreSpec(t, body), "--dry-run"})

	err := c.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), `cannot deploy "hello-astro" to AgentCore Runtime:`)
}
