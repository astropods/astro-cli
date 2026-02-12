package deployment

import (
	"testing"

	"github.com/postman/astro/packages/astro-spec"
)

// TestTranslate verifies that Translator.Translate produces the correct set of
// Kubernetes manifest kinds for various AstroSpec configurations: minimal agent,
// persistent/non-persistent knowledge stores, cloud integrations with credentials,
// all four ingestion trigger types, messaging/custom interfaces, self-hosted
// models and tools, and a full combination of everything.
func TestTranslate(t *testing.T) {
	type checkFn func(t *testing.T, result *TranslationResult)

	countKind := func(result *TranslationResult, kind string) int {
		n := 0
		for _, m := range result.Manifests {
			if m.Kind == kind {
				n++
			}
		}
		return n
	}

	tests := []struct {
		name   string
		spec   *spec.AstroSpec
		creds  map[string]string
		checks []checkFn
	}{
		{
			name: "minimal agent - container only",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{
					Image: "my-agent:latest",
				},
			},
			checks: []checkFn{
				func(t *testing.T, r *TranslationResult) {
					// Should have: 1 ConfigMap, 1 Service (agent), 1 Deployment (agent)
					if countKind(r, "ConfigMap") != 1 {
						t.Errorf("expected 1 ConfigMap, got %d", countKind(r, "ConfigMap"))
					}
					if countKind(r, "Service") != 1 {
						t.Errorf("expected 1 Service, got %d", countKind(r, "Service"))
					}
					if countKind(r, "Deployment") != 1 {
						t.Errorf("expected 1 Deployment, got %d", countKind(r, "Deployment"))
					}
					if countKind(r, "Secret") != 0 {
						t.Errorf("expected 0 Secrets (no credentials), got %d", countKind(r, "Secret"))
					}
				},
			},
		},
		{
			name: "knowledge stores - persistent and non-persistent",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{
					Image: "my-agent:latest",
				},
				Knowledge: map[string]spec.Knowledge{
					"vectors": {
						Provider:   "qdrant",
						Persistent: true,
					},
					"cache": {
						Provider: "redis",
					},
				},
			},
			checks: []checkFn{
				func(t *testing.T, r *TranslationResult) {
					if countKind(r, "StatefulSet") != 1 {
						t.Errorf("expected 1 StatefulSet (persistent qdrant), got %d", countKind(r, "StatefulSet"))
					}
					// 1 agent + 1 non-persistent redis = 2 Deployments
					if countKind(r, "Deployment") != 2 {
						t.Errorf("expected 2 Deployments (agent + redis), got %d", countKind(r, "Deployment"))
					}
					// 1 agent + 1 qdrant + 1 redis = 3 Services
					if countKind(r, "Service") != 3 {
						t.Errorf("expected 3 Services, got %d", countKind(r, "Service"))
					}
				},
			},
		},
		{
			name: "integrations only - credentials provided",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{
					Image: "my-agent:latest",
				},
				Integrations: spec.Integrations{
					Models: []spec.IntegrationModel{
						{Name: "claude", Provider: "anthropic"},
					},
					Tools: []spec.IntegrationTool{
						{Name: "gh", Provider: "github"},
					},
				},
			},
			creds: map[string]string{
				"ANTHROPIC_API_KEY": "sk-test",
				"GITHUB_TOKEN":     "ghp-test",
			},
			checks: []checkFn{
				func(t *testing.T, r *TranslationResult) {
					if countKind(r, "Secret") != 1 {
						t.Errorf("expected 1 Secret, got %d", countKind(r, "Secret"))
					}
					// No extra deployments for cloud integrations
					if countKind(r, "Deployment") != 1 {
						t.Errorf("expected 1 Deployment (agent only), got %d", countKind(r, "Deployment"))
					}
				},
			},
		},
		{
			name: "all ingestion types",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{
					Image: "my-agent:latest",
				},
				Ingestion: map[string]spec.Ingestion{
					"scheduled": {
						Container: spec.ContainerConfig{Image: "ingest:latest"},
						Trigger:   spec.IngestionTrigger{Type: "schedule", Schedule: "0 * * * *"},
					},
					"on-start": {
						Container: spec.ContainerConfig{Image: "ingest:latest"},
						Trigger:   spec.IngestionTrigger{Type: "startup"},
					},
					"webhook-ingest": {
						Container: spec.ContainerConfig{Image: "ingest:latest"},
						Trigger:   spec.IngestionTrigger{Type: "webhook"},
					},
					"manual-ingest": {
						Container: spec.ContainerConfig{Image: "ingest:latest"},
						Trigger:   spec.IngestionTrigger{Type: "manual"},
					},
				},
			},
			checks: []checkFn{
				func(t *testing.T, r *TranslationResult) {
					if countKind(r, "CronJob") != 1 {
						t.Errorf("expected 1 CronJob (schedule), got %d", countKind(r, "CronJob"))
					}
					if countKind(r, "Job") != 1 {
						t.Errorf("expected 1 Job (startup), got %d", countKind(r, "Job"))
					}
					// agent + webhook = 2
					if countKind(r, "Deployment") != 2 {
						t.Errorf("expected 2 Deployments (agent + webhook), got %d", countKind(r, "Deployment"))
					}
					// agent + webhook = 2
					if countKind(r, "Service") != 2 {
						t.Errorf("expected 2 Services (agent + webhook), got %d", countKind(r, "Service"))
					}
				},
			},
		},
		{
			name: "interfaces - slack and custom",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{
					Image: "my-agent:latest",
				},
				Interfaces: map[string]spec.Interface{
					"slack-bot": {
						Type: "slack",
					},
					"web-ui": {
						Type: "custom",
						Service: &spec.InterfaceService{
							Image: "web-ui:latest",
							Ports: []string{"3000"},
						},
					},
				},
			},
			checks: []checkFn{
				func(t *testing.T, r *TranslationResult) {
					// agent + slack messaging + custom interface = 3 Deployments
					if countKind(r, "Deployment") != 3 {
						t.Errorf("expected 3 Deployments (agent + slack + custom), got %d", countKind(r, "Deployment"))
					}
					// agent + slack messaging + custom interface = 3 Services
					if countKind(r, "Service") != 3 {
						t.Errorf("expected 3 Services, got %d", countKind(r, "Service"))
					}
				},
			},
		},
		{
			name: "self-hosted model",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{
					Image: "my-agent:latest",
				},
				Models: map[string]spec.Model{
					"embedder": {
						Container: spec.ContainerConfig{
							Image: "embedder:latest",
						},
					},
				},
			},
			checks: []checkFn{
				func(t *testing.T, r *TranslationResult) {
					// agent + model = 2 Deployments
					if countKind(r, "Deployment") != 2 {
						t.Errorf("expected 2 Deployments (agent + model), got %d", countKind(r, "Deployment"))
					}
					// agent + model = 2 Services
					if countKind(r, "Service") != 2 {
						t.Errorf("expected 2 Services, got %d", countKind(r, "Service"))
					}
				},
			},
		},
		{
			name: "self-hosted tool",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{
					Image: "my-agent:latest",
				},
				Tools: map[string]spec.Tool{
					"mcp-server": {
						Container: &spec.ContainerConfig{
							Image: "mcp:latest",
						},
					},
				},
			},
			checks: []checkFn{
				func(t *testing.T, r *TranslationResult) {
					// agent + tool = 2 Deployments
					if countKind(r, "Deployment") != 2 {
						t.Errorf("expected 2 Deployments (agent + tool), got %d", countKind(r, "Deployment"))
					}
					if countKind(r, "Service") != 2 {
						t.Errorf("expected 2 Services, got %d", countKind(r, "Service"))
					}
				},
			},
		},
		{
			name: "full combination",
			spec: &spec.AstroSpec{
				Agent: "full-agent",
				Meta:  spec.Meta{Version: "2.0"},
				Container: spec.Container{
					Image: "my-agent:latest",
				},
				Models: map[string]spec.Model{
					"embedder": {
						Container: spec.ContainerConfig{Image: "embedder:latest"},
					},
				},
				Knowledge: map[string]spec.Knowledge{
					"vectors": {
						Provider:   "qdrant",
						Persistent: true,
					},
				},
				Tools: map[string]spec.Tool{
					"mcp": {Container: &spec.ContainerConfig{Image: "mcp:latest"}},
				},
				Integrations: spec.Integrations{
					Models: []spec.IntegrationModel{
						{Name: "claude", Provider: "anthropic"},
					},
				},
				Interfaces: map[string]spec.Interface{
					"slack": {Type: "slack"},
				},
				Ingestion: map[string]spec.Ingestion{
					"cron": {
						Container: spec.ContainerConfig{Image: "ingest:latest"},
						Trigger:   spec.IngestionTrigger{Type: "schedule", Schedule: "*/5 * * * *"},
					},
					"boot": {
						Container: spec.ContainerConfig{Image: "ingest:latest"},
						Trigger:   spec.IngestionTrigger{Type: "startup"},
					},
				},
			},
			creds: map[string]string{"ANTHROPIC_API_KEY": "sk-test"},
			checks: []checkFn{
				func(t *testing.T, r *TranslationResult) {
					if countKind(r, "Secret") != 1 {
						t.Errorf("expected 1 Secret, got %d", countKind(r, "Secret"))
					}
					if countKind(r, "ConfigMap") != 1 {
						t.Errorf("expected 1 ConfigMap, got %d", countKind(r, "ConfigMap"))
					}
					if countKind(r, "StatefulSet") != 1 {
						t.Errorf("expected 1 StatefulSet (qdrant), got %d", countKind(r, "StatefulSet"))
					}
					if countKind(r, "CronJob") != 1 {
						t.Errorf("expected 1 CronJob, got %d", countKind(r, "CronJob"))
					}
					if countKind(r, "Job") != 1 {
						t.Errorf("expected 1 Job, got %d", countKind(r, "Job"))
					}
					// agent + embedder + mcp + slack = 4 Deployments
					if countKind(r, "Deployment") != 4 {
						t.Errorf("expected 4 Deployments, got %d", countKind(r, "Deployment"))
					}
					// agent + embedder + qdrant + mcp + slack = 5 Services
					if countKind(r, "Service") != 5 {
						t.Errorf("expected 5 Services, got %d", countKind(r, "Service"))
					}
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator("test-agent", "1.0", "test-ns", "registry.example.com", tt.creds)
			if tt.spec.Agent != "" {
				translator = NewTranslator(tt.spec.Agent, tt.spec.Meta.Version, "test-ns", "registry.example.com", tt.creds)
			}

			result, err := translator.Translate(tt.spec)
			if err != nil {
				t.Fatalf("Translate returned error: %v", err)
			}

			for _, check := range tt.checks {
				check(t, result)
			}
		})
	}
}

