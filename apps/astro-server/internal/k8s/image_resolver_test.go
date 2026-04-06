package k8s

import (
	"testing"
)

func TestImageResolver_ResolveImage(t *testing.T) {
	tests := []struct {
		name              string
		proxyRegistryHost string
		ecrRegistryURL    string
		inputImage        string
		expectedImage     string
		expectError       bool
	}{
		{
			name:              "resolve proxy image to ECR",
			proxyRegistryHost: "registry.example.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.example.com/user_01kggdgfrw46qcsnxeqbr1hr1z/engineering-assistant:latest",
			expectedImage:     "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-user_01kggdgfrw46qcsnxeqbr1hr1z/engineering-assistant:latest",
			expectError:       false,
		},
		{
			name:              "resolve proxy image with version tag",
			proxyRegistryHost: "registry.example.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.example.com/user_abc123/myagent:v1.0.0",
			expectedImage:     "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-user_abc123/myagent:v1.0.0",
			expectError:       false,
		},
		{
			name:              "pass through non-proxy image",
			proxyRegistryHost: "registry.example.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "docker.io/nginx:latest",
			expectedImage:     "docker.io/nginx:latest",
			expectError:       false,
		},
		{
			name:              "pass through when no proxy host configured",
			proxyRegistryHost: "",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.example.com/user_xxx/image:latest",
			expectedImage:     "registry.example.com/user_xxx/image:latest",
			expectError:       false,
		},
		{
			name:              "error on invalid image format",
			proxyRegistryHost: "registry.example.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.example.com/invalid",
			expectedImage:     "",
			expectError:       true,
		},
		{
			name:              "resolve image with digest",
			proxyRegistryHost: "registry.example.com",
			ecrRegistryURL:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.example.com/user_123/image@sha256:abcdef123456",
			expectedImage:     "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-user_123/image@sha256:abcdef123456",
			expectError:       false,
		},
		{
			name:              "resolve with ECR URL without scheme",
			proxyRegistryHost: "registry.example.com",
			ecrRegistryURL:    "123456789.dkr.ecr.us-east-1.amazonaws.com",
			inputImage:        "registry.example.com/user_123/image:latest",
			expectedImage:     "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-user_123/image:latest",
			expectError:       false,
		},
		{
			name:              "error on empty ECR URL",
			proxyRegistryHost: "registry.example.com",
			ecrRegistryURL:    "",
			inputImage:        "registry.example.com/user_123/image:latest",
			expectedImage:     "",
			expectError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewImageResolver(tt.proxyRegistryHost, tt.ecrRegistryURL, "prod")
			result, err := resolver.ResolveImage(tt.inputImage)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expectedImage {
				t.Errorf("expected %s, got %s", tt.expectedImage, result)
			}
		})
	}
}

// ===== ECR namespace migration: old builds (account name) vs new builds (UUID) =====
//
// ImageResolver is called on images that are already stored in the deployment spec.
// For old builds the spec contains ECR URLs with account names; for new builds they
// contain UUIDs. In both cases ImageResolver must pass them through unchanged
// (they don't start with the proxy host, so no re-resolution occurs).
//
// The resolver also acts as a safety net for any edge case where an image is still
// in proxy format — tested here for completeness.

func TestImageResolver_OldBuild_AlreadyResolvedECRURLPassesThrough(t *testing.T) {
	// Old agent_version: ECR URL stored with account name namespace.
	// ImageResolver must not alter it — the ECR repo prod-tenant-saswatds still exists.
	resolver := NewImageResolver("registry.example.com", "https://123456789.dkr.ecr.us-east-1.amazonaws.com", "prod")

	oldBuildImage := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-saswatds/my-agent:abc"
	got, err := resolver.ResolveImage(oldBuildImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != oldBuildImage {
		t.Errorf("old build ECR URL should pass through unchanged: expected %s, got %s", oldBuildImage, got)
	}
}

func TestImageResolver_NewBuild_AlreadyResolvedUUIDECRURLPassesThrough(t *testing.T) {
	// New agent_version: ECR URL stored with UUID namespace.
	// ImageResolver must not alter it.
	resolver := NewImageResolver("registry.example.com", "https://123456789.dkr.ecr.us-east-1.amazonaws.com", "prod")

	newBuildImage := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-01kggdgfrw46qcsnxeqbr1hr1z/my-agent:newbuild"
	got, err := resolver.ResolveImage(newBuildImage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != newBuildImage {
		t.Errorf("new build ECR URL should pass through unchanged: expected %s, got %s", newBuildImage, got)
	}
}

func TestImageResolver_OldAndNewBuilds_DifferentECRPaths(t *testing.T) {
	// Sanity check: old and new build images resolve to different ECR paths
	// (the account-name path vs the UUID path), confirming coexistence.
	resolver := NewImageResolver("registry.example.com", "https://123456789.dkr.ecr.us-east-1.amazonaws.com", "prod")

	oldImage := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-saswatds/my-agent:abc"
	newImage := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-01kggdgfrw46qcsnxeqbr1hr1z/my-agent:newbuild"

	gotOld, err := resolver.ResolveImage(oldImage)
	if err != nil {
		t.Fatalf("unexpected error on old image: %v", err)
	}
	gotNew, err := resolver.ResolveImage(newImage)
	if err != nil {
		t.Fatalf("unexpected error on new image: %v", err)
	}

	if gotOld != oldImage {
		t.Errorf("old build: expected pass-through %s, got %s", oldImage, gotOld)
	}
	if gotNew != newImage {
		t.Errorf("new build: expected pass-through %s, got %s", newImage, gotNew)
	}
	if gotOld == gotNew {
		t.Error("old and new build images must remain distinct after resolution")
	}
}
