package compose

import (
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// helper to dereference a *string from env maps, returning "" if nil.
func envVal(env map[string]*string, key string) string {
	if v, ok := env[key]; ok && v != nil {
		return *v
	}
	return ""
}

func TestBuildProject_MinimalSpec(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	if project.Name != "my-agent" {
		t.Errorf("Name = %q, want %q", project.Name, "my-agent")
	}

	if _, ok := project.Services["agent"]; !ok {
		t.Error("missing agent service")
	}

	agent := project.Services["agent"]
	if agent.Image != "agent:latest" {
		t.Errorf("agent.Image = %q, want %q", agent.Image, "agent:latest")
	}
}

func TestBuildProject_SlackInterface(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack"},
				},
			},
		},
	}

	envVars := map[string]string{
		"SLACK_BOT_TOKEN": "xoxb-test",
		"SLACK_APP_TOKEN": "xapp-test",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	// Should create messaging sidecar
	messaging, ok := project.Services["astro-messaging"]
	if !ok {
		t.Fatal("missing astro-messaging service")
	}

	// Messaging env should have slack tokens
	if envVal(messaging.Environment, "SLACK_BOT_TOKEN") != "xoxb-test" {
		t.Errorf("SLACK_BOT_TOKEN = %q, want %q", envVal(messaging.Environment, "SLACK_BOT_TOKEN"), "xoxb-test")
	}
	if envVal(messaging.Environment, "SLACK_ENABLED") != "true" {
		t.Error("SLACK_ENABLED should be true")
	}

	// Should NOT have playground (only slack, no web)
	if _, ok := project.Services["playground"]; ok {
		t.Error("playground should not exist for slack-only interface")
	}

	// Agent should have GRPC_SERVER_ADDR
	agent := project.Services["agent"]
	if envVal(agent.Environment, "GRPC_SERVER_ADDR") != "astro-messaging:9090" {
		t.Error("agent should have GRPC_SERVER_ADDR")
	}
}

func TestBuildProject_WebInterface(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"web"},
				},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	// Should create messaging sidecar and playground
	if _, ok := project.Services["astro-messaging"]; !ok {
		t.Error("missing astro-messaging service")
	}
	if _, ok := project.Services["playground"]; !ok {
		t.Error("missing playground service for web interface")
	}

	// Messaging should have WEB_ENABLED
	messaging := project.Services["astro-messaging"]
	if envVal(messaging.Environment, "WEB_ENABLED") != "true" {
		t.Error("WEB_ENABLED should be true")
	}

	// Messaging should expose both gRPC (9090) and HTTP (3100) ports
	if len(messaging.Ports) != 2 {
		t.Errorf("messaging ports = %d, want 2 (grpc + http)", len(messaging.Ports))
	}
}

func TestBuildProject_CloudProviderCredentials(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Models: map[string]spec.Model{
			"anthropic": {Provider: "anthropic"},
		},
		Tools: map[string]spec.Tool{
			"github": {Provider: "github"},
		},
	}

	envVars := map[string]string{
		"ANTHROPIC_API_KEY": "sk-test",
		"GITHUB_TOKEN":      "ghp-test",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "ANTHROPIC_API_KEY") != "sk-test" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", envVal(agent.Environment, "ANTHROPIC_API_KEY"), "sk-test")
	}
	if envVal(agent.Environment, "GITHUB_TOKEN") != "ghp-test" {
		t.Errorf("GITHUB_TOKEN = %q, want %q", envVal(agent.Environment, "GITHUB_TOKEN"), "ghp-test")
	}

	// Cloud providers should NOT create services
	if _, ok := project.Services["model-anthropic"]; ok {
		t.Error("cloud model provider should not create a service")
	}
	if _, ok := project.Services["tool-github"]; ok {
		t.Error("cloud tool provider should not create a service")
	}
}