// TestTranslateManifestOrdering verifies that manifests are emitted in dependency
// order: Secret/ConfigMap first, then Services, StatefulSets, Deployments,
// CronJobs, and finally Jobs.
func TestTranslateManifestOrdering(t *testing.T) {
	astroSpec := &spec.AstroSpec{
		Agent: "order-agent",
		Meta:  spec.Meta{Version: "1.0"},
		Container: spec.Container{
			Image: "my-agent:latest",
		},
		Knowledge: map[string]spec.Knowledge{
			"db": {
				Provider:   "qdrant",
				Persistent: true,
			},
		},
		Ingestion: map[string]spec.Ingestion{
			"cron": {
				Container: spec.ContainerConfig{Image: "ingest:latest"},
				Trigger:   spec.IngestionTrigger{Type: "schedule", Schedule: "0 * * * *"},
			},
			"boot": {
				Container: spec.ContainerConfig{Image: "ingest:latest"},
				Trigger:   spec.IngestionTrigger{Type: "startup"},
			},
		},
	}

	creds := map[string]string{"API_KEY": "test"}
	translator := NewTranslator("order-agent", "1.0", "test-ns", "registry.example.com", creds)
	result, err := translator.Translate(astroSpec)
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}

	// Expected order: Secret/ConfigMap first, then Services, StatefulSets, Deployments, CronJobs, Jobs
	kindOrder := map[string]int{
		"Secret":      0,
		"ConfigMap":   1,
		"Service":     2,
		"StatefulSet": 3,
		"Deployment":  4,
		"CronJob":     5,
		"Job":         6,
	}

	lastOrder := -1
	lastKind := ""
	for _, m := range result.Manifests {
		order, ok := kindOrder[m.Kind]
		if !ok {
			t.Errorf("unexpected manifest kind: %s", m.Kind)
			continue
		}
		if order < lastOrder {
			t.Errorf("manifest ordering violated: %s came after %s", m.Kind, lastKind)
		}
		if order > lastOrder {
			lastOrder = order
			lastKind = m.Kind
		}
	}
}

