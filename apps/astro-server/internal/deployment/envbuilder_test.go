package deployment

import (
	"fmt"
	"testing"

	"github.com/postman/astro/packages/astro-spec"
)

// TestBuildConnectionStrings verifies that EnvBuilder.BuildConnectionStrings
// produces the correct environment variable keys and values for each provider:
// qdrant (HOST/PORT/URL with http scheme), redis (HOST/PORT/URL with redis scheme),
// postgres (HOST/PORT only, no URL since no URLScheme), self-hosted models
// (MODEL_{NAME}_*), self-hosted tools (TOOL_{NAME}_*), always-present keys
// (AGENT_URL, OTEL endpoint, ASTRO_AGENT_NAME), custom port overrides, and
// fallback to Knowledge.Type when Provider is empty.
func TestBuildConnectionStrings(t *testing.T) {
	tests := []struct {
		name       string
		spec       *spec.AstroSpec
		wantKeys   []string // keys that must exist
		wantValues map[string]string // exact key=value checks
	}{
		{
			name: "qdrant knowledge store",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Knowledge: map[string]spec.Knowledge{
					"vectors": {
						Provider: "qdrant",
					},
				},
			},
			wantKeys: []string{"QDRANT_HOST", "QDRANT_PORT", "QDRANT_URL"},
			wantValues: map[string]string{
				"QDRANT_PORT": "6333",
			},
		},
		{
			name: "redis knowledge store",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Knowledge: map[string]spec.Knowledge{
					"cache": {
						Provider: "redis",
					},
				},
			},
			wantKeys: []string{"REDIS_HOST", "REDIS_PORT", "REDIS_URL"},
			wantValues: map[string]string{
				"REDIS_PORT": "6379",
			},
		},
		{
			name: "postgres knowledge store - no URL (no URLScheme)",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Knowledge: map[string]spec.Knowledge{
					"db": {
						Provider: "postgres",
					},
				},
			},
			wantKeys: []string{"POSTGRES_HOST", "POSTGRES_PORT"},
			wantValues: map[string]string{
				"POSTGRES_PORT": "5432",
			},
		},
		{
			name: "self-hosted model",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Models: map[string]spec.Model{
					"embedder": {
						Container: spec.ContainerConfig{
							Image: "embedder:latest",
							Port:  9000,
						},
					},
				},
			},
			wantKeys: []string{"MODEL_EMBEDDER_HOST", "MODEL_EMBEDDER_PORT", "MODEL_EMBEDDER_URL"},
			wantValues: map[string]string{
				"MODEL_EMBEDDER_PORT": "9000",
			},
		},
		{
			name: "self-hosted tool",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Tools: map[string]spec.Tool{
					"mcp-server": {
						Container: &spec.ContainerConfig{
							Image: "mcp:latest",
							Port:  3000,
						},
					},
				},
			},
			wantKeys: []string{"TOOL_MCP-SERVER_HOST", "TOOL_MCP-SERVER_PORT", "TOOL_MCP-SERVER_URL"},
			wantValues: map[string]string{
				"TOOL_MCP-SERVER_PORT": "3000",
			},
		},
		{
			name: "always includes AGENT_URL and OTEL endpoint and ASTRO_AGENT_NAME",
			spec: &spec.AstroSpec{
				Agent: "my-agent",
				Meta:  spec.Meta{Version: "2.0"},
				Container: spec.Container{Image: "agent:latest"},
			},
			wantKeys: []string{"AGENT_URL", "OTEL_EXPORTER_OTLP_ENDPOINT", "ASTRO_AGENT_NAME"},
			wantValues: map[string]string{
				"ASTRO_AGENT_NAME":    "my-agent",
				"ASTRO_AGENT_VERSION": "2.0",
			},
		},
		{
			name: "custom port overrides provider default",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Knowledge: map[string]spec.Knowledge{
					"custom-redis": {
						Container: &spec.ContainerConfig{
							Image: "redis:latest",
							Port:  16379,
						},
					},
				},
			},
			wantValues: map[string]string{
				"KNOWLEDGE_CUSTOM-REDIS_PORT": "16379",
			},
		},
		{
			name: "knowledge without provider falls back to generic env vars",
			spec: &spec.AstroSpec{
				Agent: "test-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Knowledge: map[string]spec.Knowledge{
					"custom-store": {
						Container: &spec.ContainerConfig{
							Image: "custom-store:latest",
							Port:  5000,
						},
					},
				},
			},
			wantKeys: []string{"KNOWLEDGE_CUSTOM-STORE_HOST", "KNOWLEDGE_CUSTOM-STORE_PORT"},
			wantValues: map[string]string{
				"KNOWLEDGE_CUSTOM-STORE_PORT": "5000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewEnvBuilder("test-ns")
			env := builder.BuildConnectionStrings(tt.spec)

			for _, key := range tt.wantKeys {
				if _, ok := env[key]; !ok {
					t.Errorf("expected key %s to be present, available keys: %v", key, keys(env))
				}
			}

			for key, want := range tt.wantValues {
				got, ok := env[key]
				if !ok {
					t.Errorf("expected key %s to be present", key)
					continue
				}
				if got != want {
					t.Errorf("key %s: expected %q, got %q", key, want, got)
				}
			}
		})
	}
}

