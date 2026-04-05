package compose

import (
	"encoding/json"
	"strings"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/docker/compose/v5/pkg/api"
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

func TestBuildProject_ScopedName(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "@example/release-note-helper",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	if project.Name != "release-note-helper" {
		t.Errorf("Name = %q, want %q", project.Name, "release-note-helper")
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

func TestBuildProject_SlackInterface_MissingToken(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack", "web"},
				},
			},
		},
	}

	project, err := BuildProject(s, "/work", map[string]string{})
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	messaging, ok := project.Services["astro-messaging"]
	if !ok {
		t.Fatal("missing astro-messaging service")
	}

	if envVal(messaging.Environment, "SLACK_ENABLED") != "" {
		t.Error("SLACK_ENABLED should not be set when token is missing")
	}
	if envVal(messaging.Environment, "WEB_ENABLED") != "true" {
		t.Error("WEB_ENABLED should still be true")
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

	// Messaging should expose both gRPC (19090->9090) and HTTP (3100->8080) ports
	if len(messaging.Ports) != 2 {
		t.Errorf("messaging ports = %d, want 2 (grpc + http)", len(messaging.Ports))
	}
	hasGrpc := false
	hasWeb := false
	for _, p := range messaging.Ports {
		if p.Target == 9090 && p.Published == "19090" {
			hasGrpc = true
		}
		if p.Target == 8080 && p.Published == "3100" {
			hasWeb = true
		}
	}
	if !hasGrpc {
		t.Errorf("messaging should publish gRPC as 19090->9090, got %#v", messaging.Ports)
	}
	if !hasWeb {
		t.Errorf("messaging should publish web as 3100->8080, got %#v", messaging.Ports)
	}

	// Collector should not be present in dev mode (runs as K8s sidecar only).
	if _, ok := project.Services["astro-collector"]; ok {
		t.Error("astro-collector should not be present in dev compose")
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

func TestBuildProject_CustomKnowledgePersistence(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"db": {
				Container: &spec.ContainerConfig{
					Image:      "pgvector/pgvector:pg17",
					Port:       5432,
					Volume:     "/var/lib/postgresql/data",
					Persistent: true,
				},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["knowledge-db"]
	if !ok {
		t.Fatal("missing knowledge-db service")
	}
	if svc.Image != "pgvector/pgvector:pg17" {
		t.Errorf("Image = %q, want %q", svc.Image, "pgvector/pgvector:pg17")
	}
	if len(svc.Volumes) == 0 {
		t.Error("persistent custom container should have volumes")
	} else if svc.Volumes[0].Target != "/var/lib/postgresql/data" {
		t.Errorf("volume target = %q, want %q", svc.Volumes[0].Target, "/var/lib/postgresql/data")
	}
	if _, ok := project.Volumes["knowledge-db-data"]; !ok {
		t.Error("missing volume knowledge-db-data")
	}
}

func TestBuildProject_CustomKnowledgePersistentNoVolume(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"db": {
				Container: &spec.ContainerConfig{
					Image:      "postgres:17",
					Port:       5432,
					Persistent: true,
					// No Volume set — should error
				},
			},
		},
	}

	_, err := BuildProject(s, "/work", nil)
	if err == nil {
		t.Fatal("expected error for persistent custom container without volume")
	}
	if !strings.Contains(err.Error(), "no volume path") {
		t.Errorf("error = %q, want it to mention 'no volume path'", err.Error())
	}
}

func TestBuildProject_KnowledgeExtraPortsPublished(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"graph": {Provider: "neo4j"},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["knowledge-graph"]
	if !ok {
		t.Fatal("missing knowledge-graph service")
	}

	hasBoltPort := false
	hasDefaultPort := false
	for _, p := range svc.Ports {
		if p.Target == 7687 && p.Published == "7687" {
			hasBoltPort = true
		}
		if p.Target == 7474 && p.Published == "7474" {
			hasDefaultPort = true
		}
	}
	if !hasDefaultPort {
		t.Error("Neo4j default HTTP port 7474 should be published")
	}
	if !hasBoltPort {
		t.Error("Neo4j bolt port 7687 should be published via ExtraPorts")
	}
}

func TestBuildProject_QdrantExtraPortsPublished(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"docs": {Provider: "qdrant"},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc := project.Services["knowledge-docs"]
	hasGrpcPort := false
	for _, p := range svc.Ports {
		if p.Target == 6334 && p.Published == "6334" {
			hasGrpcPort = true
		}
	}
	if !hasGrpcPort {
		t.Error("Qdrant gRPC port 6334 should be published via ExtraPorts")
	}
}

func TestBuildProject_SlackConfigJSON(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack"},
					Slack: &spec.SlackAdapterConfig{
						ActionableReactions: []string{"ticket", "bug"},
						AllowedChannelIDs:   []string{"C123", "C999"},
						AllowedUserIDs:      []string{"U123", "U999"},
						SocketMode:          boolPtr(false),
						AutoThread:          boolPtr(true),
					},
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

	messaging := project.Services["astro-messaging"]

	if _, ok := messaging.Environment["SLACK_ACTIONABLE_REACTIONS"]; ok {
		t.Error("legacy SLACK_ACTIONABLE_REACTIONS should no longer be set")
	}
	if _, ok := messaging.Environment["SLACK_SOCKET_MODE"]; ok {
		t.Error("legacy SLACK_SOCKET_MODE should no longer be set")
	}

	raw := envVal(messaging.Environment, "SLACK_CONFIG")
	if raw == "" {
		t.Fatal("SLACK_CONFIG env var not set")
	}

	var cfg spec.SlackAdapterConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("SLACK_CONFIG is not valid JSON: %v", err)
	}
	if len(cfg.ActionableReactions) != 2 || cfg.ActionableReactions[0] != "ticket" || cfg.ActionableReactions[1] != "bug" {
		t.Errorf("ActionableReactions = %v, want [ticket bug]", cfg.ActionableReactions)
	}
	if len(cfg.AllowedChannelIDs) != 2 || cfg.AllowedChannelIDs[0] != "C123" || cfg.AllowedChannelIDs[1] != "C999" {
		t.Errorf("AllowedChannelIDs = %v, want [C123 C999]", cfg.AllowedChannelIDs)
	}
	if len(cfg.AllowedUserIDs) != 2 || cfg.AllowedUserIDs[0] != "U123" || cfg.AllowedUserIDs[1] != "U999" {
		t.Errorf("AllowedUserIDs = %v, want [U123 U999]", cfg.AllowedUserIDs)
	}
	if cfg.SocketMode == nil || *cfg.SocketMode != false {
		t.Errorf("SocketMode = %v, want false", cfg.SocketMode)
	}
	if cfg.AutoThread == nil || *cfg.AutoThread != true {
		t.Errorf("AutoThread = %v, want true", cfg.AutoThread)
	}
}

