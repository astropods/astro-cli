package k8s

import (
	"strings"
	"testing"
)

// TestBuildConfigMap verifies that BuildConfigMap produces a ConfigMap with the
// {agent}-{version}-config name format, all input data entries preserved
// exactly, and the astro.dev/agent label set.
func TestBuildConfigMap(t *testing.T) {
	data := map[string]string{
		"QDRANT_HOST": "agent-knowledge-vectors.default.svc.cluster.local",
		"QDRANT_PORT": "6333",
		"AGENT_URL":   "http://agent.default.svc.cluster.local:8080",
	}

	cm := BuildConfigMap("prod-ns", "my-agent", "1.0", data)

	// Name format: {agent}-{version}-config
	if !strings.Contains(cm.Name, "my-agent") || !strings.Contains(cm.Name, "config") {
		t.Errorf("name should follow {agent}-{version}-config, got %s", cm.Name)
	}

	if cm.Namespace != "prod-ns" {
		t.Errorf("namespace: expected prod-ns, got %s", cm.Namespace)
	}

	if cm.Kind != "ConfigMap" {
		t.Errorf("kind: expected ConfigMap, got %s", cm.Kind)
	}

	// Data should match input
	for k, v := range data {
		if cm.Data[k] != v {
			t.Errorf("key %s: expected %s, got %s", k, v, cm.Data[k])
		}
	}

	if len(cm.Data) != len(data) {
		t.Errorf("expected %d data entries, got %d", len(data), len(cm.Data))
	}

	// Labels
	if cm.Labels["astro.dev/agent"] != "my-agent" {
		t.Errorf("expected agent label my-agent, got %s", cm.Labels["astro.dev/agent"])
	}
}