// TestTranslateResourceNaming verifies that generated resource names follow the
// {agent}-{type}-{name} convention for knowledge/tool resources and
// {agent}-{type} for the main agent resource.
func TestTranslateResourceNaming(t *testing.T) {
	astroSpec := &spec.AstroSpec{
		Agent: "my-agent",
		Meta:  spec.Meta{Version: "1.0"},
		Container: spec.Container{
			Image: "my-agent:latest",
		},
		Knowledge: map[string]spec.Knowledge{
			"vectors": {
				Provider: "qdrant",
			},
		},
		Tools: map[string]spec.Tool{
			"mcp": {Container: &spec.ContainerConfig{Image: "mcp:latest"}},
		},
	}

	translator := NewTranslator("my-agent", "1.0", "test-ns", "registry.example.com", nil)
	result, err := translator.Translate(astroSpec)
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}

	// Check that resource names follow the {agent}-{type}-{name} pattern
	expectedNames := map[string]bool{
		"my-agent-knowledge-vectors": true,
		"my-agent-tool-mcp":          true,
		"my-agent-agent":             true,
	}

	for _, m := range result.Manifests {
		if m.Kind == "ConfigMap" || m.Kind == "Secret" {
			continue
		}
		if !expectedNames[m.Name] {
			t.Errorf("unexpected resource name: %s (kind: %s)", m.Name, m.Kind)
		}
	}
}