func TestBuildProject_CustomProviderCredentials(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Providers: map[string]spec.CustomProvider{
			"my-service": {
				Scope: []string{"tools"},
				Variables: []spec.Input{
					{Name: "MY_SERVICE_API_KEY", Datatype: "string", Secret: true},
					{Name: "MY_SERVICE_SECRET", Datatype: "string", Secret: true},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"jira": {Provider: "my-service"},
		},
	}

	envVars := map[string]string{
		"MY_SERVICE_API_KEY": "key1",
		"MY_SERVICE_SECRET":  "s3cret",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "MY_SERVICE_API_KEY") != "key1" {
		t.Errorf("MY_SERVICE_API_KEY = %q, want %q", envVal(agent.Environment, "MY_SERVICE_API_KEY"), "key1")
	}
	if envVal(agent.Environment, "MY_SERVICE_SECRET") != "s3cret" {
		t.Errorf("MY_SERVICE_SECRET = %q, want %q", envVal(agent.Environment, "MY_SERVICE_SECRET"), "s3cret")
	}
}

func TestBuildProject_CustomProviderMissingEnvVar(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Providers: map[string]spec.CustomProvider{
			"my-service": {
				Scope: []string{"tools"},
				Variables: []spec.Input{
					{Name: "MY_SERVICE_API_KEY", Datatype: "string", Secret: true},
					{Name: "MY_SERVICE_SECRET", Datatype: "string", Secret: true},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"jira": {Provider: "my-service"},
		},
	}

	// Only provide one of two variables
	envVars := map[string]string{
		"MY_SERVICE_API_KEY": "key1",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "MY_SERVICE_API_KEY") != "key1" {
		t.Error("present env var should be injected")
	}
	if _, ok := agent.Environment["MY_SERVICE_SECRET"]; ok {
		t.Error("absent env var should not be injected")
	}
}

func TestBuildProject_KnowledgeStore(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"docs": {Provider: "qdrant", Persistent: true},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["knowledge-docs"]
	if !ok {
		t.Fatal("missing knowledge-docs service")
	}
	if svc.Image != "qdrant/qdrant:latest" {
		t.Errorf("Image = %q, want %q", svc.Image, "qdrant/qdrant:latest")
	}

	// Should have persistent volume
	if len(svc.Volumes) == 0 {
		t.Error("persistent knowledge store should have volumes")
	}
	if _, ok := project.Volumes["knowledge-docs-data"]; !ok {
		t.Error("missing volume knowledge-docs-data")
	}

	// Agent should get QDRANT_HOST and QDRANT_PORT
	agent := project.Services["agent"]
	if envVal(agent.Environment, "QDRANT_HOST") != "knowledge-docs" {
		t.Errorf("QDRANT_HOST = %q, want %q", envVal(agent.Environment, "QDRANT_HOST"), "knowledge-docs")
	}
}

func TestBuildProject_IngestionScheduleHasProfile(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"schedule": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/schedule/Dockerfile"},
				},
				Trigger: spec.IngestionTrigger{Type: "schedule"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["ingestion-schedule"]
	if !ok {
		t.Fatal("missing ingestion-schedule service")
	}
	// schedule-type ingestions are on-demand — must have the ingestion profile
	if len(svc.Profiles) != 1 || svc.Profiles[0] != "ingestion" {
		t.Errorf("profiles = %v, want [ingestion]", svc.Profiles)
	}
	// must not expose ports
	if len(svc.Ports) != 0 {
		t.Errorf("unexpected ports on schedule ingestion: %v", svc.Ports)
	}
}

func TestBuildProject_IngestionStartupHasProfile(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"data": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/data/Dockerfile"},
				},
				Trigger: spec.IngestionTrigger{Type: "startup"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["ingestion-data"]
	if !ok {
		t.Fatal("missing ingestion-data service")
	}
	if len(svc.Profiles) != 1 || svc.Profiles[0] != "ingestion" {
		t.Errorf("profiles = %v, want [ingestion]", svc.Profiles)
	}
}

func TestBuildProject_IngestionWebhookNoProfileExposesPort(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"webhook": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/webhook/Dockerfile"},
					Port:  3001,
				},
				Trigger: spec.IngestionTrigger{Type: "webhook"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["ingestion-webhook"]
	if !ok {
		t.Fatal("missing ingestion-webhook service")
	}
	// webhook ingestions are persistent servers — must NOT have the ingestion profile
	if len(svc.Profiles) != 0 {
		t.Errorf("webhook ingestion should have no profiles, got %v", svc.Profiles)
	}
	// must expose port 3001
	if len(svc.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Ports))
	}
	if svc.Ports[0].Target != 3001 {
		t.Errorf("port target = %d, want 3001", svc.Ports[0].Target)
	}
	if svc.Ports[0].Published != "3001" {
		t.Errorf("port published = %q, want 3001", svc.Ports[0].Published)
	}
}

func TestBuildProject_IngestionWebhookDefaultPort(t *testing.T) {
	// When no port is specified in the spec, webhook should default to 3001
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"webhook": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/webhook/Dockerfile"},
				},
				Trigger: spec.IngestionTrigger{Type: "webhook"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc := project.Services["ingestion-webhook"]
	if len(svc.Ports) != 1 || svc.Ports[0].Target != 3001 {
		t.Errorf("expected default port 3001, got %v", svc.Ports)
	}
}

