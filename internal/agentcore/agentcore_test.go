package agentcore

import (
	"strings"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseSpec() *spec.AstroSpec {
	return &spec.AstroSpec{
		Name:  "hello-astro",
		Agent: spec.Container{Annotations: map[string]string{"runtime": spec.AgentCoreRuntime}},
	}
}

func TestBuild_CoreShape(t *testing.T) {
	p, err := Build(baseSpec(), Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if p.AgentCore.Protocol != "HTTP" {
		t.Errorf("Protocol = %q, want HTTP", p.AgentCore.Protocol)
	}
	if p.AgentCore.Container.Port != 8080 {
		t.Errorf("Container.Port = %d, want 8080", p.AgentCore.Container.Port)
	}
	// No subnets in Options -> PUBLIC (the no-EKS POC default).
	if p.AgentCore.NetworkMode != "PUBLIC" {
		t.Errorf("NetworkMode = %q, want PUBLIC", p.AgentCore.NetworkMode)
	}
	if p.AgentCore.Env["ASTRO_RUNTIME"] != "agentcore" {
		t.Errorf("ASTRO_RUNTIME = %q, want agentcore", p.AgentCore.Env["ASTRO_RUNTIME"])
	}
	if p.EKS.MessagingEnv["AGENT_TRANSPORT"] != "agentcore" {
		t.Errorf("EKS AGENT_TRANSPORT = %q, want agentcore", p.EKS.MessagingEnv["AGENT_TRANSPORT"])
	}
	// Runtime name is sanitized (hyphen -> underscore).
	if p.AgentCore.AgentRuntimeName != "hello_astro" {
		t.Errorf("AgentRuntimeName = %q, want hello_astro", p.AgentCore.AgentRuntimeName)
	}
	// Every agent's /data disk maps to an S3 Files mount.
	if len(p.AgentCore.FilesystemConfigs) != 1 ||
		p.AgentCore.FilesystemConfigs[0].Type != "s3FilesAccessPoint" ||
		p.AgentCore.FilesystemConfigs[0].MountPath != spec.DefaultAgentVolumeMount {
		t.Errorf("FilesystemConfigs = %+v, want one s3FilesAccessPoint at %s", p.AgentCore.FilesystemConfigs, spec.DefaultAgentVolumeMount)
	}
}

// The plan's NetworkMode is the single source of truth: no subnets -> PUBLIC,
// subnets -> VPC, and the emitted create-agent-runtime command must match it.
func TestBuild_NetworkMode(t *testing.T) {
	t.Run("no subnets -> PUBLIC", func(t *testing.T) {
		p, err := Build(baseSpec(), Options{})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if p.AgentCore.NetworkMode != "PUBLIC" {
			t.Errorf("NetworkMode = %q, want PUBLIC", p.AgentCore.NetworkMode)
		}
		rt := &AWSCLIRuntime{}
		if got := rt.networkJSON(p.AgentCore); got != `{"networkMode":"PUBLIC"}` {
			t.Errorf("networkJSON = %s, want PUBLIC", got)
		}
	})

	t.Run("subnets -> VPC and command matches", func(t *testing.T) {
		p, err := Build(baseSpec(), Options{Subnets: []string{"subnet-1"}, SecurityGroups: []string{"sg-1"}})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if p.AgentCore.NetworkMode != "VPC" {
			t.Errorf("NetworkMode = %q, want VPC", p.AgentCore.NetworkMode)
		}
		rt := &AWSCLIRuntime{}
		got := rt.networkJSON(p.AgentCore)
		if !strings.Contains(got, `"networkMode":"VPC"`) || !strings.Contains(got, "subnet-1") || !strings.Contains(got, "sg-1") {
			t.Errorf("networkJSON = %s, want VPC with subnet-1 / sg-1", got)
		}
	})
}

func TestBuild_Rejections(t *testing.T) {
	t.Run("frontend rejected", func(t *testing.T) {
		s := baseSpec()
		s.Agent.Interfaces = &spec.Interfaces{Frontend: true}
		_, err := Build(s, Options{})
		assertRejection(t, err, "WebIngress")
	})
	t.Run("distributed rejected", func(t *testing.T) {
		s := baseSpec()
		s.Agent.Distributed = true
		_, err := Build(s, Options{})
		assertRejection(t, err, "replicas")
	})
	t.Run("non-arm64 rejected", func(t *testing.T) {
		_, err := Build(baseSpec(), Options{ImageArch: "amd64"})
		assertRejection(t, err, "arm64")
	})
}

func TestAgentCoreCaps(t *testing.T) {
	c := AgentCoreCaps()
	if !c.PersistentDisk || c.Replicas || c.WebIngress {
		t.Errorf("AgentCoreCaps() = %+v, want {PersistentDisk:true, Replicas:false, WebIngress:false}", c)
	}
}

func TestBuild_CloudProviderSecrets(t *testing.T) {
	s := baseSpec()
	s.Models = map[string]spec.Model{"openai": {Provider: "openai"}}
	s.Integrations = map[string]spec.Integration{"github": {Provider: "github"}}
	p, err := Build(s, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if p.AgentCore.Env["OPENAI_API_KEY"] != "@SECRET:OPENAI_API_KEY" {
		t.Errorf("OPENAI_API_KEY = %q, want @SECRET:OPENAI_API_KEY", p.AgentCore.Env["OPENAI_API_KEY"])
	}
	if p.AgentCore.Env["GITHUB_TOKEN"] != "@SECRET:GITHUB_TOKEN" {
		t.Errorf("GITHUB_TOKEN = %q, want @SECRET:GITHUB_TOKEN", p.AgentCore.Env["GITHUB_TOKEN"])
	}
	if got := strings.Join(p.SecretsNeeded, ","); got != "GITHUB_TOKEN,OPENAI_API_KEY" {
		t.Errorf("SecretsNeeded = %q, want GITHUB_TOKEN,OPENAI_API_KEY (sorted)", got)
	}
}

func TestBuild_SelfHostedHostRewrite(t *testing.T) {
	s := baseSpec()
	s.Knowledge = map[string]spec.Knowledge{"graph": {Provider: "neo4j"}}
	p, err := Build(s, Options{
		DependencyHosts: map[string]string{"neo4j.default.svc.cluster.local": "neo4j.astro.internal"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if p.AgentCore.Env["NEO4J_HOST"] != "neo4j.astro.internal" {
		t.Errorf("NEO4J_HOST = %q, want neo4j.astro.internal (rewritten)", p.AgentCore.Env["NEO4J_HOST"])
	}
	if len(p.Rewrites) == 0 {
		t.Error("expected at least one recorded EnvRewrite")
	}
	for k, v := range p.AgentCore.Env {
		if strings.Contains(v, ".svc.cluster.local") {
			t.Errorf("env %q still leaks in-cluster DNS: %q", k, v)
		}
	}
}

func TestBuild_ContainerModeHost(t *testing.T) {
	s := baseSpec()
	s.Knowledge = map[string]spec.Knowledge{"cache": {Container: &spec.ContainerConfig{Image: "redis:7"}}}
	p, err := Build(s, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := p.AgentCore.Env["KNOWLEDGE_CACHE_HOST"]; got != "cache.default.svc.cluster.local" {
		t.Errorf("KNOWLEDGE_CACHE_HOST = %q, want cache.default.svc.cluster.local", got)
	}
}

func TestBuild_InputsBecomeEnv(t *testing.T) {
	s := baseSpec()
	s.Inputs = map[string]spec.Input{
		"owner": {Name: "GITHUB_OWNER", Default: "awslabs"},
		"tok":   {Name: "GITHUB_APP_KEY", Secret: true},
	}
	p, err := Build(s, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if p.AgentCore.Env["GITHUB_OWNER"] != "awslabs" {
		t.Errorf("GITHUB_OWNER = %q, want awslabs", p.AgentCore.Env["GITHUB_OWNER"])
	}
	if p.AgentCore.Env["GITHUB_APP_KEY"] != "@SECRET:GITHUB_APP_KEY" {
		t.Errorf("GITHUB_APP_KEY = %q, want @SECRET:GITHUB_APP_KEY", p.AgentCore.Env["GITHUB_APP_KEY"])
	}
}

func TestResolveSecrets(t *testing.T) {
	t.Run("resolves and injects extras", func(t *testing.T) {
		s := baseSpec()
		s.Models = map[string]spec.Model{"openai": {Provider: "openai"}}
		p, _ := Build(s, Options{})
		unresolved := ResolveSecrets(p, map[string]string{
			"OPENAI_API_KEY":  "sk-123",
			"OPENAI_BASE_URL": "https://gw/v1", // extra, not a placeholder
		})
		if len(unresolved) != 0 {
			t.Errorf("unresolved = %v, want none", unresolved)
		}
		if p.AgentCore.Env["OPENAI_API_KEY"] != "sk-123" {
			t.Errorf("OPENAI_API_KEY = %q, want sk-123", p.AgentCore.Env["OPENAI_API_KEY"])
		}
		if p.AgentCore.Env["OPENAI_BASE_URL"] != "https://gw/v1" {
			t.Errorf("OPENAI_BASE_URL = %q, want injected", p.AgentCore.Env["OPENAI_BASE_URL"])
		}
	})
	// An input with no default is emitted as "", and that empty value must not
	// shadow what the operator supplied, or declaring the input makes it unsettable.
	t.Run("supplied value beats a default-less input", func(t *testing.T) {
		s := baseSpec()
		s.Inputs = map[string]spec.Input{
			"GITHUB_OWNER": {Name: "GITHUB_OWNER", Datatype: "string"},
		}
		p, _ := Build(s, Options{})
		require.Empty(t, p.AgentCore.Env["GITHUB_OWNER"], "precondition: empty before resolve")

		unresolved := ResolveSecrets(p, map[string]string{"GITHUB_OWNER": "awslabs"})

		assert.Empty(t, unresolved)
		assert.Equal(t, "awslabs", p.AgentCore.Env["GITHUB_OWNER"])
	})
	// Only an empty input is treated as absent, so the fix stays narrow.
	t.Run("a declared default is not overwritten", func(t *testing.T) {
		s := baseSpec()
		s.Inputs = map[string]spec.Input{
			"GITHUB_REPO": {Name: "GITHUB_REPO", Datatype: "string", Default: "agents"},
		}
		p, _ := Build(s, Options{})

		ResolveSecrets(p, map[string]string{"GITHUB_REPO": "other"})

		assert.Equal(t, "agents", p.AgentCore.Env["GITHUB_REPO"])
	})
	t.Run("fails closed on missing", func(t *testing.T) {
		s := baseSpec()
		s.Models = map[string]spec.Model{"openai": {Provider: "openai"}}
		p, _ := Build(s, Options{})
		unresolved := ResolveSecrets(p, map[string]string{})
		if len(unresolved) != 1 || unresolved[0] != "OPENAI_API_KEY" {
			t.Errorf("unresolved = %v, want [OPENAI_API_KEY]", unresolved)
		}
		if p.AgentCore.Env["OPENAI_API_KEY"] != "@SECRET:OPENAI_API_KEY" {
			t.Errorf("placeholder should remain unresolved, got %q", p.AgentCore.Env["OPENAI_API_KEY"])
		}
	})
}

// --- deploy layer ---

type fakeRuntime struct {
	existingArn, existingID string
	created, updated        bool
	// statuses are returned in order; the last one repeats.
	statuses    []RuntimeStatus
	statusCalls int
}

func (f *fakeRuntime) Status(string) (RuntimeStatus, error) {
	if len(f.statuses) == 0 {
		return RuntimeStatus{}, nil
	}
	i := min(f.statusCalls, len(f.statuses)-1)
	f.statusCalls++
	return f.statuses[i], nil
}

func (f *fakeRuntime) GetByName(string) (string, string, error) {
	return f.existingArn, f.existingID, nil
}
func (f *fakeRuntime) Create(CreateAgentRuntime, string, string) (string, string, string, error) {
	f.created = true
	return "arn:aws:bedrock-agentcore:us-east-1:acct:runtime/new", "id-new", "1", nil
}
func (f *fakeRuntime) Update(string, CreateAgentRuntime, string) (string, string, error) {
	f.updated = true
	return "arn:aws:bedrock-agentcore:us-east-1:acct:runtime/upd", "2", nil
}

func TestRun_Local(t *testing.T) {
	p, _ := Build(baseSpec(), Options{})
	res, err := Run(p, TargetLocal, nil, "", "")
	if err != nil {
		t.Fatalf("Run(local) error = %v", err)
	}
	if res.Action != "local" {
		t.Errorf("Action = %q, want local", res.Action)
	}
	if res.MessagingEnv["ASTRO_DEPLOY_TARGET"] != "local" {
		t.Errorf("ASTRO_DEPLOY_TARGET = %q, want local", res.MessagingEnv["ASTRO_DEPLOY_TARGET"])
	}
	if res.MessagingEnv["AGENT_RUNTIME_ENDPOINT"] != "http://localhost:8080" {
		t.Errorf("AGENT_RUNTIME_ENDPOINT = %q, want default localhost", res.MessagingEnv["AGENT_RUNTIME_ENDPOINT"])
	}
	if _, ok := res.MessagingEnv["AGENT_RUNTIME_ARN"]; ok {
		t.Error("local mode must not set AGENT_RUNTIME_ARN")
	}
}

func TestRun_AWSCreateAndUpdate(t *testing.T) {
	p, _ := Build(baseSpec(), Options{})

	t.Run("creates when absent", func(t *testing.T) {
		rt := &fakeRuntime{}
		res, err := Run(p, TargetAWS, rt, "us-east-1", "")
		if err != nil {
			t.Fatalf("Run(aws) error = %v", err)
		}
		if !rt.created || rt.updated {
			t.Errorf("expected Create, got created=%v updated=%v", rt.created, rt.updated)
		}
		if res.Action != "created" {
			t.Errorf("Action = %q, want created", res.Action)
		}
		if res.MessagingEnv["ASTRO_DEPLOY_TARGET"] != "aws" || res.MessagingEnv["AWS_REGION"] != "us-east-1" {
			t.Errorf("messaging env = %+v, want aws/us-east-1", res.MessagingEnv)
		}
		if res.MessagingEnv["AGENT_RUNTIME_ARN"] != res.RuntimeArn || res.RuntimeArn == "" {
			t.Errorf("AGENT_RUNTIME_ARN = %q, want = RuntimeArn %q", res.MessagingEnv["AGENT_RUNTIME_ARN"], res.RuntimeArn)
		}
	})

	t.Run("updates when present", func(t *testing.T) {
		rt := &fakeRuntime{existingArn: "arn:old", existingID: "id-old"}
		res, err := Run(p, TargetAWS, rt, "us-east-1", "")
		if err != nil {
			t.Fatalf("Run(aws) error = %v", err)
		}
		if rt.created || !rt.updated {
			t.Errorf("expected Update, got created=%v updated=%v", rt.created, rt.updated)
		}
		if res.Action != "updated" {
			t.Errorf("Action = %q, want updated", res.Action)
		}
	})
}

func TestEnvExportsSorted(t *testing.T) {
	r := &Result{MessagingEnv: map[string]string{"B": "2", "A": "1"}}
	if got := r.EnvExports(); got != "A=1\nB=2\n" {
		t.Errorf("EnvExports() = %q, want A=1\\nB=2\\n", got)
	}
}

func TestMaskEnvArg(t *testing.T) {
	a := &AWSCLIRuntime{SecretKeys: map[string]bool{"OPENAI_API_KEY": true}}
	args := []string{"--environment-variables", envJSON(map[string]string{
		"OPENAI_API_KEY": "sk-leak",
		"NEO4J_HOST":     "neo4j.astro.internal",
	})}
	masked := strings.Join(a.maskEnvArg(args), " ")
	if strings.Contains(masked, "sk-leak") {
		t.Errorf("masked output leaks secret: %q", masked)
	}
	if !strings.Contains(masked, "***") {
		t.Errorf("masked output missing ***: %q", masked)
	}
	if !strings.Contains(masked, "neo4j.astro.internal") {
		t.Errorf("masked output should keep non-secret env: %q", masked)
	}
}

func assertRejection(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a rejection error, got nil")
	}
	rej, ok := err.(*RejectionError)
	if !ok {
		t.Fatalf("error type = %T, want *RejectionError", err)
	}
	if !strings.Contains(rej.Reason, wantSubstr) {
		t.Errorf("rejection %q does not mention %q", rej.Reason, wantSubstr)
	}
}