// TestBuildConnectionStringsQdrantURL verifies the full QDRANT_URL value uses
// the http scheme and the correct service DNS hostname with port 6333.
func TestBuildConnectionStringsQdrantURL(t *testing.T) {
	s := &spec.AstroSpec{
		Agent:     "test-agent",
		Meta:      spec.Meta{Version: "1.0"},
		Container: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"vectors": {
				Provider: "qdrant",
			},
		},
	}

	builder := NewEnvBuilder("prod-ns")
	env := builder.BuildConnectionStrings(s)

	url := env["QDRANT_URL"]
	host := env["QDRANT_HOST"]
	expectedURL := fmt.Sprintf("http://%s:6333", host)
	if url != expectedURL {
		t.Errorf("QDRANT_URL: expected %s, got %s", expectedURL, url)
	}
}

// TestBuildConnectionStringsRedisURL verifies the full REDIS_URL value uses
// the redis:// scheme and the correct service DNS hostname with port 6379.
func TestBuildConnectionStringsRedisURL(t *testing.T) {
	s := &spec.AstroSpec{
		Agent:     "test-agent",
		Meta:      spec.Meta{Version: "1.0"},
		Container: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"cache": {
				Provider: "redis",
			},
		},
	}

	builder := NewEnvBuilder("prod-ns")
	env := builder.BuildConnectionStrings(s)

	url := env["REDIS_URL"]
	host := env["REDIS_HOST"]
	expectedURL := fmt.Sprintf("redis://%s:6379", host)
	if url != expectedURL {
		t.Errorf("REDIS_URL: expected %s, got %s", expectedURL, url)
	}
}

// TestBuildConnectionStringsPostgresNoURL verifies that postgres does not
// produce a POSTGRES_URL key because the postgres provider has no URLScheme.
func TestBuildConnectionStringsPostgresNoURL(t *testing.T) {
	s := &spec.AstroSpec{
		Agent:     "test-agent",
		Meta:      spec.Meta{Version: "1.0"},
		Container: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"db": {
				Provider: "postgres",
			},
		},
	}

	builder := NewEnvBuilder("prod-ns")
	env := builder.BuildConnectionStrings(s)

	if _, ok := env["POSTGRES_URL"]; ok {
		t.Errorf("postgres should not have URL (no URLScheme), but POSTGRES_URL was present: %s", env["POSTGRES_URL"])
	}
}

// TestBuildCredentialEnvRefs verifies that BuildCredentialEnvRefs produces
// EnvVar entries with uppercased names and SecretKeyRef pointing to the given
// secret name and original key.
func TestBuildCredentialEnvRefs(t *testing.T) {
	builder := NewEnvBuilder("test-ns")

	creds := map[string]string{
		"anthropic_api_key": "sk-test",
		"GITHUB_TOKEN":      "ghp-test",
	}

	refs := builder.BuildCredentialEnvRefs(creds, "my-secret")

	if len(refs) != 2 {
		t.Fatalf("expected 2 env refs, got %d", len(refs))
	}

	refMap := make(map[string]EnvVar)
	for _, ref := range refs {
		refMap[ref.Name] = ref
	}

	// anthropic_api_key → ANTHROPIC_API_KEY
	if ref, ok := refMap["ANTHROPIC_API_KEY"]; !ok {
		t.Error("expected uppercased key ANTHROPIC_API_KEY")
	} else {
		if ref.ValueFrom == nil || ref.ValueFrom.SecretKeyRef == nil {
			t.Fatal("expected SecretKeyRef")
		}
		if ref.ValueFrom.SecretKeyRef.Name != "my-secret" {
			t.Errorf("expected secret name my-secret, got %s", ref.ValueFrom.SecretKeyRef.Name)
		}
		if ref.ValueFrom.SecretKeyRef.Key != "anthropic_api_key" {
			t.Errorf("expected original key anthropic_api_key, got %s", ref.ValueFrom.SecretKeyRef.Key)
		}
	}

	// GITHUB_TOKEN stays as-is
	if ref, ok := refMap["GITHUB_TOKEN"]; !ok {
		t.Error("expected key GITHUB_TOKEN")
	} else {
		if ref.ValueFrom.SecretKeyRef.Key != "GITHUB_TOKEN" {
			t.Errorf("expected key GITHUB_TOKEN, got %s", ref.ValueFrom.SecretKeyRef.Key)
		}
	}
}

