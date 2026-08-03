package compose

import (
	"encoding/json"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestBuildProject_AgentDataVolumeScopedName(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
	}

	project, err := BuildProject(s, "/work", nil)
	require.NoError(t, err)

	vol, ok := project.Volumes[agentDataVolume]
	require.True(t, ok, "agent-data volume must be declared")
	// A non-empty name is required: compose rejects an empty volume name at Up
	// time. The name is scoped to the project so each agent gets its own volume;
	// a shared fixed name would leak chat history and uploads between agents.
	require.NotEmpty(t, vol.Name, "agent-data volume needs a non-empty name")
	assert.Equal(t, "my-agent-agent-data", vol.Name)
	assert.Contains(t, vol.Name, project.Name, "volume name must be project-scoped")
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

	// Dev mode should always be enabled in ast dev
	if envVal(messaging.Environment, "DEV") != "true" {
		t.Error("DEV should be true for messaging in dev mode")
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

// dev.interfaces.messaging.log_level overrides the messaging sidecar's
// default LOG_LEVEL=info. Empty / unset falls back to info.
func TestBuildProject_MessagingLogLevel(t *testing.T) {
	cases := []struct {
		name     string
		override string
		want     string
	}{
		{"default when unset", "", "debug"},
		{"override applied", "info", "info"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &spec.AstroSpec{
				Name:  "my-agent",
				Agent: spec.Container{Image: "agent:latest"},
				Dev: &spec.Dev{
					Interfaces: &spec.DevInterfaces{
						Messaging: &spec.DevMessaging{
							Adapters: []string{"slack"},
							LogLevel: tc.override,
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
			messaging, ok := project.Services["astro-messaging"]
			if !ok {
				t.Fatal("missing astro-messaging service")
			}
			if got := envVal(messaging.Environment, "LOG_LEVEL"); got != tc.want {
				t.Errorf("LOG_LEVEL = %q, want %q", got, tc.want)
			}
		})
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

	// Should create messaging sidecar (playground is bundled into messaging, not a separate service)
	if _, ok := project.Services["astro-messaging"]; !ok {
		t.Error("missing astro-messaging service")
	}
	if _, ok := project.Services["playground"]; ok {
		t.Error("playground should not be a separate service — it is bundled into messaging")
	}

	// Messaging is the chat backend only: WEB_ENABLED stays true, but the chat
	// UI is served by the CLI (internal/chatui) and chat history is persisted
	// via CHAT_DB_PATH.
	messaging := project.Services["astro-messaging"]
	if envVal(messaging.Environment, "WEB_ENABLED") != "true" {
		t.Error("WEB_ENABLED should be true")
	}
	if envVal(messaging.Environment, "CHAT_DB_PATH") != chatDBPath {
		t.Errorf("CHAT_DB_PATH should be %q, got %q", chatDBPath, envVal(messaging.Environment, "CHAT_DB_PATH"))
	}
	if envVal(messaging.Environment, "DEV") != "true" {
		t.Error("DEV should be true for messaging in dev mode")
	}

	// Messaging should expose gRPC (19090->9090) and HTTP (MessagingWebHostPort
	// ->8080). The CLI serves the chat UI on 3100 and proxies to the HTTP port.
	if len(messaging.Ports) != 2 {
		t.Errorf("messaging ports = %d, want 2 (grpc + http)", len(messaging.Ports))
	}
	hasGrpc := false
	hasWeb := false
	for _, p := range messaging.Ports {
		if p.Target == 9090 && p.Published == "19090" {
			hasGrpc = true
		}
		if p.Target == 8080 && p.Published == MessagingWebHostPort {
			hasWeb = true
		}
	}
	if !hasGrpc {
		t.Errorf("messaging should publish gRPC as 19090->9090, got %#v", messaging.Ports)
	}
	if !hasWeb {
		t.Errorf("messaging should publish web as %s->8080, got %#v", MessagingWebHostPort, messaging.Ports)
	}

	// Chat history persistence volume should be mounted at /data.
	hasChatVolume := false
	for _, v := range messaging.Volumes {
		if v.Target == chatDataMountPath {
			hasChatVolume = true
		}
	}
	if !hasChatVolume {
		t.Errorf("messaging should mount a chat-data volume at %s, got %#v", chatDataMountPath, messaging.Volumes)
	}

	// A one-shot init container must chown the fresh (root-owned) chat-data
	// volume to the non-root astro uid before the sidecar starts; otherwise the
	// sidecar cannot create its SQLite DB on /data and crashes (SQLITE_CANTOPEN).
	init, ok := project.Services["astro-messaging-init"]
	if !ok {
		t.Fatal("missing astro-messaging-init service (chowns chat-data volume for non-root sidecar)")
	}
	if init.User != "0:0" {
		t.Errorf("init container must run as root to chown the volume, got user %q", init.User)
	}
	initMountsData := false
	for _, v := range init.Volumes {
		if v.Target == chatDataMountPath {
			initMountsData = true
		}
	}
	if !initMountsData {
		t.Errorf("init container must mount the chat-data volume at %s, got %#v", chatDataMountPath, init.Volumes)
	}
	dep, ok := messaging.DependsOn["astro-messaging-init"]
	if !ok {
		t.Fatal("messaging must depend on astro-messaging-init")
	}
	if dep.Condition != types.ServiceConditionCompletedSuccessfully {
		t.Errorf("messaging should wait for init to complete successfully, got condition %q", dep.Condition)
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
		Integrations: map[string]spec.Integration{
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
		t.Error("cloud integration provider should not create a service")
	}
}

func TestBuildProject_CustomProviderCredentials(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Providers: map[string]spec.CustomProvider{
			"my-service": {
				Scope: []string{"integrations"},
				Variables: []spec.Input{
					{Name: "API_KEY", Datatype: "string", Secret: true},
					{Name: "SECRET", Datatype: "string", Secret: true},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"jira": {Provider: "my-service"},
		},
	}

	// Resolver emits <PROVIDER>_<VARNAME>: MY_SERVICE_API_KEY / MY_SERVICE_SECRET.
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
				Scope: []string{"integrations"},
				Variables: []spec.Input{
					{Name: "API_KEY", Datatype: "string", Secret: true},
					{Name: "SECRET", Datatype: "string", Secret: true},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"jira": {Provider: "my-service"},
		},
	}

	// Only provide one of two variables (using the resolver-correct prefixed name).
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
			"docs": {Provider: "qdrant"},
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
					Image:  "pgvector/pgvector:pg17",
					Port:   5432,
					Volume: "/var/lib/postgresql/data",
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

func TestBuildProject_KnowledgeInputsInjected(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"db": {
				Container: &spec.ContainerConfig{
					Image:  "postgres:17",
					Port:   5432,
					Volume: "/var/lib/postgresql/data",
				},
				Inputs: []spec.Input{
					{Name: "POSTGRES_PASSWORD", Datatype: "string", Default: "default-pw"},
					{Name: "POSTGRES_DB", Datatype: "string", Default: "my_db"},
				},
			},
		},
	}

	// Test with default values
	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc := project.Services["knowledge-db"]
	if envVal(svc.Environment, "POSTGRES_PASSWORD") != "default-pw" {
		t.Errorf("POSTGRES_PASSWORD = %q, want %q", envVal(svc.Environment, "POSTGRES_PASSWORD"), "default-pw")
	}
	if envVal(svc.Environment, "POSTGRES_DB") != "my_db" {
		t.Errorf("POSTGRES_DB = %q, want %q", envVal(svc.Environment, "POSTGRES_DB"), "my_db")
	}

	// Test with envVars override
	project2, err := BuildProject(s, "/work", map[string]string{
		"POSTGRES_PASSWORD": "override-pw",
	})
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc2 := project2.Services["knowledge-db"]
	if envVal(svc2.Environment, "POSTGRES_PASSWORD") != "override-pw" {
		t.Errorf("POSTGRES_PASSWORD = %q, want %q", envVal(svc2.Environment, "POSTGRES_PASSWORD"), "override-pw")
	}
	// POSTGRES_DB should still use default
	if envVal(svc2.Environment, "POSTGRES_DB") != "my_db" {
		t.Errorf("POSTGRES_DB = %q, want %q", envVal(svc2.Environment, "POSTGRES_DB"), "my_db")
	}
}

func TestBuildProject_PostgresCredentialsAutoInjected(t *testing.T) {
	// Container-mode postgres should get USER/PASSWORD/DB on both the sidecar
	// and the agent without the user declaring any inputs — mirrors prod, where
	// generateKnowledgeCredentials does the same.
	s := &spec.AstroSpec{
		Name:  "recruiter-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"db": {Provider: "postgres"},
		},
	}

	t.Run("defaults when no envVars", func(t *testing.T) {
		project, err := BuildProject(s, "/work", nil)
		require.NoError(t, err)

		sidecar := project.Services["knowledge-db"]
		assert.Equal(t, "astro", envVal(sidecar.Environment, "POSTGRES_USER"))
		assert.Equal(t, "localdev", envVal(sidecar.Environment, "POSTGRES_PASSWORD"))
		assert.Equal(t, "recruiter_agent", envVal(sidecar.Environment, "POSTGRES_DB"))

		agent := project.Services["agent"]
		assert.Equal(t, "astro", envVal(agent.Environment, "POSTGRES_USER"))
		assert.Equal(t, "localdev", envVal(agent.Environment, "POSTGRES_PASSWORD"))
		assert.Equal(t, "recruiter_agent", envVal(agent.Environment, "POSTGRES_DB"))
		assert.Equal(t, "knowledge-db", envVal(agent.Environment, "POSTGRES_HOST"))
		assert.Equal(t, "5432", envVal(agent.Environment, "POSTGRES_PORT"))
	})

	t.Run("envVars override defaults on both sidecar and agent", func(t *testing.T) {
		project, err := BuildProject(s, "/work", map[string]string{
			"POSTGRES_USER":     "custom_user",
			"POSTGRES_PASSWORD": "custom_pw",
			"POSTGRES_DB":       "custom_db",
		})
		require.NoError(t, err)

		for _, svcName := range []string{"knowledge-db", "agent"} {
			svc := project.Services[svcName]
			assert.Equal(t, "custom_user", envVal(svc.Environment, "POSTGRES_USER"), svcName)
			assert.Equal(t, "custom_pw", envVal(svc.Environment, "POSTGRES_PASSWORD"), svcName)
			assert.Equal(t, "custom_db", envVal(svc.Environment, "POSTGRES_DB"), svcName)
		}
	})
}

func TestBuildProject_IntegrationInputsInjected(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Integrations: map[string]spec.Integration{
			"mcp": {
				Container: &spec.ContainerConfig{
					Image: "my-mcp:latest",
					Port:  8080,
				},
				Inputs: []spec.Input{
					{Name: "API_KEY", Datatype: "string", Default: "test-key"},
				},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc := project.Services["tool-mcp"]
	if envVal(svc.Environment, "API_KEY") != "test-key" {
		t.Errorf("API_KEY = %q, want %q", envVal(svc.Environment, "API_KEY"), "test-key")
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

func TestBuildEnvironment_FrontendInjectsPORT(t *testing.T) {
	t.Run("default port 80", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name: "my-agent",
			Agent: spec.Container{
				Image:      "agent:latest",
				Interfaces: &spec.Interfaces{Frontend: true},
			},
		}
		env := BuildEnvironment(s, nil)
		if got := envVal(env, "PORT"); got != "80" {
			t.Errorf("PORT = %q, want 80", got)
		}
	})

	t.Run("dev.interfaces.frontend.port override", func(t *testing.T) {
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
		env := BuildEnvironment(s, nil)
		if got := envVal(env, "PORT"); got != "3000" {
			t.Errorf("PORT = %q, want 3000", got)
		}
	})

	t.Run("no frontend → no PORT", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "agent:latest"},
		}
		env := BuildEnvironment(s, nil)
		if _, ok := env["PORT"]; ok {
			t.Error("PORT should not be set when frontend is disabled")
		}
	})
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
		Integrations: map[string]spec.Integration{
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

	// Resolver-correct prefixed names only — matches what the deployer injects
	// in prod. Bare names are deliberately not emitted (used to be, caused
	// dev/prod divergence — see ai-gateway-astro-server changelog).
	if envVal(agent.Environment, "CLOUDFLARE_AI_API_KEY") != "cf-key-123" {
		t.Errorf("CLOUDFLARE_AI_API_KEY = %q, want %q", envVal(agent.Environment, "CLOUDFLARE_AI_API_KEY"), "cf-key-123")
	}
	if envVal(agent.Environment, "CLOUDFLARE_ACCOUNT_ID") != "acc-456" {
		t.Errorf("CLOUDFLARE_ACCOUNT_ID = %q, want %q", envVal(agent.Environment, "CLOUDFLARE_ACCOUNT_ID"), "acc-456")
	}
	if _, ok := agent.Environment["AI_API_KEY"]; ok {
		t.Error("bare AI_API_KEY must not be emitted; only the resolver-correct CLOUDFLARE_AI_API_KEY")
	}
	if _, ok := agent.Environment["ACCOUNT_ID"]; ok {
		t.Error("bare ACCOUNT_ID must not be emitted; only the resolver-correct CLOUDFLARE_ACCOUNT_ID")
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

func TestBuildProject_ResolverCredentialNames(t *testing.T) {
	// Single-entry cloud provider → bare ANTHROPIC_API_KEY (§8.1). Matches
	// what the deployer injects in prod — dev and prod use the same
	// env-var names so agent code is portable.
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Models: map[string]spec.Model{
			"fallback": {Provider: "anthropic"},
		},
	}

	envVars := map[string]string{
		"ANTHROPIC_API_KEY": "sk-anthropic",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "ANTHROPIC_API_KEY") != "sk-anthropic" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", envVal(agent.Environment, "ANTHROPIC_API_KEY"), "sk-anthropic")
	}
	// Old <NAME>_<SUFFIX> convention should NOT appear — the resolver names by
	// provider, with entry-name qualification only when multiple entries share
	// a provider.
	if _, ok := agent.Environment["FALLBACK_API_KEY"]; ok {
		t.Error("FALLBACK_API_KEY (old per-entry convention) must not appear; resolver names by provider")
	}
}

func TestBuildProject_AgentCoreRuntime(t *testing.T) {
	newSpec := func() *spec.AstroSpec {
		return &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{AgentCore: true},
			Agent: spec.Container{Image: "agent:latest"},
			Dev: &spec.Dev{
				Interfaces: &spec.DevInterfaces{
					Messaging: &spec.DevMessaging{Adapters: []string{"web"}},
				},
			},
		}
	}

	// AgentCore dev: the agent serves /invocations; messaging invokes it by the
	// compose service DNS name over HTTP (topology inverted vs the gRPC default).
	t.Run("agentcore flips transport", func(t *testing.T) {
		project, err := BuildProject(newSpec(), "/work", nil)
		if err != nil {
			t.Fatalf("BuildProject() error = %v", err)
		}
		agent := project.Services["agent"]
		if envVal(agent.Environment, "ASTRO_RUNTIME") != "agentcore" {
			t.Errorf("agent ASTRO_RUNTIME = %q, want agentcore", envVal(agent.Environment, "ASTRO_RUNTIME"))
		}
		if got := envVal(agent.Environment, "GRPC_SERVER_ADDR"); got != "" {
			t.Errorf("agent GRPC_SERVER_ADDR = %q, want empty (agent serves, not dials)", got)
		}
		messaging := project.Services["astro-messaging"]
		if envVal(messaging.Environment, "AGENT_TRANSPORT") != "agentcore" {
			t.Errorf("messaging AGENT_TRANSPORT = %q, want agentcore", envVal(messaging.Environment, "AGENT_TRANSPORT"))
		}
		if got := envVal(messaging.Environment, "AGENT_RUNTIME_ENDPOINT"); got != "http://agent:8080" {
			t.Errorf("messaging AGENT_RUNTIME_ENDPOINT = %q, want http://agent:8080", got)
		}
	})

	// Default (eks) is unchanged: agent dials the messaging gRPC server.
	t.Run("eks default unchanged", func(t *testing.T) {
		s := newSpec()
		s.Meta.AgentCore = false
		project, err := BuildProject(s, "/work", nil)
		if err != nil {
			t.Fatalf("BuildProject() error = %v", err)
		}
		agent := project.Services["agent"]
		if envVal(agent.Environment, "GRPC_SERVER_ADDR") != "astro-messaging:9090" {
			t.Errorf("agent GRPC_SERVER_ADDR = %q, want astro-messaging:9090", envVal(agent.Environment, "GRPC_SERVER_ADDR"))
		}
		if got := envVal(agent.Environment, "ASTRO_RUNTIME"); got != "" {
			t.Errorf("agent ASTRO_RUNTIME = %q, want empty for eks", got)
		}
		if got := envVal(project.Services["astro-messaging"].Environment, "AGENT_TRANSPORT"); got != "" {
			t.Errorf("messaging AGENT_TRANSPORT = %q, want empty for eks", got)
		}
	})
}