func TestBuildProject_FrontendInterface(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Image:      "agent:latest",
			Interfaces: &spec.Interfaces{Frontend: true},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if len(agent.Ports) != 1 {
		t.Fatalf("agent ports = %d, want 1", len(agent.Ports))
	}
	if agent.Ports[0].Target != 80 {
		t.Errorf("port target = %d, want 80 (default)", agent.Ports[0].Target)
	}
	if agent.Ports[0].Published != "3200" {
		t.Errorf("port published = %q, want 3200", agent.Ports[0].Published)
	}
}

func TestBuildProject_FrontendInterfaceCustomPort(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Image:      "agent:latest",
			Interfaces: &spec.Interfaces{Frontend: true},
		},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Frontend: &spec.DevFrontend{Port: 3000},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if len(agent.Ports) != 1 {
		t.Fatalf("agent ports = %d, want 1", len(agent.Ports))
	}
	if agent.Ports[0].Target != 3000 {
		t.Errorf("port target = %d, want 3000 (custom)", agent.Ports[0].Target)
	}
}

func TestBuildProject_NoFrontendNoPorts(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Image:      "agent:latest",
			Interfaces: &spec.Interfaces{Messaging: true},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if len(agent.Ports) != 0 {
		t.Errorf("agent should have no ports when frontend is not enabled, got %d", len(agent.Ports))
	}
}

func TestBuildProject_CustomProviderPrefixedKeys(t *testing.T) {
	// When ast configure stores keys as CLOUDFLARE_AI_API_KEY (prefix + var name),
	// buildEnvironment must find them and inject both prefixed and bare forms.
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Scope: []string{"integrations"},
				Variables: []spec.Input{
					{Name: "AI_API_KEY", Datatype: "string", Secret: true},
					{Name: "ACCOUNT_ID", Datatype: "string", Secret: true},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	// Keys stored by ast configure with CLOUDFLARE_ prefix
	envVars := map[string]string{
		"CLOUDFLARE_AI_API_KEY": "cf-key-123",
		"CLOUDFLARE_ACCOUNT_ID": "acc-456",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]

	// Prefixed keys should be injected
	if envVal(agent.Environment, "CLOUDFLARE_AI_API_KEY") != "cf-key-123" {
		t.Errorf("CLOUDFLARE_AI_API_KEY = %q, want %q", envVal(agent.Environment, "CLOUDFLARE_AI_API_KEY"), "cf-key-123")
	}
	if envVal(agent.Environment, "CLOUDFLARE_ACCOUNT_ID") != "acc-456" {
		t.Errorf("CLOUDFLARE_ACCOUNT_ID = %q, want %q", envVal(agent.Environment, "CLOUDFLARE_ACCOUNT_ID"), "acc-456")
	}

	// Bare keys should also be injected for convenience
	if envVal(agent.Environment, "AI_API_KEY") != "cf-key-123" {
		t.Errorf("AI_API_KEY = %q, want %q", envVal(agent.Environment, "AI_API_KEY"), "cf-key-123")
	}
	if envVal(agent.Environment, "ACCOUNT_ID") != "acc-456" {
		t.Errorf("ACCOUNT_ID = %q, want %q", envVal(agent.Environment, "ACCOUNT_ID"), "acc-456")
	}
}

func TestBuildProject_CustomProviderBareKeys(t *testing.T) {
	// When env vars use bare names (from .env file), they should still be injected.
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Scope: []string{"integrations"},
				Variables: []spec.Input{
					{Name: "AI_API_KEY", Datatype: "string", Secret: true},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	envVars := map[string]string{
		"AI_API_KEY": "bare-key",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "AI_API_KEY") != "bare-key" {
		t.Errorf("AI_API_KEY = %q, want %q", envVal(agent.Environment, "AI_API_KEY"), "bare-key")
	}
	if envVal(agent.Environment, "CLOUDFLARE_AI_API_KEY") != "bare-key" {
		t.Errorf("CLOUDFLARE_AI_API_KEY = %q, want %q", envVal(agent.Environment, "CLOUDFLARE_AI_API_KEY"), "bare-key")
	}
}

func TestBuildProject_NameDerivedCredentials(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Models: map[string]spec.Model{
			"fallback": {Provider: "anthropic"},
		},
	}

	envVars := map[string]string{
		"FALLBACK_API_KEY": "sk-fallback",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "FALLBACK_API_KEY") != "sk-fallback" {
		t.Errorf("FALLBACK_API_KEY = %q, want %q", envVal(agent.Environment, "FALLBACK_API_KEY"), "sk-fallback")
	}
	// Should NOT have ANTHROPIC_API_KEY — the name "fallback" drives the key
	if _, ok := agent.Environment["ANTHROPIC_API_KEY"]; ok {
		t.Error("should not have ANTHROPIC_API_KEY when model name is 'fallback'")
	}
}