// TestTranslateServiceDNSMap verifies that the ServiceDNSMap entries follow the
// {name}.{namespace}.svc.cluster.local format and that entries exist for the
// agent service and each knowledge store service.
func TestTranslateServiceDNSMap(t *testing.T) {
	astroSpec := &spec.AstroSpec{
		Agent: "dns-agent",
		Meta:  spec.Meta{Version: "1.0"},
		Container: spec.Container{
			Image: "my-agent:latest",
		},
		Knowledge: map[string]spec.Knowledge{
			"db": {
				Provider: "qdrant",
			},
		},
	}

	translator := NewTranslator("dns-agent", "1.0", "prod-ns", "registry.example.com", nil)
	result, err := translator.Translate(astroSpec)
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}

	// Check DNS entries follow {name}.{namespace}.svc.cluster.local
	for name, dns := range result.ServiceDNSMap {
		expected := name + ".prod-ns.svc.cluster.local"
		if dns != expected {
			t.Errorf("DNS for %s: expected %s, got %s", name, expected, dns)
		}
	}

	// Should have entries for agent and knowledge store
	agentName := GenerateAgentResourceName("dns-agent", "agent")
	if _, ok := result.ServiceDNSMap[agentName]; !ok {
		t.Errorf("expected DNS entry for agent service %s", agentName)
	}

	knowledgeName := GenerateResourceName("dns-agent", "knowledge", "db")
	if _, ok := result.ServiceDNSMap[knowledgeName]; !ok {
		t.Errorf("expected DNS entry for knowledge service %s", knowledgeName)
	}
}