// TestBuildCredentialEnvRefsEmpty verifies that BuildCredentialEnvRefs returns
// nil for empty credentials.
func TestBuildCredentialEnvRefsEmpty(t *testing.T) {
	builder := NewEnvBuilder("test-ns")
	refs := builder.BuildCredentialEnvRefs(map[string]string{}, "my-secret")
	if len(refs) != 0 {
		t.Errorf("expected 0 env refs for empty creds, got %d", len(refs))
	}
}

// TestBuildConfigMapEnvRefs verifies that BuildConfigMapEnvRefs produces EnvVar
// entries with ConfigMapKeyRef pointing to the given configmap name and key.
func TestBuildConfigMapEnvRefs(t *testing.T) {
	builder := NewEnvBuilder("test-ns")

	configMapKeys := []string{"QDRANT_HOST", "QDRANT_PORT", "AGENT_URL"}
	refs := builder.BuildConfigMapEnvRefs("my-config", configMapKeys)

	if len(refs) != 3 {
		t.Fatalf("expected 3 env refs, got %d", len(refs))
	}

	for i, ref := range refs {
		if ref.Name != configMapKeys[i] {
			t.Errorf("ref[%d] name: expected %s, got %s", i, configMapKeys[i], ref.Name)
		}
		if ref.ValueFrom == nil || ref.ValueFrom.ConfigMapKeyRef == nil {
			t.Fatalf("ref[%d]: expected ConfigMapKeyRef", i)
		}
		if ref.ValueFrom.ConfigMapKeyRef.Name != "my-config" {
			t.Errorf("ref[%d] configmap: expected my-config, got %s", i, ref.ValueFrom.ConfigMapKeyRef.Name)
		}
		if ref.ValueFrom.ConfigMapKeyRef.Key != configMapKeys[i] {
			t.Errorf("ref[%d] key: expected %s, got %s", i, configMapKeys[i], ref.ValueFrom.ConfigMapKeyRef.Key)
		}
	}
}

// TestBuildConfigMapEnvRefsEmpty verifies that BuildConfigMapEnvRefs returns
// nil for an empty key list.
func TestBuildConfigMapEnvRefsEmpty(t *testing.T) {
	builder := NewEnvBuilder("test-ns")
	refs := builder.BuildConfigMapEnvRefs("my-config", nil)
	if len(refs) != 0 {
		t.Errorf("expected 0 env refs for nil keys, got %d", len(refs))
	}
}

// TestBuildConnectionStringsGRPCAddr verifies that BuildConnectionStrings
// includes GRPC_SERVER_ADDR when interfaces are defined.
func TestBuildConnectionStringsGRPCAddr(t *testing.T) {
	s := &spec.AstroSpec{
		Agent:     "my-agent",
		Meta:      spec.Meta{Version: "1.0"},
		Container: spec.Container{Image: "agent:latest"},
		Interfaces: map[string]spec.Interface{
			"slack": {Type: "slack"},
		},
	}

	builder := NewEnvBuilder("prod-ns")
	env := builder.BuildConnectionStrings(s)

	grpcAddr, ok := env["GRPC_SERVER_ADDR"]
	if !ok {
		t.Fatal("expected GRPC_SERVER_ADDR to be present")
	}
	if grpcAddr == "" {
		t.Error("GRPC_SERVER_ADDR should not be empty")
	}
}

func keys(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
