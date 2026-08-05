package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// writeSpec drops an astropods.yml in a temp dir and returns its path.
func writeSpec(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "astropods.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const agentCoreSpecYAML = `spec: package/v1
name: hello-astro
agent:
  image: hello:latest
  annotations:
    runtime: agentcore
`

// newAgentCoreDeployCmd builds the same command init() registers, so tests
// exercise the real flag set rather than a hand-rolled copy.
func newAgentCoreDeployCmd(out *bytes.Buffer) *cobra.Command {
	c := &cobra.Command{Use: "deploy", Args: cobra.NoArgs, RunE: runAgentCoreOpsDeploy}
	registerAgentCoreDeployFlags(c)
	c.SetOut(out)
	c.SetErr(out)
	return c
}

func TestLoadAgentCoreSpec_RequiresAgentCoreRuntime(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string // substring; empty means success expected
	}{
		{
			name: "agentcore runtime is accepted",
			body: agentCoreSpecYAML,
		},
		{
			name: "no runtime annotation is rejected with the field to set",
			body: "spec: package/v1\nname: hello-astro\nagent:\n  image: hello:latest\n",
			// The operator needs to be told what to add, not just that it is absent.
			wantErr: "set agent.annotations.runtime: agentcore",
		},
		{
			name:    "a different runtime is rejected by name",
			body:    "spec: package/v1\nname: hello-astro\nagent:\n  image: hello:latest\n  annotations:\n    runtime: kubernetes\n",
			wantErr: `selects the "kubernetes" runtime`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, path, err := loadAgentCoreSpec(writeSpec(t, tt.body))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("loadAgentCoreSpec() = %v, want error containing %q", s, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadAgentCoreSpec() error = %v", err)
			}
			if s.Name != "hello-astro" {
				t.Errorf("Name = %q, want hello-astro", s.Name)
			}
			if path == "" {
				t.Error("expected the resolved spec path to be returned")
			}
		})
	}
}

func TestAWSRegionFromEnv(t *testing.T) {
	t.Run("AWS_REGION wins", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
		if got := awsRegionFromEnv(); got != "us-east-1" {
			t.Errorf("awsRegionFromEnv() = %q, want us-east-1", got)
		}
	})
	t.Run("falls back to AWS_DEFAULT_REGION", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
		if got := awsRegionFromEnv(); got != "eu-west-1" {
			t.Errorf("awsRegionFromEnv() = %q, want eu-west-1", got)
		}
	})
	t.Run("empty when neither is set", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		if got := awsRegionFromEnv(); got != "" {
			t.Errorf("awsRegionFromEnv() = %q, want empty", got)
		}
	})
}

// Secret values must never reach stdout: the plan is printed with @SECRET:
// placeholders and the rendered aws commands mask the resolved value.
func TestAgentCoreOpsDeploy_DryRunNeverPrintsSecretValues(t *testing.T) {
	specPath := writeSpec(t, agentCoreSpecYAML+`inputs:
  API_TOKEN:
    name: API_TOKEN
    datatype: string
    secret: true
`)
	var out bytes.Buffer
	c := newAgentCoreDeployCmd(&out)
	c.SetArgs([]string{"-f", specPath, "--dry-run", "--secret", "API_TOKEN=super-secret-value"})
	if err := c.Execute(); err != nil {
		t.Fatalf("dry-run deploy error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, "super-secret-value") {
		t.Errorf("dry-run output leaked the secret value:\n%s", got)
	}
	if !strings.Contains(got, "@SECRET:API_TOKEN") {
		t.Errorf("expected the plan to show the @SECRET: placeholder, got:\n%s", got)
	}
	// Dry-run must render the command it would have run, not silently do nothing.
	if !strings.Contains(got, "create-agent-runtime") {
		t.Errorf("expected the rendered aws command, got:\n%s", got)
	}
}

// A real deploy has to fail before it can write a placeholder or an incomplete
// runtime to AWS. Each guard is asserted separately so a reordering that skips
// one is caught.
func TestAgentCoreOpsDeploy_RealDeployFailsClosed(t *testing.T) {
	specWithSecret := agentCoreSpecYAML + `inputs:
  API_TOKEN:
    name: API_TOKEN
    datatype: string
    secret: true
`
	tests := []struct {
		name    string
		body    string
		args    []string
		role    string
		wantErr string
	}{
		{
			name:    "unresolved secret",
			body:    specWithSecret,
			args:    []string{"--image", "123.dkr.ecr.us-east-1.amazonaws.com/x:1"},
			role:    "arn:aws:iam::123456789012:role/preview-agentcore-exec",
			wantErr: "missing secret value(s) API_TOKEN",
		},
		{
			name:    "missing image",
			body:    agentCoreSpecYAML,
			args:    nil,
			role:    "arn:aws:iam::123456789012:role/preview-agentcore-exec",
			wantErr: "needs --image",
		},
		{
			name:    "missing execution role",
			body:    agentCoreSpecYAML,
			args:    []string{"--image", "123.dkr.ecr.us-east-1.amazonaws.com/x:1"},
			role:    "",
			wantErr: "needs AGENTCORE_EXEC_ROLE_ARN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(execRoleEnv, tt.role)
			var out bytes.Buffer
			c := newAgentCoreDeployCmd(&out)
			c.SetArgs(append([]string{"-f", writeSpec(t, tt.body)}, tt.args...))
			err := c.Execute()
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil and output:\n%s", tt.wantErr, out.String())
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