func TestBuildProject_SlackNoConfigBlock(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
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

	messaging := project.Services["astro-messaging"]
	if _, ok := messaging.Environment["SLACK_CONFIG"]; ok {
		t.Error("SLACK_CONFIG should not be set when no slack block is configured")
	}
	if _, ok := messaging.Environment["SLACK_ACTIONABLE_REACTIONS"]; ok {
		t.Error("legacy SLACK_ACTIONABLE_REACTIONS should never be set")
	}
}

func TestBuildProject_SlackConfigReactionsOnly(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack"},
					Slack: &spec.SlackAdapterConfig{
						ActionableReactions: []string{"ticket"},
					},
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

	messaging := project.Services["astro-messaging"]
	raw := envVal(messaging.Environment, "SLACK_CONFIG")
	if raw == "" {
		t.Fatal("SLACK_CONFIG env var not set")
	}

	var cfg spec.SlackAdapterConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("SLACK_CONFIG is not valid JSON: %v", err)
	}
	if len(cfg.ActionableReactions) != 1 || cfg.ActionableReactions[0] != "ticket" {
		t.Errorf("ActionableReactions = %v, want [ticket]", cfg.ActionableReactions)
	}
	if cfg.SocketMode != nil {
		t.Errorf("SocketMode should be omitted (nil), got %v", *cfg.SocketMode)
	}
	if cfg.AutoThread != nil {
		t.Errorf("AutoThread should be omitted (nil), got %v", *cfg.AutoThread)
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

func TestBuildProject_CustomLabels(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"run": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "Dockerfile"},
				},
				Trigger: spec.IngestionTrigger{Type: "startup"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	requiredKeys := []string{
		api.ProjectLabel,
		api.ServiceLabel,
		api.VersionLabel,
		api.WorkingDirLabel,
		api.OneoffLabel,
	}

	for name, svc := range project.Services {
		for _, key := range requiredKeys {
			if _, ok := svc.CustomLabels[key]; !ok {
				t.Errorf("service %q missing required CustomLabel %q", name, key)
			}
		}
		if svc.CustomLabels[api.ProjectLabel] != project.Name {
			t.Errorf("service %q: ProjectLabel = %q, want %q", name, svc.CustomLabels[api.ProjectLabel], project.Name)
		}
		if svc.CustomLabels[api.ServiceLabel] != name {
			t.Errorf("service %q: ServiceLabel = %q, want %q", name, svc.CustomLabels[api.ServiceLabel], name)
		}
		if svc.CustomLabels[api.WorkingDirLabel] != project.WorkingDir {
			t.Errorf("service %q: WorkingDirLabel = %q, want %q", name, svc.CustomLabels[api.WorkingDirLabel], project.WorkingDir)
		}
		if svc.CustomLabels[api.OneoffLabel] != "False" {
			t.Errorf("service %q: OneoffLabel = %q, want %q", name, svc.CustomLabels[api.OneoffLabel], "False")
		}
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
